// AGENTV1 FILE START: YAML key is used only in the HTTP header, not serialized config/payload.
package exporter

import (
	"context"
	"github.com/agent-i/agent/internal/config"
	"github.com/agent-i/agent/internal/policy"
	"io"
	"strings"
	"testing"
)

func TestYAMLAuthRequest(t *testing.T) {
	c, e := config.Parse(strings.NewReader("observe:\n  endpoint: https://example.com/api/v1/otlp\n  api_key: fixture-inline-only\ncollection:\n  metrics:\n    enabled: true\n"))
	if e != nil {
		t.Fatal(e)
	}
	r, e := Request(context.Background(), c, policy.Metrics, strings.NewReader(`{"resourceMetrics":[]}`), c.SecretProvider())
	if e != nil {
		t.Fatal(e)
	}
	if r.Header.Get("Authorization") != "ApiKey fixture-inline-only" {
		t.Fatal("authorization contract")
	}
	raw, _ := io.ReadAll(r.Body)
	if strings.Contains(string(raw), "fixture-inline-only") {
		t.Fatal("secret in payload")
	}
}

// AGENTV1 FILE END
