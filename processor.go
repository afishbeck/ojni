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
	"log"
	"runtime"

	"github.com/afishbeck/jnigi"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/processor/processorhelper"
	"go.uber.org/zap"
)

var jvm *jnigi.JVM

func init() {
	if err := jnigi.LoadJVMLib(jnigi.AttemptToFindJVMLibPath()); err != nil {
		log.Fatal(err)
	}

	runtime.LockOSThread()
	jvm2, _, err := jnigi.CreateJVM(jnigi.NewJVMInitArgs(false, true, jnigi.DEFAULT_VERSION, []string{"-Djava.class.path=/home/tony/otel/otelcol-dev/lib-all.jar", "-Xcheck:jni"}))

	if err != nil {
		log.Fatal(err)
	}
	jvm = jvm2

	runtime.UnlockOSThread()
}

const (
	// The value of "type" key in configuration.
	typeStr = "jni"
	// The stability level of the processor.
	stability = component.StabilityLevelDevelopment
)

var processorCapabilities = consumer.Capabilities{MutatesData: true}

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
	test string
}

func createDefaultConfig() component.Config {
	return &Config{}
}

// Validate checks if the processor configuration is valid
func (cfg *Config) Validate() error {
	return nil
}

type jniProcessor struct {
	logger *zap.Logger
}

func createTracesProcessor(
	ctx context.Context,
	set processor.CreateSettings,
	cfg component.Config,
	nextConsumer consumer.Traces) (processor.Traces, error) {

	proc := &jniProcessor{logger: set.Logger}
	return processorhelper.NewTracesProcessor(
		ctx,
		set,
		cfg,
		nextConsumer,
		proc.processTraces,
		processorhelper.WithCapabilities(processorCapabilities))
}

func createMetricsProcessor(
	ctx context.Context,
	set processor.CreateSettings,
	cfg component.Config,
	nextConsumer consumer.Metrics) (processor.Metrics, error) {
	proc := &jniProcessor{logger: set.Logger}
	return processorhelper.NewMetricsProcessor(
		ctx,
		set,
		cfg,
		nextConsumer,
		proc.processMetrics,
		processorhelper.WithCapabilities(processorCapabilities))
}

func createLogsProcessor(
	ctx context.Context,
	set processor.CreateSettings,
	cfg component.Config,
	nextConsumer consumer.Logs) (processor.Logs, error) {
	proc := &jniProcessor{logger: set.Logger}
	return processorhelper.NewLogsProcessor(
		ctx,
		set,
		cfg,
		nextConsumer,
		proc.processLogs,
		processorhelper.WithCapabilities(processorCapabilities))
}

func (ltp *jniProcessor) processTraces(ctx context.Context, traces ptrace.Traces) (ptrace.Traces, error) {
	marshaler := &ptrace.ProtoMarshaler{}
	marshaledBytes, err := marshaler.MarshalTraces(traces)
	if err != nil {
		ltp.logger.Info("error marshaling traces to protobuffers")
		ltp.logger.Error(err.Error())
		return traces, nil
	}

	// hopefully the concurrency can be optimized.. need to investigate processor threading later
	runtime.LockOSThread()
	nenv := jvm.AttachCurrentThread()

	var processedBytes []byte
	if err = nenv.CallStaticMethod("ojnilib/Library", "processTraces", &processedBytes, marshaledBytes); err != nil {
		ltp.logger.Info("error callning Java method ojnilib/Library.processTraces")
		ltp.logger.Error(err.Error())
		return traces, nil
	}

	if err := jvm.DetachCurrentThread(); err != nil {
		ltp.logger.Info("error detaching thread")
		ltp.logger.Error(err.Error())
		return traces, nil
	}
	runtime.UnlockOSThread() // need to investigate how this interacts with the rest of the pipeline

	unmarshaler := &ptrace.ProtoUnmarshaler{}
	traces2, err2 := unmarshaler.UnmarshalTraces(processedBytes)
	if err2 != nil {
		ltp.logger.Error("error unmarshaling traces from processedBytes protobuffers")
		ltp.logger.Error(err2.Error())
		return traces, nil
	}

	jsonmarshaler := ptrace.JSONMarshaler{}
	jsonret, _ := jsonmarshaler.MarshalTraces(traces2)
	ltp.logger.Info("processTraces result", zap.String("json", string(jsonret)))

	//traces2.CopyTo(traces)

	return traces2, nil
}

