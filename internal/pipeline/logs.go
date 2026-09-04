// AGENTV1 FILE START: standard pdata OTLP JSON construction for Agent file logs.
package pipeline

import (
	"errors"
	"path/filepath"
	"strconv"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
)

type LogRecord struct {
	Body                     string
	SourceID                 string
	RelativePath             string
	FileIdentity             string
	StartOffset, EndOffset   int64
	ObservedAt               time.Time
	ServiceName, Environment string
	SourceType, SeverityText string
	Attributes               map[string]string
}

func LogJSON(in LogRecord, resource map[string]string, maxBytes int) ([]byte, error) {
	if in.Body == "" || len(in.Body) > 1<<20 || maxBytes < 1024 {
		return nil, errors.New("log record invalid")
	}
	out := plog.NewLogs()
	rl := out.ResourceLogs().AppendEmpty()
	for k, v := range resource {
		rl.Resource().Attributes().PutStr(k, v)
	}
	if in.ServiceName != "" {
		rl.Resource().Attributes().PutStr("service.name", in.ServiceName)
	}
	if in.Environment != "" {
		rl.Resource().Attributes().PutStr("deployment.environment.name", in.Environment)
	}
	scope := rl.ScopeLogs().AppendEmpty()
	scopeName := "observe-agent.filelog"
	if in.SourceType == "journald" {
		scopeName = "observe-agent.journald"
	}
	scope.Scope().SetName(scopeName)
	scope.Scope().SetVersion("1")
	record := scope.LogRecords().AppendEmpty()
	record.Body().SetStr(in.Body)
	ts := pcommon.NewTimestampFromTime(in.ObservedAt)
	record.SetObservedTimestamp(ts)
	record.SetTimestamp(ts)
	if in.SeverityText != "" {
		record.SetSeverityText(in.SeverityText)
	}
	record.Attributes().PutStr("observe.log.source.id", in.SourceID)
	if in.SourceType != "" {
		record.Attributes().PutStr("observe.log.source.type", in.SourceType)
	}
	if in.RelativePath != "" {
		record.Attributes().PutStr("log.file.name", filepath.Base(in.RelativePath))
		record.Attributes().PutStr("log.file.path", in.RelativePath)
		record.Attributes().PutStr("log.file.identity", in.FileIdentity)
		record.Attributes().PutStr("log.file.record_offset", strconv.FormatInt(in.StartOffset, 10))
		record.Attributes().PutStr("log.file.record_end_offset", strconv.FormatInt(in.EndOffset, 10))
	}
	for key, value := range in.Attributes {
		if key != "" && value != "" {
			record.Attributes().PutStr(key, value)
		}
	}
	raw, err := (&plog.JSONMarshaler{}).MarshalLogs(out)
	if err != nil || len(raw) > maxBytes {
		return nil, errors.New("OTLP log serialization failed or exceeds request bound")
	}
	return raw, nil
}

// AGENTV1 FILE END
