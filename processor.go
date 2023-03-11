// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package jniprocessor // import "github.com/afishbeck/opentelemetry-collector-contrib/processor/jniprocessor"

import (
	"context"
	"errors"
	"log"
	"runtime"

	"github.com/afishbeck/jnigi"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/processor"
	"go.uber.org/zap"
)

const (
	// The value of "type" key in configuration.
	typeStr = "jni"
	// The stability level of the processor.
	stability = component.StabilityLevelDevelopment
)

var jvm *jnigi.JVM

func init() {
	if err := jnigi.LoadJVMLib(jnigi.AttemptToFindJVMLibPath()); err != nil {
		log.Fatal(err)
	}

	runtime.LockOSThread()
	jvm2, _, err := jnigi.CreateJVM(jnigi.NewJVMInitArgs(false, true, jnigi.DEFAULT_VERSION, []string{"-Xcheck:jni"}))
	if err != nil {
		log.Fatal(err)
	}
	jvm = jvm2

	runtime.UnlockOSThread()
}

func NewFactory() processor.Factory {
	return processor.NewFactory(
		typeStr,
		createDefaultConfig,
		processor.WithTraces(createTracesProcessor, stability),
		processor.WithMetrics(createMetricsProcessor, stability),
		processor.WithLogs(createLogsProcessor, stability),
	)
}

type Config struct {
	jarpath string
}

func createDefaultConfig() component.Config {
	return &Config{}
}

// Validate checks if the processor configuration is valid
func (cfg *Config) Validate() error {
	return nil
}

func createLogsProcessor(ctx context.Context, set processor.CreateSettings, cfg component.Config, lgcon consumer.Logs) (processor.Logs, error) {
	pConfig := cfg.(*Config)
	return newLogProcessor(pConfig, lgcon, set.Logger)
}

type logsJniProcessor struct {
	logger       *zap.Logger
	config       *Config
	nextConsumer consumer.Logs
}

func newLogProcessor(config *Config, nextConsumer consumer.Logs, logger *zap.Logger) (*logsJniProcessor, error) {
	if config.jarpath == "fail" {
		return nil, errors.New("you wanted to fail")
	}
	p := &logsJniProcessor{
		logger:       logger,
		config:       config,
		nextConsumer: nextConsumer,
	}
	return p, nil
}

func (ltp *logsJniProcessor) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{MutatesData: true}
}

func (ltp *logsJniProcessor) Shutdown(ctx context.Context) error {
	ltp.logger.Info("Stopping logs jni processor")
	return nil
}

func (ltp *logsJniProcessor) Start(ctx context.Context, host component.Host) error {
	ltp.logger.Info("Starting logs jni processor")
	return nil
}

func (ltp *logsJniProcessor) ConsumeLogs(ctx context.Context, ld plog.Logs) error {
	marshaler := &plog.ProtoMarshaler{}
	marshaledBytes, err := marshaler.MarshalLogs(ld)
	if err != nil {
		ltp.logger.Error("error marshaling logs to protobuffers")
		return nil
	}

	// hopefully the concurrency can be optimized.. need to investigate processor threading later
	runtime.LockOSThread()
	nenv := jvm.AttachCurrentThread()

	var processedBytes []byte
	if err = nenv.CallStaticMethod("OtelJniProcessor", "processLogs", processedBytes, marshaledBytes); err != nil {
		ltp.logger.Error("error callning Java method OtelJniProcessor/Processor.processLogs")
		return nil
	}

	if err := jvm.DetachCurrentThread(); err != nil {
		ltp.logger.Error("error detaching thread")
		return nil
	}
	runtime.UnlockOSThread() // need to investigate how this interacts with the rest of the pipeline

	unmarshaler := &plog.ProtoUnmarshaler{}
	ld2, err2 := unmarshaler.UnmarshalLogs(processedBytes)
	if err2 != nil {
		ltp.logger.Error("error unmarshaling from processedBytes protobuffers")
		return nil
	}

	ld2.CopyTo(ld)
	return nil
}

func createTracesProcessor(ctx context.Context, set processor.CreateSettings, cfg component.Config, trcon consumer.Traces) (processor.Traces, error) {
	pConfig := cfg.(*Config)
	return newTracesProcessor(pConfig, trcon, set.Logger)
}

type tracesJniProcessor struct {
	logger       *zap.Logger
	config       *Config
	nextConsumer consumer.Traces
}

