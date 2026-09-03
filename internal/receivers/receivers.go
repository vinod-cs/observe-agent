// AGENTV1 FILE START: future standard OTel receiver boundary, no registered listeners.
package receivers

import "context"

type Receiver interface {
	Start(context.Context) error
	Shutdown(context.Context) error
}

// AGENTV1 FILE END
