// AGENTV1 FILE START: lifecycle registration; constructors must have no host side effects.
package collectors

import (
	"context"
	"github.com/agent-i/agent/internal/policy"
)

type Collector interface {
	Start(context.Context) error
	// Stop joins workers/closes files/listeners, and is safe after a partial Start.
	Stop(context.Context) error
}
type Factory func() (Collector, error)
type Registration struct {
	Descriptor policy.Descriptor
	New        Factory
}
type Registry map[policy.Capability]Registration

// No implemented collectors ship in foundation. Enabling one explicitly fails.
func Foundation() Registry { return Registry{} }

// AGENTV1 FILE END
