//go:build !linux

// AGENTV1 FILE START: metric runtime remains explicitly Linux-only.
package platform

import "github.com/agent-i/agent/internal/config"

func NewHostMetrics(config.Collection) (HostMetrics, error) { return nil, ErrUnsupported }

// AGENTV1 FILE END
