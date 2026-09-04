// AGENTV1 FILE START: pdata OTLP Logs shape and trusted resource enrichment tests.
package pipeline

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestLogJSONGoldenShape(t *testing.T) {
	raw, err := LogJSON(LogRecord{Body: "fixture", SourceID: "app", RelativePath: "app.log", FileIdentity: "1:2", StartOffset: 0, EndOffset: 8, ObservedAt: time.Unix(10, 0).UTC(), ServiceName: "checkout", Environment: "production"}, map[string]string{"host.id": "i-0123456789abcdef0", "cloud.provider": "aws", "cloud.account.id": "123", "cloud.region": "us-east-2", "telemetry.distro.name": "agent-i"}, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	var envelope map[string]any
	if json.Unmarshal(raw, &envelope) != nil || envelope["resourceLogs"] == nil {
		t.Fatal("not standard OTLP JSON")
	}
	text := string(raw)
	for _, want := range []string{"resourceLogs", "scopeLogs", "logRecords", "checkout", "production", "i-0123456789abcdef0", "cloud.account.id", "log.file.path"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %s", want)
		}
	}
	if strings.Contains(text, "Authorization") {
		t.Fatal("credential field in payload")
	}
}

// AGENTV1 FILE END
