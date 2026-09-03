// AGENTV1 FILE START: request-only tests, no real backend traffic.
package exporter

import (
	"context"
	"github.com/agent-i/agent/internal/config"
	"github.com/agent-i/agent/internal/policy"
	"github.com/agent-i/agent/internal/security"
	"io"
	"strings"
	"testing"
)

func TestRequestContract(t *testing.T) {
	t.Setenv("TEST_OBSERVE_AUTH", "ApiKey unit-test-not-a-real-key")
	cfg := config.Config{Exporter: config.Exporter{Type: "otlp_http", Endpoint: "https://ingest.example.com/api/v1/otlp", HeadersEnv: map[string]string{"Authorization": "TEST_OBSERVE_AUTH"}}, Policy: policy.Document{Version: 1, Enabled: map[policy.Capability]bool{policy.Metrics: true, policy.Logs: true, policy.Traces: true}}, Limits: config.Limits{RequestBytes: 4096, QueueBytes: 1 << 20, MemoryMiB: 64, ShutdownSeconds: 15}}
	for _, signal := range []policy.Capability{policy.Metrics, policy.Logs, policy.Traces} {
		r, e := Request(context.Background(), cfg, signal, strings.NewReader(`{}`), security.Environment{})
		if e != nil {
			t.Fatal(e)
		}
		if r.URL.Path != "/api/v1/otlp/v1/"+string(signal) || r.Header.Get("Authorization") != "ApiKey unit-test-not-a-real-key" || r.Header.Get("Content-Type") != "application/json" {
			t.Fatal("wire contract")
		}
		io.Copy(io.Discard, r.Body)
		r.Body.Close()
		r.Header.Del("Authorization")
	}
	cfg.Policy.Enabled[policy.Logs] = false
	if _, e := Request(context.Background(), cfg, policy.Logs, strings.NewReader(`{}`), security.Environment{}); e == nil {
		t.Fatal("disabled signal export")
	}
	cfg.Exporter.Endpoint = "http://ingest.example.com/api/v1/otlp"
	if _, e := Request(context.Background(), cfg, policy.Metrics, strings.NewReader(`{}`), security.Environment{}); e == nil {
		t.Fatal("plaintext export")
	}
	if Client().CheckRedirect(nil, nil) == nil {
		t.Fatal("redirect allowed")
	}
}
func TestSecretFailuresAreRedacted(t *testing.T) {
	for _, value := range []string{"", "Bearer forbidden-secret", "ApiKey forbidden\nsecret"} {
		t.Setenv("TEST_OBSERVE_AUTH", value)
		_, e := (security.Environment{}).Authorization(context.Background(), "TEST_OBSERVE_AUTH")
		if e == nil || strings.Contains(e.Error(), "forbidden") {
			t.Fatal("credential error unsafe")
		}
	}
}

type countingSecrets struct{ calls int }

func (s *countingSecrets) Authorization(context.Context, string) ([]byte, error) {
	s.calls++
	return []byte("ApiKey fixture-only"), nil
}
func TestPayloadBoundsBeforeSecretAccess(t *testing.T) {
	cfg := config.Config{Exporter: config.Exporter{Type: "otlp_http", Endpoint: "https://ingest.example.invalid/api/v1/otlp", HeadersEnv: map[string]string{"Authorization": "TEST_AUTH"}}, Policy: policy.Document{Version: 1, Enabled: map[policy.Capability]bool{policy.Metrics: true}}, Limits: config.Limits{RequestBytes: 1024, QueueBytes: 1 << 20, MemoryMiB: 64, ShutdownSeconds: 15}}
	for _, test := range []struct {
		name string
		body io.Reader
	}{
		{"nil", nil}, {"malformed", strings.NewReader(`{"secret":`)}, {"oversize", strings.NewReader(`{"x":"` + strings.Repeat("a", 1024) + `"}`)},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := &countingSecrets{}
			if _, err := Request(context.Background(), cfg, policy.Metrics, test.body, s); err == nil {
				t.Fatal("bad payload accepted")
			}
			if s.calls != 0 {
				t.Fatal("credentials read before safe payload")
			}
		})
	}
	s := &countingSecrets{}
	if _, err := Request(context.Background(), cfg, policy.Logs, strings.NewReader(`{}`), s); err == nil || s.calls != 0 {
		t.Fatal("disabled signal accessed credentials")
	}
	c := Client()
	if c.Timeout <= 0 {
		t.Fatal("unbounded client")
	}
}

// AGENTV1 FILE END
