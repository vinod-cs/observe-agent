// AGENTV1 FILE START: centralized metrics runtime limits and metadata policy.
package config

import (
	"errors"
	"path"
	"path/filepath"
	"regexp"
	"strings"
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

// LogsConfig is deliberately separate from Delivery: the existing Delivery
// fields and limits continue to describe the metrics spool only.
type LogsConfig struct {
	StateDirectory     string      `json:"state_directory,omitempty"`
	QueueBytes         int64       `json:"queue_bytes,omitempty"`
	QueueItems         int         `json:"queue_items,omitempty"`
	OverflowPolicy     string      `json:"overflow_policy,omitempty"`
	PollIntervalMillis int         `json:"poll_interval_millis,omitempty"`
	MaxFiles           int         `json:"max_files,omitempty"`
	Sources            []LogSource `json:"files,omitempty"`
}

type LogSource struct {
	ID           string       `json:"id"`
	Root         string       `json:"root"`
	Include      []string     `json:"include"`
	Exclude      []string     `json:"exclude,omitempty"`
	StartAt      string       `json:"start_at,omitempty"`
	ServiceName  string       `json:"service_name,omitempty"`
	Environment  string       `json:"environment,omitempty"`
	MaxOpenFiles int          `json:"max_open_files,omitempty"`
	MaxLineBytes int          `json:"max_line_bytes,omitempty"`
	Multiline    LogMultiline `json:"multiline,omitempty"`
}

type LogMultiline struct {
	Enabled            bool   `json:"enabled"`
	StartPattern       string `json:"start_pattern,omitempty"`
	FlushTimeoutMillis int    `json:"flush_timeout_millis,omitempty"`
	MaxLines           int    `json:"max_lines,omitempty"`
	MaxBytes           int    `json:"max_bytes,omitempty"`
}

func (c Config) LogsRuntime() LogsConfig {
	l := c.Logs
	if l.StateDirectory == "" {
		l.StateDirectory = "/var/lib/observe-agent/logs"
	}
	if l.QueueBytes == 0 {
		l.QueueBytes = 64 << 20
	}
	if l.QueueItems == 0 {
		l.QueueItems = 1024
	}
	if l.OverflowPolicy == "" {
		l.OverflowPolicy = "reject_new"
	}
	if l.PollIntervalMillis == 0 {
		l.PollIntervalMillis = 1000
	}
	if l.MaxFiles == 0 {
		l.MaxFiles = 256
	}
	for i := range l.Sources {
		s := &l.Sources[i]
		if s.StartAt == "" {
			s.StartAt = "end"
		}
		if s.MaxOpenFiles == 0 {
			s.MaxOpenFiles = 32
		}
		if s.MaxLineBytes == 0 {
			s.MaxLineBytes = 256 << 10
		}
		if s.Multiline.FlushTimeoutMillis == 0 {
			s.Multiline.FlushTimeoutMillis = 5000
		}
		if s.Multiline.MaxLines == 0 {
			s.Multiline.MaxLines = 200
		}
		if s.Multiline.MaxBytes == 0 {
			s.Multiline.MaxBytes = 256 << 10
		}
	}
	return l
}

var logID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

func validateLogPattern(v string) bool {
	if v == "" || len(v) > 256 || strings.ContainsAny(v, "\\/\x00\r\n") || v == "." || v == ".." {
		return false
	}
	_, err := path.Match(v, "candidate.log")
	return err == nil
}

func (c Config) validateLogs() error {
	l := c.LogsRuntime()
	if !path.IsAbs(l.StateDirectory) || strings.ContainsAny(l.StateDirectory, "\x00\r\n") || l.QueueBytes < 1<<20 || l.QueueBytes > 1<<30 || l.QueueItems < 1 || l.QueueItems > 4096 || l.OverflowPolicy != "reject_new" || l.PollIntervalMillis < 200 || l.PollIntervalMillis > 60000 || l.MaxFiles < 1 || l.MaxFiles > 1024 || len(l.Sources) > 16 {
		return errors.New("logs delivery limits invalid")
	}
	seen := map[string]bool{}
	for _, s := range l.Sources {
		if !logID.MatchString(s.ID) || seen[s.ID] || !path.IsAbs(s.Root) || path.Clean(s.Root) != s.Root || s.Root == "/" || strings.ContainsAny(s.Root, "\x00\r\n") || len(s.Root) > 4096 {
			return errors.New("logs source identity or root invalid")
		}
		seen[s.ID] = true
		if len(s.Include) == 0 || len(s.Include) > 32 || len(s.Exclude) > 32 {
			return errors.New("logs source patterns invalid")
		}
		for _, pattern := range append(append([]string{}, s.Include...), s.Exclude...) {
			if !validateLogPattern(pattern) {
				return errors.New("logs source pattern invalid")
			}
		}
		if s.StartAt != "beginning" && s.StartAt != "end" {
			return errors.New("logs start_at invalid")
		}
		if s.MaxOpenFiles < 1 || s.MaxOpenFiles > 128 || s.MaxLineBytes < 256 || s.MaxLineBytes > 1<<20 || len(s.ServiceName) > 256 || len(s.Environment) > 256 || strings.ContainsAny(s.ServiceName+s.Environment, "\x00\r\n") {
			return errors.New("logs source limits invalid")
		}
		m := s.Multiline
		if m.Enabled {
			if m.StartPattern == "" || len(m.StartPattern) > 1024 {
				return errors.New("logs multiline pattern required")
			}
			if _, err := regexp.Compile(m.StartPattern); err != nil {
				return errors.New("logs multiline pattern invalid")
			}
		}
		if m.FlushTimeoutMillis < 100 || m.FlushTimeoutMillis > 60000 || m.MaxLines < 1 || m.MaxLines > 1000 || m.MaxBytes < 256 || m.MaxBytes > 1<<20 || m.MaxBytes > s.MaxLineBytes {
			return errors.New("logs multiline limits invalid")
		}
	}
	return nil
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
	if err := c.validateLogs(); err != nil {
		return err
	}
	return nil
}

// AGENTV1 FILE END
