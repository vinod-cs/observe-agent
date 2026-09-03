//go:build !linux

// AGENTV1 FILE START: disk durability requires native OS implementation before enabling collection.
package queue

import (
	"context"
	"errors"
)

type Disk struct{}

// AGENTV1 START: no unsupported-OS durability fallback.
func OpenScopedDisk(string, Scope, string, int64, int) (*Disk, error) {
	return nil, errors.New("durable metrics spool supported on Linux only")
}

// AGENTV1 END: v2 compile surface

func OpenDisk(string, string, int64, int) (*Disk, error) {
	return nil, errors.New("durable metrics spool supported on Linux only")
}
func (*Disk) Put(context.Context, []byte) (Receipt, error) { return "", errors.New("unsupported") }
func (*Disk) Next(context.Context) (Item, error)           { return Item{}, errors.New("unsupported") }
func (*Disk) Ack(context.Context, Receipt) error           { return errors.New("unsupported") }
func (*Disk) Close(context.Context) error                  { return nil }
func (*Disk) Corrupt() uint64                              { return 0 }

// AGENTV1 FILE END
