//go:build !linux

// AGENTV1 FILE START: non-Linux Logs capability remains explicitly unsupported.
package collectors

import (
	"context"
	"errors"
	"github.com/agent-i/agent/internal/config"
	"github.com/agent-i/agent/internal/selftelemetry"
)

type Logs struct{}

func NewLogs(config.Config, *selftelemetry.LogCounters, *selftelemetry.Counters) *Logs {
	return &Logs{}
}
func (*Logs) Start(context.Context) error {
	return errors.New("Linux file logs are unsupported on this platform")
}
func (*Logs) Stop(context.Context) error  { return nil }
func (*Logs) Snapshot() map[string]uint64 { return map[string]uint64{} }

// AGENTV1 FILE END
