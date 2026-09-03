// AGENTV1 FILE START: OS contracts, no platform-specific imports.
package platform

import (
	"context"
	"errors"
	"os"
	"time"
)

var ErrUnsupported = errors.New("platform capability not implemented")

type Layout struct{ Config, Secrets, State, Logs string }
type HostMetrics interface {
	Sample(context.Context) (Snapshot, error)
}
type Snapshot struct {
	// AGENTV1 START: kernel boot origin for cumulative host counters
	StartTime time.Time
	// AGENTV1 END: kernel boot origin for cumulative host counters
	ObservedAt time.Time
	Values     []Measurement
	Issues     []Issue
}
type Measurement struct {
	Name, Unit, Kind string
	Value            float64
	Attributes       map[string]string
}
type Issue struct{ Capability, Code string }
type Permissions interface {
	Check(context.Context, string, bool) error
}
type FileIdentity interface {
	ID(*os.File) (string, error)
}
type StateStore interface {
	Read(context.Context, string) ([]byte, error)
	Replace(context.Context, string, []byte) error
}
type Service interface {
	Run(context.Context, func(context.Context) error) error
}
type Native struct{}

// AGENTV1 FILE END
