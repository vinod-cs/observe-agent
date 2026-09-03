// AGENTV1 FILE START: closed, data-only capability policy; omitted means disabled.
package policy

import (
	"errors"
)

type Capability string

const (
	Metrics    Capability = "metrics"
	Logs       Capability = "logs"
	Traces     Capability = "traces"
	Processes  Capability = "processes"
	Containers Capability = "containers"
)

func All() []Capability { return []Capability{Metrics, Logs, Traces, Processes, Containers} }
func Known(c Capability) bool {
	for _, x := range All() {
		if c == x {
			return true
		}
	}
	return false
}

// Document cannot carry commands, URLs, executable names, secrets or filesystem paths.
// Remote policies can only activate capabilities explicitly permitted by local policy.
type Document struct {
	Version uint64              `json:"version"`
	Enabled map[Capability]bool `json:"enabled"`
}

func (p Document) Validate() error {
	if p.Version == 0 {
		return errors.New("policy version must be positive")
	}
	for c := range p.Enabled {
		if !Known(c) {
			return errors.New("unknown capability")
		}
	}
	return nil
}
func (p Document) Clone() Document {
	out := Document{Version: p.Version, Enabled: map[Capability]bool{}}
	for k, v := range p.Enabled {
		out.Enabled[k] = v
	}
	return out
}
func (p Document) Equal(other Document) bool {
	if p.Version != other.Version {
		return false
	}
	for _, c := range All() {
		if p.Enabled[c] != other.Enabled[c] {
			return false
		}
	}
	return true
}

type Access struct {
	Resource  string
	Privilege string
	Writes    bool
}
type Descriptor struct {
	Capability  Capability
	Access      []Access
	Implemented bool
}

// Catalogue is a declaration, not registration: none of these receivers ship in foundation.
func Catalogue() []Descriptor {
	return []Descriptor{
		{Metrics, []Access{{"OS CPU/memory/network/disk counters", "restricted service account where supported", false}}, false},
		{Logs, []Access{{"locally approved log paths", "read ACL on each configured path", false}}, false},
		{Traces, []Access{{"locally approved loopback OTLP listener", "bind configured non-privileged port", false}}, false},
		{Processes, []Access{{"process metadata/counters", "OS- and process-owner-specific read access", false}}, false},
		{Containers, []Access{{"explicitly approved container endpoint", "socket access may imply host control; opt-in required", false}}, false},
	}
}

// AGENTV1 FILE END
