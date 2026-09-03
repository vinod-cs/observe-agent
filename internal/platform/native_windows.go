//go:build windows

// AGENTV1 FILE START: fail-closed Windows placeholders, not SCM support.
package platform

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
)

// Advisory console paths only; future service installation must use trusted Known Folder APIs.
func Defaults() Layout {
	root := filepath.Join(os.Getenv("ProgramData"), "Observe", "Agent")
	return Layout{filepath.Join(root, "agent.json"), filepath.Join(root, "env"), filepath.Join(root, "state"), filepath.Join(root, "logs")}
}
func (Native) MachineID(context.Context) (string, error)     { return "", ErrUnsupported }
func (Native) ID(*os.File) (string, error)                   { return "", ErrUnsupported }
func (Native) Sample(context.Context) (Snapshot, error)      { return Snapshot{}, ErrUnsupported }
func (Native) Check(context.Context, string, bool) error     { return ErrUnsupported }
func (Native) Read(context.Context, string) ([]byte, error)  { return nil, ErrUnsupported }
func (Native) Replace(context.Context, string, []byte) error { return ErrUnsupported }
func (Native) Run(ctx context.Context, run func(context.Context) error) error {
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()
	return run(ctx)
}

// AGENTV1 FILE END
