//go:build linux

// AGENTV1 FILE START: Linux primitives only, no collectors or installer actions.
package platform

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
)

func Defaults() Layout {
	// AGENTV1 START: new package paths never default to the deployed legacy Agent.
	return Layout{"/etc/observe-agent/agent.yaml", "/etc/observe-agent/env", "/var/lib/observe-agent", ""}
	// AGENTV1 END: isolated package defaults
}
func (Native) MachineID(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	f, err := os.Open("/etc/machine-id")
	if err != nil {
		return "", errors.New("machine identity unavailable")
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 257))
	if err != nil || len(b) > 256 {
		return "", errors.New("machine identity invalid")
	}
	s := strings.TrimSpace(string(b))
	if len(s) != 32 || strings.Trim(s, "0123456789abcdef") != "" {
		return "", errors.New("stable machine identity unavailable")
	}
	return s, nil
}
func (Native) ID(f *os.File) (string, error) {
	s, err := f.Stat()
	if err != nil {
		return "", err
	}
	v, ok := s.Sys().(*syscall.Stat_t)
	if !ok {
		return "", ErrUnsupported
	}
	return fmt.Sprintf("%d:%d", uint64(v.Dev), uint64(v.Ino)), nil
}
func (Native) Sample(context.Context) (Snapshot, error) { return Snapshot{}, ErrUnsupported }
func (Native) Check(ctx context.Context, path string, write bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if write {
		return ErrUnsupported
	}
	f, err := os.Open(path)
	if err != nil {
		return errors.New("read permission unavailable")
	}
	return f.Close()
}
func (Native) Run(ctx context.Context, run func(context.Context) error) error {
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()
	return run(ctx)
}
func (Native) Read(ctx context.Context, path string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 65537))
	if len(b) > 65536 {
		return nil, errors.New("state exceeds bound")
	}
	return b, err
}

// Fixed local path only. Existing directory must be private and service-owned.
// This is a local state primitive, NOT a telemetry WAL or secret store.
func (Native) Replace(ctx context.Context, path string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(data) > 65536 {
		return errors.New("state exceeds bound")
	}
	dir := filepath.Dir(path)
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.IsDir() || info.Mode().Perm()&0077 != 0 || st.Uid != uint32(os.Geteuid()) {
		return errors.New("state directory must be private and owned by service user")
	}
	f, err := os.CreateTemp(dir, ".agent-state-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = os.Rename(name, path); err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

// AGENTV1 FILE END
