// AGENTV1 FILE START: future OTel pipeline staging, no custom wire codec.
package pipeline

import (
	"context"
	"github.com/agent-i/agent/internal/policy"
)

type Builder interface {
	Validate(policy.Document) error
	Prepare(context.Context, policy.Document) (Prepared, error)
}
type Prepared interface {
	Start(context.Context) error
	Stop(context.Context) error
}

// AGENTV1 FILE END
