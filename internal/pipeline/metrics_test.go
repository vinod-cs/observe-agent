// AGENTV1 FILE START: standard pdata JSON and exact current-backend host identity fixtures.
package pipeline

import (
	"context"
	"encoding/json"
	"github.com/agent-i/agent/internal/cloud"
	"github.com/agent-i/agent/internal/identity"
	"github.com/agent-i/agent/internal/platform"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"os"
	"testing"
	"time"
)

func TestMetricsContractAndBounds(t *testing.T) {
	id, e := identity.Resolve(context.Background(), "", cloud.Evidence{Verified: true, Provider: "aws", Platform: "aws_ec2", Account: "127696279140", Region: "us-east-2", InstanceID: "i-0345d461c99a6da2f", ResourceID: "arn:aws:ec2:us-east-2:127696279140:instance/i-0345d461c99a6da2f"}, nil)
	if e != nil {
		t.Fatal(e)
	}
	resource := id.Resource("linux", "amd64", "slice-test", "")
	s := platform.Snapshot{ObservedAt: time.Now().UTC(), StartTime: time.Now().Add(-time.Hour)}
	for i := 0; i < 2001; i++ {
		s.Values = append(s.Values, platform.Measurement{Name: "system.disk.io", Kind: "sum", Unit: "By", Value: float64(i), Attributes: map[string]string{"device": "nvme0n1", "direction": "read"}})
	}
	batches, e := Batches(s, resource, 500, 65536)
	if e != nil {
		t.Fatal(e)
	}
	points := 0
	for _, raw := range batches {
		if len(raw) > 65536 {
			t.Fatal("oversize")
		}
		md, e := (&pmetric.JSONUnmarshaler{}).UnmarshalMetrics(raw)
		if e != nil {
			t.Fatal(e)
		}
		points += md.DataPointCount()
		if md.DataPointCount() > 500 {
			t.Fatal("backend point cap exceeded")
		}
		attrs := md.ResourceMetrics().At(0).Resource().Attributes()
		host, _ := attrs.Get("host.id")
		provider, _ := attrs.Get("cloud.provider")
		if host.Str() != "i-0345d461c99a6da2f" || provider.Str() != "aws" {
			t.Fatal("canonical identity lost")
		}
		metric := md.ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics().At(0)
		if metric.Type() != pmetric.MetricTypeSum || !metric.Sum().IsMonotonic() || metric.Sum().AggregationTemporality() != pmetric.AggregationTemporalityCumulative {
			t.Fatal("counter semantics lost")
		}
	}
	if points != 2001 {
		t.Fatal("points dropped")
	}
	if out := os.Getenv("AGENT_CONTRACT_OUTPUT"); out != "" {
		// Explicit local test artifact only; never used by the installed runtime.
		fixture, _ := json.Marshal(batchesToJSON(batches))
		if e = os.WriteFile(out, fixture, 0600); e != nil {
			t.Fatal(e)
		}
	}
}
func batchesToJSON(batches [][]byte) []json.RawMessage {
	out := make([]json.RawMessage, len(batches))
	for i, b := range batches {
		out[i] = b
	}
	return out
}

// AGENTV1 FILE END