func (ltp *jniProcessor) processLogs(ctx context.Context, ld plog.Logs) (plog.Logs, error) {
	marshaler := &plog.ProtoMarshaler{}
	marshaledBytes, err := marshaler.MarshalLogs(ld)
	if err != nil {
		ltp.logger.Error("error marshaling logs to protobuffers")
		ltp.logger.Error(err.Error())
		return ld, nil
	}

	// hopefully the concurrency can be optimized.. need to investigate processor threading later
	runtime.LockOSThread()
	nenv := jvm.AttachCurrentThread()

	var processedBytes []byte
	if err = nenv.CallStaticMethod("ojnilib/Library", "processLogs", &processedBytes, marshaledBytes); err != nil {
		ltp.logger.Info("error callning Java method ojnilib/Library.processLogs")
		ltp.logger.Error(err.Error())
		return ld, nil
	}

	if err := jvm.DetachCurrentThread(); err != nil {
		ltp.logger.Error("error detaching thread")
		ltp.logger.Error(err.Error())
		return ld, nil
	}
	runtime.UnlockOSThread() // need to investigate how this interacts with the rest of the pipeline

	unmarshaler := &plog.ProtoUnmarshaler{}
	ld2, err2 := unmarshaler.UnmarshalLogs(processedBytes)
	if err2 != nil {
		ltp.logger.Error("error unmarshaling from processedBytes protobuffers")
		ltp.logger.Error(err.Error())
		return ld, nil
	}

	jsonmarshaler := plog.JSONMarshaler{}
	jsonret, _ := jsonmarshaler.MarshalLogs(ld2)
	ltp.logger.Info("processLogs result", zap.String("json", string(jsonret)))

	//ld2.CopyTo(ld)
	return ld2, nil
}

func (ltp *jniProcessor) processMetrics(ctx context.Context, metrics pmetric.Metrics) (pmetric.Metrics, error) {
	marshaler := &pmetric.ProtoMarshaler{}
	marshaledBytes, err := marshaler.MarshalMetrics(metrics)
	if err != nil {
		ltp.logger.Info("error marshaling metrics to protobuffers")
		ltp.logger.Error(err.Error())
		return metrics, nil
	}

	// hopefully the concurrency can be optimized.. need to investigate processor threading later
	runtime.LockOSThread()
	nenv := jvm.AttachCurrentThread()

	var processedBytes []byte
	if err = nenv.CallStaticMethod("ojnilib/Library", "processMetrics", &processedBytes, marshaledBytes); err != nil {
		ltp.logger.Info("error callning Java method ojnilib/Library.processMetrics")
		ltp.logger.Error(err.Error())
		return metrics, nil
	}

	if err := jvm.DetachCurrentThread(); err != nil {
		ltp.logger.Error("error detaching thread")
		return metrics, nil
	}
	runtime.UnlockOSThread() // need to investigate how this interacts with the rest of the pipeline

	unmarshaler := &pmetric.ProtoUnmarshaler{}
	metrics2, err2 := unmarshaler.UnmarshalMetrics(processedBytes)
	if err2 != nil {
		ltp.logger.Error("error unmarshaling metrics from processedBytes protobuffers")
		return metrics, nil
	}

	jsonmarshaler := pmetric.JSONMarshaler{}
	jsonret, _ := jsonmarshaler.MarshalMetrics(metrics2)
	ltp.logger.Info("processMetrics result", zap.String("json", string(jsonret)))

	//metrics2.CopyTo(metrics)
	return metrics2, nil
}