func newTracesProcessor(config *Config, nextConsumer consumer.Traces, logger *zap.Logger) (*tracesJniProcessor, error) {
	if config.jarpath == "fail" {
		return nil, errors.New("you wanted to fail")
	}
	p := &tracesJniProcessor{
		logger:       logger,
		config:       config,
		nextConsumer: nextConsumer,
	}
	return p, nil
}

func (ltp *tracesJniProcessor) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{MutatesData: true}
}

func (ltp *tracesJniProcessor) Shutdown(ctx context.Context) error {
	ltp.logger.Info("Stopping traces jni processor")
	return nil
}

func (ltp *tracesJniProcessor) Start(ctx context.Context, host component.Host) error {
	ltp.logger.Info("Starting traces jni processor")
	return nil
}

func (ltp *tracesJniProcessor) ConsumeTraces(ctx context.Context, traces ptrace.Traces) error {
	marshaler := &ptrace.ProtoMarshaler{}
	marshaledBytes, err := marshaler.MarshalTraces(traces)
	if err != nil {
		ltp.logger.Error("error marshaling traces to protobuffers")
		return nil
	}

	// hopefully the concurrency can be optimized.. need to investigate processor threading later
	runtime.LockOSThread()
	nenv := jvm.AttachCurrentThread()

	var processedBytes []byte
	if err = nenv.CallStaticMethod("OtelJniProcessor", "processTraces", processedBytes, marshaledBytes); err != nil {
		ltp.logger.Error("error callning Java method OtelJniProcessor/Processor.processTraces")
		return nil
	}

	if err := jvm.DetachCurrentThread(); err != nil {
		ltp.logger.Error("error detaching thread")
		return nil
	}
	runtime.UnlockOSThread() // need to investigate how this interacts with the rest of the pipeline

	unmarshaler := &ptrace.ProtoUnmarshaler{}
	traces2, err2 := unmarshaler.UnmarshalTraces(processedBytes)
	if err2 != nil {
		ltp.logger.Error("error unmarshaling traces from processedBytes protobuffers")
		return nil
	}

	traces.CopyTo(traces2)
	return nil
}

func createMetricsProcessor(ctx context.Context, set processor.CreateSettings, cfg component.Config, mtcon consumer.Metrics) (processor.Metrics, error) {
	pConfig := cfg.(*Config)
	return newMetricsProcessor(pConfig, mtcon, set.Logger)
}

type metricsJniProcessor struct {
	logger       *zap.Logger
	config       *Config
	nextConsumer consumer.Metrics
}

func newMetricsProcessor(config *Config, nextConsumer consumer.Metrics, logger *zap.Logger) (*metricsJniProcessor, error) {
	if config.jarpath == "fail" {
		return nil, errors.New("you wanted to fail")
	}
	p := &metricsJniProcessor{
		logger:       logger,
		config:       config,
		nextConsumer: nextConsumer,
	}

	return p, nil
}

func (ltp *metricsJniProcessor) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{MutatesData: true}
}

func (ltp *metricsJniProcessor) Shutdown(ctx context.Context) error {
	ltp.logger.Info("Stopping traces jni processor")
	return nil
}

func (ltp *metricsJniProcessor) Start(ctx context.Context, host component.Host) error {
	ltp.logger.Info("Starting traces jni processor")
	return nil
}

func (ltp *metricsJniProcessor) ConsumeMetrics(ctx context.Context, metrics pmetric.Metrics) error {
	marshaler := &pmetric.ProtoMarshaler{}
	marshaledBytes, err := marshaler.MarshalMetrics(metrics)
	if err != nil {
		ltp.logger.Error("error marshaling metrics to protobuffers")
		return nil
	}

	// hopefully the concurrency can be optimized.. need to investigate processor threading later
	runtime.LockOSThread()
	nenv := jvm.AttachCurrentThread()

	var processedBytes []byte
	if err = nenv.CallStaticMethod("OtelJniProcessor", "processMetrics", processedBytes, marshaledBytes); err != nil {
		ltp.logger.Error("error callning Java method OtelJniProcessor/Processor.processMetrics")
		return nil
	}

	if err := jvm.DetachCurrentThread(); err != nil {
		ltp.logger.Error("error detaching thread")
		return nil
	}
	runtime.UnlockOSThread() // need to investigate how this interacts with the rest of the pipeline

	unmarshaler := &pmetric.ProtoUnmarshaler{}
	metrics2, err2 := unmarshaler.UnmarshalMetrics(processedBytes)
	if err2 != nil {
		ltp.logger.Error("error unmarshaling metrics from processedBytes protobuffers")
		return nil
	}

	metrics2.CopyTo(metrics)
	return nil
}
