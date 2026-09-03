//go:build linux

// AGENTV1 FILE START: native Linux OS-read smoke and counter math.
package platform

import (
	"context"
	"github.com/agent-i/agent/internal/config"
	"testing"
	"time"
)

func TestCPUCounterSemantics(t *testing.T) {
	before := []float64{10, 0, 10, 70, 10, 0, 0, 0}
	after := []float64{20, 0, 20, 80, 20, 0, 0, 0}
	value, ok := CounterRatio(before, after, time.Second)
	if !ok || value != 50 {
		t.Fatalf("CPU ratio %v %v", value, ok)
	}
	for _, gap := range []time.Duration{0, -time.Second, 121 * time.Second} {
		if _, ok = CounterRatio(before, after, gap); ok {
			t.Fatal("invalid gap")
		}
	}
	if _, ok = CounterRatio(after, before, time.Second); ok {
		t.Fatal("reset")
	}
	if _, ok = CounterRatio(before, before, time.Second); ok {
		t.Fatal("zero elapsed counters")
	}
	if localFilesystem("nfs") || localFilesystem("cifs") || localFilesystem("fuse.sshfs") {
		t.Fatal("remote filesystem touched")
	}
}
func TestLiveLinuxMetrics(t *testing.T) {
	limits, _, _ := (config.Config{}).Runtime()
	reader, e := NewHostMetrics(limits)
	if e != nil {
		t.Fatal(e)
	}
	sample, e := reader.Sample(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	found := map[string]bool{}
	for _, m := range sample.Values {
		found[m.Name] = true
		if m.Kind == "sum" && sample.StartTime.IsZero() {
			t.Fatal("counter has no origin")
		}
	}
	for _, name := range []string{"system.cpu.time", "system.memory.usage", "system.network.io", "system.cpu.load_average.1m"} {
		if !found[name] {
			t.Fatalf("missing real Linux signal %s; issues=%v", name, sample.Issues)
		}
	}
	if len(sample.Values) > limits.MaxPoints {
		t.Fatal("sample cap")
	}
	t.Logf("real Linux snapshot: %d points, issues=%v; disk=%v filesystem=%v", len(sample.Values), sample.Issues, found["system.disk.io"], found["system.filesystem.usage"])
}

// AGENTV1 FILE END
