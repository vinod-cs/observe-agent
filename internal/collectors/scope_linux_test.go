//go:build linux

// AGENTV1 FILE START: full startup + migrated backlog -> new TLS endpoint using rotated key.
package collectors

import (
	"context"
	"encoding/json"
	"github.com/agent-i/agent/internal/config"
	"github.com/agent-i/agent/internal/queue"
	"github.com/agent-i/agent/internal/selftelemetry"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEndpointAndKeyRotationReplay(t *testing.T) {
	payload := `{"resourceMetrics":[]}`
	seen := make(chan string, 16)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if string(b) == payload {
			seen <- r.Header.Get("Authorization")
		}
		w.WriteHeader(201)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	second := httptest.NewTLSServer(srv.Config.Handler)
	defer second.Close()
	old := "https://old.example.test/api/v1/otlp"
	host := "i-0345d461c99a6da2f"
	dir := t.TempDir() + "/spool"
	q, e := queue.OpenDisk(dir, old+"\x00"+host+"\x00"+"127696279140"+"\x00us-east-2", 1<<20, 64)
	if e != nil {
		t.Fatal(e)
	}
	q.Put(context.Background(), []byte(payload))
	q.Close(context.Background())
	c, e := config.Parse(strings.NewReader(`{"exporter":{"backend_id":"backend-a","organization_id":"org-a","type":"otlp_http","endpoint":"` + srv.URL + `/api/v1/otlp","headers_env":{"Authorization":"ROTATION_TEST_KEY"}},"policy":{"version":1,"enabled":{"metrics":true}},"ec2_metadata":{"enabled":true}}`))
	if e != nil {
		t.Fatal(e)
	}
	c.Delivery.StateDirectory = dir
	c.Exporter.PreviousEndpoint = old
	for i, key := range []string{"ApiKey rotated-key-one", "ApiKey rotated-key-one", "ApiKey rotated-key-two"} {
		client := srv.Client()
		if i > 0 {
			c.Exporter.Endpoint = second.URL + "/api/v1/otlp"
			client = second.Client()
		}
		t.Setenv("ROTATION_TEST_KEY", key)
		m := NewMetrics(c, &selftelemetry.Counters{})
		m.detector = nativeIdentityFixture{}
		m.httpClient = client
		if e = m.Start(context.Background()); e != nil {
			t.Fatal(e)
		}
		awaitCounter(t, func() bool { return len(seen) > 0 && m.stats.DeliveredRecords.Load() > 0 })
		if e = m.Stop(context.Background()); e != nil {
			t.Fatal(e)
		}
		select {
		case got := <-seen:
			if got != key {
				t.Fatal("key not rotated")
			}
		default:
			t.Fatal("backlog not replayed")
		}
		c.Exporter.PreviousEndpoint = ""
		q, e = queue.OpenScopedDisk(dir, queue.Scope{BackendID: "backend-a", OrganizationID: "org-a", HostID: host, Account: "127696279140", Region: "us-east-2"}, "", 1<<20, 64)
		if e != nil {
			t.Fatal(e)
		}
		q.Put(context.Background(), []byte(payload))
		q.Close(context.Background())
	}
	b, _ := os.ReadFile(filepath.Join(dir, "manifest.json"))
	var metadata map[string]any
	if json.Unmarshal(b, &metadata) != nil || metadata["Version"] != float64(2) {
		t.Fatal("not v2")
	}
	for _, forbidden := range []string{old, srv.URL, second.URL, "rotated-key"} {
		if strings.Contains(string(b), forbidden) {
			t.Fatal("transport/credentials leaked to manifest")
		}
	}
}

// AGENTV1 FILE END
