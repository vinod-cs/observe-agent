// AGENTV1 FILE START: all native adapter sets satisfy the portable contracts.
package platform

import (
	"context"
	"errors"
	"testing"
)

var _ FileIdentity = Native{}
var _ StateStore = Native{}
var _ HostMetrics = Native{}
var _ Permissions = Native{}
var _ Service = Native{}

func TestNoFakeSamples(t *testing.T) {
	s, e := (Native{}).Sample(context.Background())
	if !errors.Is(e, ErrUnsupported) || len(s.Values) != 0 {
		t.Fatal("placeholder fabricated telemetry")
	}
}
func TestCancelledLifecycle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	e := (Native{}).Run(ctx, func(ctx context.Context) error { return ctx.Err() })
	if e == nil {
		t.Fatal("cancellation swallowed")
	}
}

// AGENTV1 FILE END
