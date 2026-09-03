//go:build darwin

// AGENTV1 FILE START: Darwin compile contract, not launchd implementation.
package platform

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

func Defaults() Layout {
	return Layout{"/Library/Application Support/Observe/Agent/agent.json", "/Library/Application Support/Observe/Agent/env", "/Library/Application Support/Observe/Agent/state", "/Library/Logs/Observe/Agent"}
}
func (Native) MachineID(context.Context) (string, error)     { return "", ErrUnsupported }
func (Native) ID(*os.File) (string, error)                   { return "", ErrUnsupported }
func (Native) Sample(context.Context) (Snapshot, error)      { return Snapshot{}, ErrUnsupported }
func (Native) Check(context.Context, string, bool) error     { return ErrUnsupported }
func (Native) Read(context.Context, string) ([]byte, error)  { return nil, ErrUnsupported }
func (Native) Replace(context.Context, string, []byte) error { return ErrUnsupported }
func (Native) Run(ctx context.Context, run func(context.Context) error) error {
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()
	return run(ctx)
}

// AGENTV1 FILE END
