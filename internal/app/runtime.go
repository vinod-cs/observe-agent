// AGENTV1 FILE START: explicit service runtime, metrics-only registration and redacted diagnostics.
package app

import (
	"context"
	"encoding/json"
	"github.com/agent-i/agent/internal/collectors"
	"github.com/agent-i/agent/internal/config"
	"github.com/agent-i/agent/internal/platform"
	"github.com/agent-i/agent/internal/policy"
	"github.com/agent-i/agent/internal/selftelemetry"
	"io"
	"time"
)

func Run(ctx context.Context, cfg config.Config, diagnostics io.Writer) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	stats := &selftelemetry.Counters{}
	logStats, logDelivery := &selftelemetry.LogCounters{}, &selftelemetry.Counters{}
	var logsCollector *collectors.Logs
	registry := collectors.Registry{
		policy.Metrics: {Descriptor: policy.Descriptor{Capability: policy.Metrics, Implemented: true, Access: policy.Catalogue()[0].Access}, New: func() (collectors.Collector, error) { return collectors.NewMetrics(cfg, stats), nil }},
		policy.Logs: {Descriptor: policy.Descriptor{Capability: policy.Logs, Implemented: true, Access: policy.Catalogue()[1].Access}, New: func() (collectors.Collector, error) {
			logsCollector = collectors.NewLogs(cfg, logStats, logDelivery)
			return logsCollector, nil
		}},
	}
	return (platform.Native{}).Run(ctx, func(ctx context.Context) error {
		manager := New(ctx, registry, nil, nil, time.Duration(cfg.Limits.ShutdownSeconds)*time.Second)
		if err := manager.Apply(ctx, cfg.Policy); err != nil {
			return err
		}
		timer := time.NewTicker(time.Minute)
		defer timer.Stop()
		report := func() {
			values := stats.Snapshot()
			if logsCollector != nil {
				for k, v := range logsCollector.Snapshot() {
					values["logs_"+k] = v
				}
			}
			_ = json.NewEncoder(diagnostics).Encode(values)
		}
		for {
			select {
			case <-ctx.Done():
				err := manager.Close()
				report()
				return err
			case <-timer.C:
				report()
			}
		}
	})
}

// AGENTV1 FILE END
