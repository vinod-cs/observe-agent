// AGENTV1 FILE START: tenant binding, remote opt-in and verifier boundary tests.
package fleet

import (
	"context"
	"errors"
	"github.com/agent-i/agent/internal/policy"
	"testing"
	"time"
)

type verifier struct {
	value Envelope
	err   error
}

func (v verifier) Verify(context.Context, []byte) (Envelope, error) { return v.value, v.err }

type applier struct{ calls int }

func (a *applier) Apply(context.Context, policy.Document) error { a.calls++; return nil }
func TestGate(t *testing.T) {
	now := time.Unix(1000, 0)
	good := Envelope{"org-a", "host-a", now.Add(time.Minute), policy.Document{Version: 1, Enabled: map[policy.Capability]bool{policy.Metrics: true}}}
	for _, test := range []struct {
		name    string
		edit    func(*Envelope)
		err     error
		allowed []policy.Capability
		pass    bool
	}{
		{"valid", func(*Envelope) {}, nil, []policy.Capability{policy.Metrics}, true},
		{"other org", func(e *Envelope) { e.OrganizationID = "org-b" }, nil, []policy.Capability{policy.Metrics}, false},
		{"other installation", func(e *Envelope) { e.InstallationID = "host-b" }, nil, []policy.Capability{policy.Metrics}, false},
		{"expired", func(e *Envelope) { e.ExpiresAt = now }, nil, []policy.Capability{policy.Metrics}, false},
		{"untrusted", func(*Envelope) {}, errors.New("secret must not appear"), []policy.Capability{policy.Metrics}, false},
		{"no local opt-in", func(*Envelope) {}, nil, nil, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			e := good
			test.edit(&e)
			a := &applier{}
			g := NewGate("org-a", "host-a", test.allowed, verifier{e, test.err}, a, func() time.Time { return now })
			err := g.Accept(context.Background(), []byte("authenticated transport fixture"))
			if (err == nil) != test.pass {
				t.Fatalf("result %v", err)
			}
			if !test.pass && a.calls != 0 {
				t.Fatal("invalid policy applied")
			}
		})
	}
}
func TestNoVerifierAndOversize(t *testing.T) {
	a := &applier{}
	g := NewGate("a", "b", nil, nil, a, nil)
	if g.Accept(context.Background(), []byte("x")) == nil {
		t.Fatal("missing verifier accepted")
	}
	if g.Accept(context.Background(), make([]byte, MaxPolicyBytes+1)) == nil {
		t.Fatal("oversize accepted")
	}
}

type files struct{ data []byte }

func (f *files) Read(context.Context, string) ([]byte, error) { return f.data, nil }
func (f *files) Replace(_ context.Context, _ string, b []byte) error {
	f.data = append([]byte(nil), b...)
	return nil
}
func TestLKG(t *testing.T) {
	s := LocalStore{Files: &files{}, Path: "fixed-local-policy.json"}
	p := policy.Document{Version: 7, Enabled: map[policy.Capability]bool{policy.Metrics: true}}
	if e := s.Save(context.Background(), p); e != nil {
		t.Fatal(e)
	}
	got, e := s.Load(context.Background())
	if e != nil || !got.Equal(p) {
		t.Fatal("LKG mismatch")
	}
}

// AGENTV1 FILE END
