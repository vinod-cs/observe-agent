// AGENTV1 FILE START: serialized capability transitions and explicit rollback/degraded state.
package app

import (
	"context"
	"errors"
	"fmt"
	"github.com/agent-i/agent/internal/collectors"
	"github.com/agent-i/agent/internal/policy"
	"github.com/agent-i/agent/internal/selftelemetry"
	"sync"
	"time"
)

type Store interface {
	Save(context.Context, policy.Document) error
}
type running struct {
	collector collectors.Collector
	cancel    context.CancelFunc
}
type Manager struct {
	mu               sync.Mutex
	root             context.Context
	registry         collectors.Registry
	active           map[policy.Capability]running
	current          policy.Document
	store            Store
	audit            selftelemetry.Sink
	timeout          time.Duration
	degraded, closed bool
}

func New(ctx context.Context, registry collectors.Registry, store Store, audit selftelemetry.Sink, timeout time.Duration) *Manager {
	if audit == nil {
		audit = selftelemetry.Discard{}
	}
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	r := collectors.Registry{}
	for k, v := range registry {
		r[k] = v
	}
	return &Manager{root: ctx, registry: r, active: map[policy.Capability]running{}, store: store, audit: audit, timeout: timeout}
}
func (m *Manager) Current() policy.Document {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.current.Clone()
}
func (m *Manager) Degraded() bool { m.mu.Lock(); defer m.mu.Unlock(); return m.degraded }
func (m *Manager) start(c policy.Capability) error {
	r := m.registry[c]
	instance, err := r.New()
	if err != nil || instance == nil {
		return errors.New("collector construction failed")
	}
	ctx, cancel := context.WithCancel(m.root)
	if err = instance.Start(ctx); err != nil {
		cancel()
		stopCtx, end := context.WithTimeout(context.Background(), m.timeout)
		defer end()
		if instance.Stop(stopCtx) != nil {
			m.degraded = true
		}
		return errors.New("collector start failed")
	}
	m.active[c] = running{instance, cancel}
	return nil
}
func (m *Manager) stop(c policy.Capability) error {
	r, ok := m.active[c]
	if !ok {
		return nil
	}
	r.cancel()
	ctx, cancel := context.WithTimeout(context.Background(), m.timeout)
	defer cancel()
	if r.collector.Stop(ctx) != nil {
		m.degraded = true
		return errors.New("collector did not stop cleanly")
	}
	delete(m.active, c)
	return nil
}
func (m *Manager) restore(old policy.Document) error {
	bad := false
	for _, c := range policy.All() {
		if !old.Enabled[c] {
			if m.stop(c) != nil {
				bad = true
			}
		}
	}
	for _, c := range policy.All() {
		if old.Enabled[c] {
			if _, ok := m.active[c]; !ok {
				if m.start(c) != nil {
					bad = true
				}
			}
		}
	}
	if bad {
		m.degraded = true
		return errors.New("policy rollback incomplete; service requires attention")
	}
	return nil
}
func (m *Manager) Apply(ctx context.Context, next policy.Document) error {
	next = next.Clone()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.degraded {
		return errors.New("lifecycle unavailable")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := m.root.Err(); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	if next.Equal(m.current) {
		return nil
	}
	if next.Version <= m.current.Version {
		return errors.New("policy version must advance")
	}
	// Validate every enabled registration before any constructor or I/O is invoked.
	for _, c := range policy.All() {
		if next.Enabled[c] {
			r, ok := m.registry[c]
			if !ok || !r.Descriptor.Implemented || r.New == nil || r.Descriptor.Capability != c {
				return fmt.Errorf("capability %s not implemented", c)
			}
		}
	}
	old := m.current.Clone()
	fail := func() error {
		rollback := m.restore(old)
		m.audit.Record(selftelemetry.Event{Code: "policy_apply_failed", Version: next.Version})
		if rollback != nil {
			return rollback
		}
		if m.degraded {
			return errors.New("policy failure requires service attention")
		}
		return errors.New("policy apply failed; prior policy retained")
	}
	for _, c := range policy.All() {
		if !next.Enabled[c] {
			if m.stop(c) != nil {
				return fail()
			}
		}
	}
	for _, c := range policy.All() {
		if next.Enabled[c] {
			if _, ok := m.active[c]; !ok {
				if m.start(c) != nil {
					return fail()
				}
			}
		}
	}
	if ctx.Err() != nil {
		return fail()
	}
	if m.store != nil {
		if m.store.Save(ctx, next) != nil {
			m.degraded = true
			return fail()
		}
	} // Persist outcome may be ambiguous after fsync; never claim committed LKG.
	m.current = next
	m.audit.Record(selftelemetry.Event{Code: "policy_applied", Version: next.Version})
	return nil
}
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	var result error
	for _, c := range policy.All() {
		if err := m.stop(c); err != nil {
			result = err
		}
	}
	return result
}

// AGENTV1 FILE END
