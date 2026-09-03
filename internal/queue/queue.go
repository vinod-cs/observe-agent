// AGENTV1 FILE START: durable ack boundary implemented by the private Linux disk spool.
package queue

import "context"

type Receipt string
type Item struct {
	Receipt Receipt
	Data    []byte
}

// Put succeeds only after durable acceptance; Ack follows downstream acceptance.
type Durable interface {
	Put(context.Context, []byte) (Receipt, error)
	Next(context.Context) (Item, error)
	Ack(context.Context, Receipt) error
	Close(context.Context) error
}

// AGENTV1 FILE END
