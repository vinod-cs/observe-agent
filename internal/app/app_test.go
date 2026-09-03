// AGENTV1 FILE START: behavior tests for enable/disable, rollback and LKG failures.
package app

import (
	"context"
	"errors"
	"github.com/agent-i/agent/internal/collectors"
	"github.com/agent-i/agent/internal/policy"
	"sync"
	"testing"
	"time"
)

type fake struct {
	starts, stops       int
	failStart, failStop bool
	ctx                 context.Context
}

func (f *fake) Start(ctx context.Context) error {
	f.starts++
	f.ctx = ctx
	if f.failStart {
		return errors.New("sensitive underlying error")
	}
	return nil
}
func (f *fake) Stop(context.Context) error {
	f.stops++
	if f.failStop {
		return errors.New("stop failed")
	}
	return nil
}

type store struct {
	saved policy.Document
	fail  bool
}

func (s *store) Save(_ context.Context, p policy.Document) error {
	if s.fail {
		return errors.New("disk")
	}
	s.saved = p.Clone()
	return nil
}
func doc(v uint64, caps ...policy.Capability) policy.Document {
	p := policy.Document{Version: v, Enabled: map[policy.Capability]bool{}}
	for _, c := range caps {
		p.Enabled[c] = true
	}
	return p
}
func registry(fakes map[policy.Capability]*fake, calls map[policy.Capability]int) collectors.Registry {
	r := collectors.Registry{}
	for c, f := range fakes {
		r[c] = collectors.Registration{Descriptor: policy.Descriptor{Capability: c, Implemented: true}, New: func() (collectors.Collector, error) { calls[c]++; return f, nil }}
	}
	return r
}
func TestSelectiveLifecycle(t *testing.T) {
	ctx := context.Background()
	metrics, logs, traces := &fake{}, &fake{}, &fake{}
	calls := map[policy.Capability]int{}
	s := &store{}
	m := New(ctx, registry(map[policy.Capability]*fake{policy.Metrics: metrics, policy.Logs: logs, policy.Traces: traces}, calls), s, nil, time.Second)
	defer m.Close()
	if e := m.Apply(ctx, doc(1, policy.Metrics)); e != nil {
		t.Fatal(e)
	}
	if calls[policy.Logs] != 0 || calls[policy.Traces] != 0 {
		t.Fatal("disabled collector constructed")
	}
	if e := m.Apply(ctx, doc(2, policy.Metrics, policy.Traces)); e != nil {
		t.Fatal(e)
	}
	if metrics.starts != 1 || traces.starts != 1 {
		t.Fatal("unrelated metrics restarted")
	}
	if e := m.Apply(ctx, doc(3, policy.Metrics)); e != nil {
		t.Fatal(e)
	}
	if traces.stops != 1 || traces.ctx.Err() == nil {
		t.Fatal("trace listener lifetime not cancelled")
	}
	if e := m.Apply(ctx, doc(3, policy.Metrics)); e != nil {
		t.Fatal("identical policy not idempotent")
	}
	if e := m.Apply(ctx, doc(2, policy.Metrics)); e == nil {
		t.Fatal("replayed policy")
	}
	if e := m.Apply(ctx, doc(3, policy.Logs)); e == nil {
		t.Fatal("same version mutation")
	}
	copy := m.Current()
	copy.Enabled[policy.Logs] = true
	if m.Current().Enabled[policy.Logs] {
		t.Fatal("policy map leaked")
	}
	if s.saved.Version != 3 {
		t.Fatal("LKG missing")
	}
}
func TestRollback(t *testing.T) {
	ctx := context.Background()
	a, b := &fake{}, &fake{failStart: true}
	m := New(ctx, registry(map[policy.Capability]*fake{policy.Metrics: a, policy.Traces: b}, map[policy.Capability]int{}), nil, nil, time.Second)
	defer m.Close()
	if e := m.Apply(ctx, doc(1, policy.Metrics)); e != nil {
		t.Fatal(e)
	}
	if e := m.Apply(ctx, doc(2, policy.Traces)); e == nil {
		t.Fatal("bad start accepted")
	}
	if m.Current().Version != 1 || a.starts != 2 || a.ctx.Err() != nil || b.stops != 1 {
		t.Fatal("rollback did not restore old running set")
	}
}
func TestPersistenceFailureIsDegraded(t *testing.T) {
	ctx := context.Background()
	s := &store{}
	m := New(ctx, collectors.Registry{}, s, nil, time.Second)
	if e := m.Apply(ctx, doc(1)); e != nil {
		t.Fatal(e)
	}
	s.fail = true
	if e := m.Apply(ctx, doc(2)); e == nil || !m.Degraded() || m.Current().Version != 1 {
		t.Fatal("ambiguous commit claimed successful")
	}
	if e := m.Apply(ctx, doc(3)); e == nil {
		t.Fatal("degraded manager accepted update")
	}
}
func TestUnsupportedHasNoSideEffects(t *testing.T) {
	calls := map[policy.Capability]int{}
	m := New(context.Background(), registry(map[policy.Capability]*fake{policy.Metrics: {}}, calls), nil, nil, time.Second)
	if m.Apply(context.Background(), doc(1, policy.Metrics, policy.Logs)) == nil {
		t.Fatal("unimplemented logs accepted")
	}
	if len(calls) != 0 {
		t.Fatal("preflight started metrics")
	}
}
func TestConcurrentIdempotency(t *testing.T) {
	f := &fake{}
	calls := map[policy.Capability]int{}
	m := New(context.Background(), registry(map[policy.Capability]*fake{policy.Metrics: f}, calls), nil, nil, time.Second)
	defer m.Close()
	var wg sync.WaitGroup
	for range 30 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if e := m.Apply(context.Background(), doc(1, policy.Metrics)); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	if f.starts != 1 {
		t.Fatal("concurrent double start")
	}
}
func TestStopFailureIsNotHealthy(t *testing.T) {
	f := &fake{}
	m := New(context.Background(), registry(map[policy.Capability]*fake{policy.Metrics: f}, map[policy.Capability]int{}), nil, nil, time.Second)
	if e := m.Apply(context.Background(), doc(1, policy.Metrics)); e != nil {
		t.Fatal(e)
	}
	f.failStop = true
	if m.Apply(context.Background(), doc(2)) == nil || !m.Degraded() {
		t.Fatal("failed stop called healthy")
	}
	f.failStop = false
	m.Close()
}

func TestCancelledApplyHasNoSideEffects(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := map[policy.Capability]int{}
	m := New(context.Background(), registry(map[policy.Capability]*fake{policy.Metrics: {}}, calls), nil, nil, time.Second)
	if m.Apply(ctx, doc(1, policy.Metrics)) == nil || len(calls) != 0 {
		t.Fatal("cancelled operation started a collector")
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if m.Apply(context.Background(), doc(1, policy.Metrics)) == nil {
		t.Fatal("closed manager accepted policy")
	}
}

// AGENTV1 FILE END
