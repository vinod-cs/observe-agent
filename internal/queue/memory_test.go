// AGENTV1 FILE START: queue byte/item saturation, ownership and cancellation.
package queue

import (
	"context"
	"errors"
	"testing"
)

func TestBoundedMemory(t *testing.T) {
	q := NewMemory(6, 2)
	b := []byte("abc")
	if e := q.Push(b); e != nil {
		t.Fatal(e)
	}
	b[0] = 'x'
	if e := q.Push([]byte("def")); e != nil {
		t.Fatal(e)
	}
	if !errors.Is(q.Push([]byte("z")), ErrFull) {
		t.Fatal("queue saturation not enforced")
	}
	n, size := q.Size()
	if n != 2 || size != 6 {
		t.Fatal("bad accounting")
	}
	v, e := q.Pop(context.Background())
	if e != nil || string(v) != "abc" {
		t.Fatal("queue payload mutated")
	}
	q.Close()
	if !errors.Is(q.Push([]byte("a")), ErrClosed) {
		t.Fatal("closed queue accepts")
	}
	if _, e = q.Pop(context.Background()); e != nil {
		t.Fatal(e)
	}
	if _, e = q.Pop(context.Background()); !errors.Is(e, ErrClosed) {
		t.Fatal("closed empty queue blocked")
	}
	c, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e = NewMemory(1, 1).Pop(c); e == nil {
		t.Fatal("cancel ignored")
	}
}

// AGENTV1 FILE END
