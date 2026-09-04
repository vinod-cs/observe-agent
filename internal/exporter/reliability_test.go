// AGENTV1 FILE START: final-attempt server retry delay cannot bypass persistent-worker cooldown.
package exporter

import (
	"context"
	"github.com/agent-i/agent/internal/config"
	"github.com/agent-i/agent/internal/policy"
	"github.com/agent-i/agent/internal/security"
	"github.com/agent-i/agent/internal/selftelemetry"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLastAttemptHonorsRetryAfter(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "1800")
		w.WriteHeader(429)
	}))
	defer server.Close()
	t.Setenv("TEST_RETRY_KEY", "ApiKey fixture-only")
	cfg, e := config.Parse(strings.NewReader(`{"exporter":{"type":"otlp_http","endpoint":"` + server.URL + `/api/v1/otlp","headers_env":{"Authorization":"TEST_RETRY_KEY"}},"delivery":{"max_attempts":1},"policy":{"version":1,"enabled":{"metrics":true}}}`))
	if e != nil {
		t.Fatal(e)
	}
	sender := NewSender(cfg, security.Environment{}, &selftelemetry.Counters{}, server.Client())
	var delay time.Duration
	sender.sleep = func(_ context.Context, d time.Duration) bool { delay = d; return true }
	if got := sender.Send(context.Background(), []byte(`{}`)); got != Exhausted || delay != 30*time.Minute {
		t.Fatalf("%s %s", got, delay)
	}
}

func TestLogsPartialSuccessIsFinal(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/otlp/v1/logs" {
			t.Error("wrong signal path")
		}
		w.WriteHeader(201)
		_, _ = w.Write([]byte(`{"partialSuccess":{"rejectedLogRecords":"1"}}`))
	}))
	defer server.Close()
	t.Setenv("TEST_INGEST", "ApiKey fixture-only")
	cfg := testConfig(t, server.URL)
	cfg.Policy.Enabled[policy.Logs] = true
	stats := &selftelemetry.Counters{}
	sender := NewSignalSender(cfg, policy.Logs, security.Environment{}, stats, server.Client())
	if got := sender.Send(context.Background(), []byte(`{"resourceLogs":[]}`)); got != Accepted || stats.PointsRejected.Load() != 1 {
		t.Fatalf("partial logs outcome=%s rejected=%d", got, stats.PointsRejected.Load())
	}
}

func TestLogsAuthenticationAndTransientReplaySemantics(t *testing.T) {
	for _, tc := range []struct {
		name     string
		statuses []int
		want     Outcome
	}{
		{"unauthorized", []int{401}, Unauthorized},
		{"forbidden", []int{403}, Unauthorized},
		{"throttled then accepted", []int{429, 201}, Accepted},
		{"unavailable then accepted", []int{503, 201}, Accepted},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v1/otlp/v1/logs" {
					t.Error("wrong Logs endpoint")
				}
				status := tc.statuses[min(calls, len(tc.statuses)-1)]
				calls++
				if status == 429 {
					w.Header().Set("Retry-After", "1")
				}
				w.WriteHeader(status)
				if status == 201 {
					_, _ = w.Write([]byte(`{"partialSuccess":{"rejectedLogRecords":0}}`))
				}
			}))
			defer server.Close()
			t.Setenv("TEST_INGEST", "ApiKey fixture-only")
			cfg := testConfig(t, server.URL)
			cfg.Policy.Enabled[policy.Logs] = true
			sender := NewSignalSender(cfg, policy.Logs, security.Environment{}, &selftelemetry.Counters{}, server.Client())
			sender.sleep = func(context.Context, time.Duration) bool { return true }
			if got := sender.Send(context.Background(), []byte(`{"resourceLogs":[]}`)); got != tc.want {
				t.Fatalf("outcome=%s calls=%d", got, calls)
			}
		})
	}
}

// AGENTV1 FILE END
