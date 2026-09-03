//go:build linux

// AGENTV1 FILE START: complete persistent scrape-to-OTLP TLS path; no external telemetry.
package collectors

import (
	"context"
	"github.com/agent-i/agent/internal/config"
	"github.com/agent-i/agent/internal/exporter"
	"github.com/agent-i/agent/internal/platform"
	"github.com/agent-i/agent/internal/queue"
	"github.com/agent-i/agent/internal/security"
	"github.com/agent-i/agent/internal/selftelemetry"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type sampleReader struct{ calls atomic.Int32 }

func (r *sampleReader) Sample(context.Context) (platform.Snapshot, error) {
	r.calls.Add(1)
	return platform.Snapshot{ObservedAt: time.Now(), Values: []platform.Measurement{{Name: "host.cpu.used_pct", Unit: "%", Kind: "gauge", Value: 12}}}, nil
}
func TestMetricsPipelineTLS(t *testing.T) {
	received := make(chan struct{}, 4)
	t.Setenv("TEST_PIPELINE_AUTH", "ApiKey fixture-only")
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		md, e := (&pmetric.JSONUnmarshaler{}).UnmarshalMetrics(b)
		if e != nil || md.DataPointCount() == 0 {
			t.Error("invalid OTLP")
		}
		if strings.Contains(string(b), "fixture-only") {
			t.Error("secret in queued payload")
		}
		attrs := md.ResourceMetrics().At(0).Resource().Attributes()
		host, _ := attrs.Get("host.id")
		if host.Str() != "vm-01" {
			t.Error("identity changed")
		}
		if r.URL.Path != "/api/v1/otlp/v1/metrics" {
			t.Error("non-metric export")
		}
		w.WriteHeader(201)
		w.Write([]byte(`{"partialSuccess":{"rejectedDataPoints":0}}`))
		select {
		case received <- struct{}{}:
		default:
		}
	}))
	defer srv.Close()
	cfg, e := config.Parse(strings.NewReader(`{"exporter":{"type":"otlp_http","endpoint":"` + srv.URL + `/api/v1/otlp","headers_env":{"Authorization":"TEST_PIPELINE_AUTH"}},"policy":{"version":1,"enabled":{"metrics":true,"logs":false,"traces":false}}}`))
	if e != nil {
		t.Fatal(e)
	}
	stats := &selftelemetry.Counters{}
	reader := &sampleReader{}
	m := NewMetrics(cfg, stats)
	m.reader = reader
	m.queue, e = queue.OpenDisk(t.TempDir()+"/spool", "test-host", 1<<20, 4)
	if e != nil {
		t.Fatal(e)
	}
	m.resource = map[string]string{"host.id": "vm-01", "host.name": "vm-01", "service.name": "vm-01", "telemetry.distro.name": "agent-i"}
	m.sender = exporter.NewSender(cfg, security.Environment{}, stats, srv.Client())
	if e = m.launch(context.Background(), 10*time.Millisecond); e != nil {
		t.Fatal(e)
	}
	select {
	case <-received:
	case <-time.After(3 * time.Second):
		t.Fatal("no metrics exported")
	}
	// Receipt by the TLS server precedes the client's successful acknowledgement.
	deadline := time.Now().Add(3 * time.Second)
	for stats.BatchesAccepted.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if e = m.Stop(ctx); e != nil {
		t.Fatal(e)
	}
	calls := reader.calls.Load()
	time.Sleep(25 * time.Millisecond)
	if reader.calls.Load() != calls {
		t.Fatal("collector reads after stop")
	}
	if stats.BatchesAccepted.Load() == 0 {
		t.Fatal("acceptance not recorded")
	}
}
func TestDisabledMetricsStartHasNoIO(t *testing.T) {
	m := NewMetrics(config.Config{}, &selftelemetry.Counters{})
	if m.Start(context.Background()) == nil {
		t.Fatal("disabled metrics started")
	}
	if m.reader != nil || m.queue != nil || m.sender != nil {
		t.Fatal("disabled capability constructed resources")
	}
}

// AGENTV1 FILE END
