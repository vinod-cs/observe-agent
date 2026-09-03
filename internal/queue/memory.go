// AGENTV1 FILE START: byte/item-bounded memory queue; deliberately not Durable.
package queue

import (
	"context"
	"errors"
	"sync"
)

var ErrFull = errors.New("metrics queue full")
var ErrClosed = errors.New("metrics queue closed")

type Memory struct {
	mu              sync.Mutex
	items           [][]byte
	bytes, maxBytes int64
	maxItems        int
	closed          bool
	wake            chan struct{}
}

func NewMemory(bytes int64, items int) *Memory {
	return &Memory{maxBytes: bytes, maxItems: items, wake: make(chan struct{}, 1)}
}
func (q *Memory) Push(b []byte) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return ErrClosed
	}
	if len(b) == 0 || int64(len(b)) > q.maxBytes-q.bytes || len(q.items) >= q.maxItems {
		return ErrFull
	}
	q.items = append(q.items, append([]byte(nil), b...))
	q.bytes += int64(len(b))
	select {
	case q.wake <- struct{}{}:
	default:
	}
	return nil
}
func (q *Memory) Pop(ctx context.Context) ([]byte, error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		q.mu.Lock()
		if len(q.items) > 0 {
			b := q.items[0]
			q.items[0] = nil
			q.items = q.items[1:]
			q.bytes -= int64(len(b))
			q.mu.Unlock()
			return b, nil
		}
		closed := q.closed
		q.mu.Unlock()
		if closed {
			return nil, ErrClosed
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-q.wake:
		}
	}
}
func (q *Memory) Size() (int, int64) { q.mu.Lock(); defer q.mu.Unlock(); return len(q.items), q.bytes }
func (q *Memory) Close() {
	q.mu.Lock()
	q.closed = true
	q.mu.Unlock()
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

// AGENTV1 FILE END
