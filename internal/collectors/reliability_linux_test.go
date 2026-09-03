//go:build linux

// AGENTV1 FILE START: real persistent worker failure, cancellation and restart tests.
package collectors

import (
	"context"
	"errors"
	"github.com/agent-i/agent/internal/config"
	"github.com/agent-i/agent/internal/exporter"
	"github.com/agent-i/agent/internal/platform"
	"github.com/agent-i/agent/internal/queue"
	"github.com/agent-i/agent/internal/security"
	"github.com/agent-i/agent/internal/selftelemetry"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type unavailableReader struct{}
type faultTransport func(*http.Request) (*http.Response, error)

func (f faultTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func (unavailableReader) Sample(context.Context) (platform.Snapshot, error) {
	return platform.Snapshot{}, errors.New("fixture: no new samples")
}
func awaitCounter(t *testing.T, p func() bool) {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for !p() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !p() {
		t.Fatal("deadline")
	}
}
func TestPersistentWorkerFailureRestart(t *testing.T) {
	for _, status := range []int{-1, 401, 422, 429, 503} {
		name := http.StatusText(status)
		if status == -1 {
			name = "endpoint_unavailable"
		}
		t.Run(name, func(t *testing.T) {
			var failure atomic.Bool
			failure.Store(true)
			var requests atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				raw, _ := io.ReadAll(r.Body)
				if string(raw) != `{"resourceMetrics":[]}` {
					t.Error("wrong retained batch")
				}
				requests.Add(1)
				if failure.Load() {
					w.Header().Set("Retry-After", "1800")
					w.WriteHeader(status)
					return
				}
				w.WriteHeader(201)
				w.Write([]byte(`{}`))
			}))
			defer server.Close()
			client := server.Client()
			if status == -1 {
				transport := client.Transport
				client.Transport = faultTransport(func(r *http.Request) (*http.Response, error) {
					if failure.Load() {
						requests.Add(1)
						return nil, errors.New("fixture network unavailable")
					}
					return transport.RoundTrip(r)
				})
			}
			t.Setenv("TEST_RELIABILITY_KEY", "ApiKey fixture-only")
			cfg, e := config.Parse(strings.NewReader(`{"exporter":{"type":"otlp_http","endpoint":"` + server.URL + `/api/v1/otlp","headers_env":{"Authorization":"TEST_RELIABILITY_KEY"}},"delivery":{"max_attempts":1},"policy":{"version":1,"enabled":{"metrics":true,"logs":false,"traces":false}}}`))
			if e != nil {
				t.Fatal(e)
			}
			dir := t.TempDir() + "/spool"
			start := func() *Metrics {
				q, e := queue.OpenDisk(dir, "host/endpoint", 1<<20, 8)
				if e != nil {
					t.Fatal(e)
				}
				m := NewMetrics(cfg, &selftelemetry.Counters{})
				m.queue = q
				m.reader = unavailableReader{}
				m.sender = exporter.NewSender(cfg, security.Environment{}, m.stats, client)
				if e = m.launch(context.Background(), time.Hour); e != nil {
					t.Fatal(e)
				}
				return m
			}
			q, e := queue.OpenDisk(dir, "host/endpoint", 1<<20, 8)
			if e != nil {
				t.Fatal(e)
			}
			id, e := q.Put(context.Background(), []byte(`{"resourceMetrics":[]}`))
			if e != nil {
				t.Fatal(e)
			}
			q.Close(context.Background())
			m := start()
			awaitCounter(t, func() bool { return requests.Load() > 0 })
			if status == 401 || status == 422 {
				awaitCounter(t, func() bool { return m.blocked.Load() })
			}
			// Shutdown while Retry-After is waiting must retain the batch and stop promptly.
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if e = m.Stop(ctx); e != nil {
				t.Fatal(e)
			}
			q, e = queue.OpenDisk(dir, "host/endpoint", 1<<20, 8)
			if e != nil {
				t.Fatal(e)
			}
			item, e := q.Next(context.Background())
			if e != nil || item.Receipt != id {
				t.Fatal("failed delivery dequeued", e)
			}
			q.Close(context.Background())
			failure.Store(false)
			m = start()
			awaitCounter(t, func() bool { return m.stats.DeliveredRecords.Load() == 1 })
			if e = m.Stop(context.Background()); e != nil {
				t.Fatal(e)
			}
			if requests.Load() != 2 {
				t.Fatal("unexpected retry or duplicate local dequeue", requests.Load())
			}
			q, e = queue.OpenDisk(dir, "host/endpoint", 1<<20, 8)
			if e != nil {
				t.Fatal(e)
			}
			defer q.Close(context.Background())
			empty, cancelEmpty := context.WithTimeout(context.Background(), 5*time.Millisecond)
			defer cancelEmpty()
			if _, e = q.Next(empty); e == nil {
				t.Fatal("delivered record reappeared")
			}
		})
	}
}

// AGENTV1 FILE END
