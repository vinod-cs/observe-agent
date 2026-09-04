// AGENTV1 FILE START: real local TLS fault injection; no external endpoint or credential.
package exporter

import (
	"context"
	"github.com/agent-i/agent/internal/config"
	"github.com/agent-i/agent/internal/policy"
	"github.com/agent-i/agent/internal/security"
	"github.com/agent-i/agent/internal/selftelemetry"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testConfig(t *testing.T, endpoint string) config.Config {
	t.Helper()
	raw := `{"exporter":{"type":"otlp_http","endpoint":"` + endpoint + `/api/v1/otlp","headers_env":{"Authorization":"TEST_INGEST"}},"policy":{"version":1,"enabled":{"metrics":true}}}`
	c, e := config.Parse(strings.NewReader(raw))
	if e != nil {
		t.Fatal(e)
	}
	return c
}
func TestSenderHTTPOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name     string
		statuses []int
		body     string
		outcome  Outcome
		requests int
	}{
		{"created", []int{201}, `{"partialSuccess":{"rejectedDataPoints":0}}`, Accepted, 1},
		{"wrapped", []int{201}, `{"data":{"partialSuccess":{"rejectedDataPoints":"0"}}}`, Accepted, 1},
		{"unauthorized", []int{401}, "", Unauthorized, 1},
		{"forbidden", []int{403}, "", Unauthorized, 1},
		{"throttle recovers", []int{429, 201}, `{}`, Accepted, 2},
		{"unavailable recovers", []int{503, 201}, `{}`, Accepted, 2},
		{"exhausted", []int{503, 503, 503, 503}, "", Exhausted, 4},
		{"invalid payload", []int{422}, "", Rejected, 1},
		{"partial no duplicate retry", []int{201}, `{"partialSuccess":{"rejectedDataPoints":"2"}}`, Rejected, 1},
		{"malformed acknowledgement", []int{201}, "not-json", Rejected, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("TEST_INGEST", "ApiKey test-only-not-a-secret")
			count := 0
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v1/otlp/v1/metrics" || r.Header.Get("Authorization") != "ApiKey test-only-not-a-secret" || r.Header.Get("Content-Type") != "application/json" {
					t.Error("request contract")
				}
				io.Copy(io.Discard, r.Body)
				status := tc.statuses[min(count, len(tc.statuses)-1)]
				count++
				w.Header().Set("Retry-After", "2")
				w.WriteHeader(status)
				w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			stats := &selftelemetry.Counters{}
			sender := NewSender(testConfig(t, srv.URL), security.Environment{}, stats, srv.Client())
			waits := []time.Duration{}
			sender.sleep = func(ctx context.Context, d time.Duration) bool { waits = append(waits, d); return true }
			if got := sender.Send(context.Background(), []byte(`{"resourceMetrics":[]}`)); got != tc.outcome || count != tc.requests {
				t.Fatalf("got %s, requests %d", got, count)
			}
			if tc.statuses[0] == 429 && (len(waits) != 1 || waits[0] < 2*time.Second || stats.Throttles.Load() != 1) {
				t.Fatal("Retry-After ignored")
			}
			if tc.outcome == Unauthorized && stats.AuthFailures.Load() != 1 {
				t.Fatal("missing auth counter")
			}
			if tc.name == "partial no duplicate retry" && stats.PointsRejected.Load() != 2 {
				t.Fatal("missing rejected point count")
			}
		})
	}
}
func TestEndpointUnavailableAndCancellation(t *testing.T) {
	t.Setenv("TEST_INGEST", "ApiKey test-only-not-a-secret")
	srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	client := srv.Client()
	srv.Close()
	stats := &selftelemetry.Counters{}
	sender := NewSender(testConfig(t, url), security.Environment{}, stats, client)
	sender.sleep = func(context.Context, time.Duration) bool { return true }
	if sender.Send(context.Background(), []byte(`{}`)) != Exhausted || stats.ExportFailures.Load() != 4 || stats.Retries.Load() != 3 {
		t.Fatal("unavailable retry bounds")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if sender.Send(ctx, []byte(`{}`)) != Cancelled {
		t.Fatal("cancelled send")
	}
}
func TestTLSVerificationNotBypassed(t *testing.T) {
	t.Setenv("TEST_INGEST", "ApiKey test-only-not-a-secret")
	srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("untrusted endpoint received key") }))
	defer srv.Close()
	c := testConfig(t, srv.URL)
	c.Delivery.MaxAttempts = 1
	s := NewSender(c, security.Environment{}, &selftelemetry.Counters{}, nil)
	// AGENTV1 START: untrusted TLS pauses persistent delivery instead of retrying forever.
	if s.Send(context.Background(), []byte(`{}`)) != Rejected {
		t.Fatal("TLS trust bypassed")
	}
	// AGENTV1 END: permanent trust failure
}

func TestRetryAfterAndBackoff(t *testing.T) {
	now := time.Now()
	if retryDelay("1800", 0, now) != 30*time.Minute {
		t.Fatal("server delay shortened")
	}
	if retryDelay("9223372036854775807", 0, now) <= 0 {
		t.Fatal("duration overflow")
	}
	future := now.Add(time.Hour).UTC().Truncate(time.Second)
	if delay := retryDelay(future.Format(http.TimeFormat), 0, now); delay < 59*time.Minute || delay > time.Hour {
		t.Fatal("date Retry-After ignored")
	}
	if delay := retryDelayForSignal(policy.Logs, future.Format(http.TimeFormat), 0, now); delay != 30*time.Minute {
		t.Fatal("Logs Retry-After was not bounded")
	}
	if delay := retryDelay("invalid", 0, now); delay < time.Second || delay > 1500*time.Millisecond {
		t.Fatal("jitter bounds")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if wait(ctx, time.Hour) {
		t.Fatal("long retry sleep not cancellable")
	}
}

// AGENTV1 FILE END
