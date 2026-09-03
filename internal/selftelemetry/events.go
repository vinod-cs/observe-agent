// AGENTV1 FILE START: structured audit context without arbitrary payloads or credentials.
package selftelemetry

import "github.com/agent-i/agent/internal/policy"

type Event struct {
	Code       string
	Version    uint64
	Capability policy.Capability
}
type Sink interface{ Record(Event) }
type Discard struct{}

func (Discard) Record(Event) {}

// AGENTV1 FILE END
