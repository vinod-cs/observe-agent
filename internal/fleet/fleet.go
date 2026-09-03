// AGENTV1 FILE START: future authenticated remote-policy contract; no invented HTTP endpoint.
package fleet

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/agent-i/agent/internal/policy"
	"sync"
	"time"
)

const MaxPolicyBytes = 64 << 10

type Envelope struct {
	OrganizationID string          `json:"organization_id"`
	InstallationID string          `json:"installation_id"`
	ExpiresAt      time.Time       `json:"expires_at"`
	Policy         policy.Document `json:"policy"`
}

// Verify authenticates exact received bytes before parsing, with signature or authenticated
// session binding. No verifier ships until the backend contract/security design is approved.
type Verifier interface {
	Verify(context.Context, []byte) (Envelope, error)
}
type Transport interface {
	Fetch(context.Context, uint64) ([]byte, error)
}
type Applier interface {
	Apply(context.Context, policy.Document) error
}
type Gate struct {
	mu                         sync.Mutex
	organization, installation string
	allowed                    map[policy.Capability]bool
	verifier                   Verifier
	apply                      Applier
	clock                      func() time.Time
}

func NewGate(org, installation string, allowed []policy.Capability, v Verifier, a Applier, clock func() time.Time) *Gate {
	m := map[policy.Capability]bool{}
	for _, c := range allowed {
		m[c] = true
	}
	if clock == nil {
		clock = time.Now
	}
	return &Gate{organization: org, installation: installation, allowed: m, verifier: v, apply: a, clock: clock}
}
func (g *Gate) Accept(ctx context.Context, raw []byte) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(raw) == 0 || len(raw) > MaxPolicyBytes || g.verifier == nil || g.apply == nil || g.organization == "" || g.installation == "" {
		return errors.New("remote policy unavailable")
	}
	e, err := g.verifier.Verify(ctx, append([]byte(nil), raw...))
	if err != nil {
		return errors.New("remote policy authentication failed")
	}
	if e.OrganizationID != g.organization || e.InstallationID != g.installation {
		return errors.New("remote policy scope mismatch")
	}
	if !e.ExpiresAt.After(g.clock()) {
		return errors.New("remote policy expired")
	}
	if err = e.Policy.Validate(); err != nil {
		return err
	}
	for c, on := range e.Policy.Enabled {
		if on && !g.allowed[c] {
			return errors.New("remote capability exceeds local allowance")
		}
	}
	return g.apply.Apply(ctx, e.Policy)
}

type StateFiles interface {
	Read(context.Context, string) ([]byte, error)
	Replace(context.Context, string, []byte) error
}
type LocalStore struct {
	Files StateFiles
	Path  string
}

func (s LocalStore) Save(ctx context.Context, p policy.Document) error {
	if s.Files == nil {
		return errors.New("state storage unavailable")
	}
	if err := p.Validate(); err != nil {
		return err
	}
	data, err := json.Marshal(p)
	if err != nil {
		return errors.New("policy encoding failed")
	}
	return s.Files.Replace(ctx, s.Path, data)
}
func (s LocalStore) Load(ctx context.Context) (policy.Document, error) {
	var p policy.Document
	if s.Files == nil {
		return p, errors.New("state storage unavailable")
	}
	raw, err := s.Files.Read(ctx, s.Path)
	if err != nil {
		return p, err
	}
	if len(raw) > MaxPolicyBytes {
		return p, errors.New("stored policy too large")
	}
	if json.Unmarshal(raw, &p) != nil {
		return p, errors.New("stored policy invalid")
	}
	return p, p.Validate()
}

// AGENTV1 FILE END
