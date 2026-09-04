//go:build !linux

// AGENTV1 FILE START: non-Linux compile-safe Logs spool boundary.
package queue

import (
	"context"
	"errors"
)

type LogDisk struct{}

func OpenLogDisk(string, Scope, int64, int) (*LogDisk, error) {
	return nil, errors.New("Linux file logs are unsupported on this platform")
}
func (*LogDisk) PutAdmission(context.Context, string, []byte) (Receipt, bool, error) {
	return "", false, errors.New("Linux file logs are unsupported on this platform")
}
func (*LogDisk) Activate(context.Context, string) error {
	return errors.New("Linux file logs are unsupported on this platform")
}
func (*LogDisk) Next(context.Context) (Item, error) {
	return Item{}, errors.New("Linux file logs are unsupported on this platform")
}
func (*LogDisk) Ack(context.Context, Receipt) error {
	return errors.New("Linux file logs are unsupported on this platform")
}
func (*LogDisk) Close(context.Context) error { return nil }
func (*LogDisk) Corrupt() uint64             { return 0 }

// AGENTV1 FILE END
