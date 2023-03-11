package OtelJniProcessor;

public class Processor
{
    int counter=0;
    public static byte[] processLogs(byte[] logdata)
    {
        return logdata;
    }

    public static byte[] processMetrics(byte[] mtdata)
    {
        return mtdata;
    }

    public static byte[] processTraces(byte[] trdata)
    {
        return trdata;
    }
}

