// AGENTV1 FILE START: final-attempt server retry delay cannot bypass persistent-worker cooldown.
package exporter

import (
	"context"
	"github.com/agent-i/agent/internal/config"
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

// AGENTV1 FILE END
