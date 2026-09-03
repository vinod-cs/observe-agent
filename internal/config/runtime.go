// AGENTV1 FILE START: centralized metrics runtime limits and metadata policy.
package config

import (
	"errors"
	"path/filepath"
)

type Collection struct {
	IntervalSeconds int `json:"interval_seconds"`
	MaxPoints       int `json:"max_points"`
	MaxDevices      int `json:"max_devices"`
	MaxInterfaces   int `json:"max_interfaces"`
	MaxMounts       int `json:"max_mounts"`
}
type EC2Metadata struct {
	Enabled        *bool `json:"enabled"`
	Required       bool  `json:"required"`
	TimeoutSeconds int   `json:"timeout_seconds"`
}
type Delivery struct {
	// AGENTV1 START: persistent spool configuration; secret-independent scope.
	StateDirectory string `json:"state_directory"`
	OverflowPolicy string `json:"overflow_policy"`
	// AGENTV1 END: persistent spool configuration
	BatchPoints           int `json:"batch_points"`
	QueueItems            int `json:"queue_items"`
	MaxAttempts           int `json:"max_attempts"`
	RequestTimeoutSeconds int `json:"request_timeout_seconds"`
}

func (c Config) Runtime() (Collection, EC2Metadata, Delivery) {
	a, b, d := c.Collection, c.EC2, c.Delivery
	// AGENTV1 START: Linux production state defaults.
	if d.StateDirectory == "" {
		d.StateDirectory = "/var/lib/observe-agent/metrics"
	}
	if d.OverflowPolicy == "" {
		d.OverflowPolicy = "reject_new"
	}
	// AGENTV1 END: Linux production state defaults
	if a.IntervalSeconds == 0 {
		a.IntervalSeconds = 15
	}
	if a.MaxPoints == 0 {
		a.MaxPoints = 4096
	}
	if a.MaxDevices == 0 {
		a.MaxDevices = 128
	}
	if a.MaxInterfaces == 0 {
		a.MaxInterfaces = 64
	}
	if a.MaxMounts == 0 {
		a.MaxMounts = 64
	}
	if b.Enabled == nil {
		on := true
		b.Enabled = &on
	}
	if b.TimeoutSeconds == 0 {
		b.TimeoutSeconds = 2
	}
	if d.BatchPoints == 0 {
		d.BatchPoints = 500
	}
	if d.QueueItems == 0 {
		d.QueueItems = 64
	}
	if d.MaxAttempts == 0 {
		d.MaxAttempts = 4
	}
	if d.RequestTimeoutSeconds == 0 {
		d.RequestTimeoutSeconds = 15
	}
	return a, b, d
}
func (c Config) ValidateRuntime() error {
	a, b, d := c.Runtime()
	// AGENTV1 START: no silent memory fallback or destructive eviction.
	if d.OverflowPolicy != "reject_new" || (d.StateDirectory != "/var/lib/observe-agent/metrics" && !filepath.IsAbs(d.StateDirectory)) {
		return errors.New("invalid persistent delivery options")
	}
	// AGENTV1 END: no silent memory fallback
	if a.IntervalSeconds < 5 || a.IntervalSeconds > 3600 || a.MaxPoints < 32 || a.MaxPoints > 16384 || a.MaxDevices < 1 || a.MaxDevices > 256 || a.MaxInterfaces < 1 || a.MaxInterfaces > 256 || a.MaxMounts < 1 || a.MaxMounts > 128 {
		return errors.New("collection limits invalid")
	}
	if b.TimeoutSeconds < 1 || b.TimeoutSeconds > 10 || (b.Required && !*b.Enabled) {
		return errors.New("EC2 metadata options invalid")
	}
	if d.BatchPoints < 1 || d.BatchPoints > 1000 || d.QueueItems < 1 || d.QueueItems > 1024 || d.MaxAttempts < 1 || d.MaxAttempts > 8 || d.RequestTimeoutSeconds < 1 || d.RequestTimeoutSeconds > 60 {
		return errors.New("delivery limits invalid")
	}
	return nil
}

// AGENTV1 FILE END
