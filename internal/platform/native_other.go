//go:build !linux && !windows && !darwin

// AGENTV1 FILE START: unsupported OS never receives a fake identity.
package platform

import (
	"context"
	"os"
)

func Defaults() Layout                                                { return Layout{} }
func (Native) MachineID(context.Context) (string, error)              { return "", ErrUnsupported }
func (Native) ID(*os.File) (string, error)                            { return "", ErrUnsupported }
func (Native) Sample(context.Context) (Snapshot, error)               { return Snapshot{}, ErrUnsupported }
func (Native) Check(context.Context, string, bool) error              { return ErrUnsupported }
func (Native) Read(context.Context, string) ([]byte, error)           { return nil, ErrUnsupported }
func (Native) Replace(context.Context, string, []byte) error          { return ErrUnsupported }
func (Native) Run(context.Context, func(context.Context) error) error { return ErrUnsupported }

// AGENTV1 FILE END
