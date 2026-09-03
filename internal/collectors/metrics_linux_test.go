//go:build linux

// AGENTV1 FILE START: native real OS startup to localhost TLS only.
package collectors

import (
	"context"
	"github.com/agent-i/agent/internal/cloud"
	"github.com/agent-i/agent/internal/config"
	"github.com/agent-i/agent/internal/platform"
	"github.com/agent-i/agent/internal/selftelemetry"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNativeLinuxStartToTLS(t *testing.T) {
	machine, machineError := (platform.Native{}).MachineID(context.Background())
	if machineError != nil {
		machine = "i-0345d461c99a6da2f"
		t.Log("VM has no machine-id; identity comes from test-only EC2 detector fixture; OS counters remain real")
	}
	t.Setenv("TEST_NATIVE_AUTH", "ApiKey localhost-only-test")
	got := make(chan int, 4)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		md, e := (&pmetric.JSONUnmarshaler{}).UnmarshalMetrics(raw)
		if e != nil {
			t.Error(e)
			w.WriteHeader(422)
			return
		}
		host, _ := md.ResourceMetrics().At(0).Resource().Attributes().Get("host.id")
		if host.Str() != machine {
			t.Error("native stable identity lost")
		}
		if r.URL.Path != "/api/v1/otlp/v1/metrics" {
			t.Error("nonmetric endpoint used")
		}
		w.WriteHeader(201)
		w.Write([]byte(`{}`))
		select {
		case got <- md.DataPointCount():
		default:
		}
	}))
	defer server.Close()
	cfg, e := config.Parse(strings.NewReader(`{"exporter":{"type":"otlp_http","endpoint":"` + server.URL + `/api/v1/otlp","headers_env":{"Authorization":"TEST_NATIVE_AUTH"}},"policy":{"version":1,"enabled":{"metrics":true,"logs":false,"traces":false}},"ec2_metadata":{"enabled":false}}`))
	if e != nil {
		t.Fatal(e)
	}
	counters := &selftelemetry.Counters{}
	// AGENTV1 START: never write installed state during a native test.
	cfg.Exporter.BackendID = "fixture-backend"
	cfg.Exporter.OrganizationID = "fixture-org"
	cfg.Delivery.StateDirectory = t.TempDir() + "/spool"
	// AGENTV1 END: isolated durable queue
	m := NewMetrics(cfg, counters)
	m.httpClient = server.Client()
	if machineError != nil {
		on := true
		m.cfg.EC2.Enabled = &on
		m.detector = nativeIdentityFixture{}
	}
	if e = m.Start(context.Background()); e != nil {
		t.Fatal(e)
	}
	defer m.Stop(context.Background())
	select {
	case count := <-got:
		if count < 1 {
			t.Fatal("empty OS telemetry")
		}
		t.Logf("native OS -> pdata -> localhost TLS: %d points", count)
	case <-time.After(5 * time.Second):
		t.Fatal("no native export")
	}
	deadline := time.Now().Add(3 * time.Second)
	for counters.BatchesAccepted.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if e = m.Stop(ctx); e != nil {
		t.Fatal(e)
	}
	if counters.BatchesAccepted.Load() == 0 {
		t.Fatal("no accepted native batch")
	}
}

type nativeIdentityFixture struct{}

func (nativeIdentityFixture) Detect(context.Context) (cloud.Evidence, error) {
	return cloud.Evidence{Verified: true, Provider: "aws", Platform: "aws_ec2", Account: "127696279140", Region: "us-east-2", InstanceID: "i-0345d461c99a6da2f"}, nil
}

// AGENTV1 FILE END
