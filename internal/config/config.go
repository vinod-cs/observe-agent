// AGENTV1 FILE START: strict foundation config with secret references, never secrets.
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"regexp"
	"strings"

	"github.com/agent-i/agent/internal/policy"
	"github.com/agent-i/agent/internal/security"
)

const MaxConfigBytes = 64 << 10

var envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type Exporter struct {
	// AGENTV1 START: non-secret stable scope and one-time v1 migration evidence.
	BackendID        string `json:"backend_id,omitempty"`
	OrganizationID   string `json:"organization_id,omitempty"`
	PreviousEndpoint string `json:"previous_endpoint,omitempty"`
	// AGENTV1 END: queue scope configuration
	Type       string            `json:"type"`
	Endpoint   string            `json:"endpoint"`
	HeadersEnv map[string]string `json:"headers_env"`
}
type Limits struct {
	RequestBytes    int   `json:"request_bytes"`
	QueueBytes      int64 `json:"queue_bytes"`
	MemoryMiB       int   `json:"memory_mib"`
	ShutdownSeconds int   `json:"shutdown_seconds"`
}
type Config struct {
	// AGENTV1 START: private/redacted YAML credentials never serialize into config dumps.
	yamlConfig bool
	credential security.Credential
	// AGENTV1 END: YAML credential carrier
	// AGENTV1 START: bounded Linux metrics-only runtime configuration
	Collection Collection  `json:"collection"`
	EC2        EC2Metadata `json:"ec2_metadata"`
	Delivery   Delivery    `json:"delivery"`
	// AGENTV1 END: bounded Linux metrics-only runtime configuration
	AgentID       string              `json:"agent_id"`
	Exporter      Exporter            `json:"exporter"`
	Policy        policy.Document     `json:"policy"`
	RemoteAllowed []policy.Capability `json:"remote_allowed"`
	Limits        Limits              `json:"limits"`
}

func Parse(r io.Reader) (Config, error) {
	var cfg Config
	raw, err := io.ReadAll(io.LimitReader(r, MaxConfigBytes+1))
	if err != nil || len(raw) > MaxConfigBytes {
		return cfg, errors.New("configuration unavailable or exceeds size limit")
	}
	// JSON is deliberately the foundation encoding. Do not claim legacy YAML support.
	// AGENTV1 START: retain the strict JSON parser; route customer YAML separately.
	if !bytes.HasPrefix(bytes.TrimSpace(raw), []byte("{")) {
		return parseYAML(raw)
	}
	// AGENTV1 END: YAML frontend
	// Reject duplicate keys as well as unknown fields: policy must be unambiguous.
	if err = rejectDuplicateKeys(raw); err != nil {
		return cfg, errors.New("configuration contains ambiguous or invalid JSON")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err = d.Decode(&cfg); err != nil {
		return Config{}, errors.New("configuration schema invalid")
	}
	if err = d.Decode(new(any)); err != io.EOF {
		return Config{}, errors.New("configuration must contain one object")
	}
	if cfg.Limits == (Limits{}) {
		cfg.Limits = Limits{4 << 20, 64 << 20, 128, 15}
	}
	if err = cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	if err := c.ValidateRuntime(); err != nil {
		return err
	}
	if len(c.AgentID) > 256 || strings.ContainsAny(c.AgentID, "\r\n\x00") {
		return errors.New("invalid agent identity configuration")
	}
	if err := c.Policy.Validate(); err != nil {
		return err
	}
	for _, x := range c.RemoteAllowed {
		if !policy.Known(x) {
			return errors.New("unknown locally allowed capability")
		}
	}
	if c.Exporter.Type != "otlp_http" {
		return errors.New("foundation requires otlp_http exporter")
	}
	u, err := url.Parse(c.Exporter.Endpoint)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" || strings.TrimSuffix(u.Path, "/") != "/api/v1/otlp" {
		return errors.New("exporter endpoint must be an HTTPS Observe /api/v1/otlp base without credentials or query")
	}
	if len(c.Exporter.HeadersEnv) != 1 || !envName.MatchString(c.Exporter.HeadersEnv["Authorization"]) {
		return errors.New("headers_env requires only Authorization with an environment variable name")
	}
	if c.Limits.RequestBytes < 1024 || c.Limits.RequestBytes > 4<<20 || c.Limits.QueueBytes < 1<<20 || c.Limits.QueueBytes > 1<<30 || c.Limits.MemoryMiB < 32 || c.Limits.MemoryMiB > 4096 || c.Limits.ShutdownSeconds < 1 || c.Limits.ShutdownSeconds > 120 {
		return errors.New("limits outside allowed bounds")
	}
	return nil
}

func rejectDuplicateKeys(raw []byte) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	var value func() error
	value = func() error {
		t, err := d.Token()
		if err != nil {
			return err
		}
		switch t {
		case json.Delim('{'):
			seen := map[string]bool{}
			for d.More() {
				key, e := d.Token()
				if e != nil {
					return e
				}
				k, ok := key.(string)
				if !ok || seen[k] {
					return errors.New("duplicate key")
				}
				seen[k] = true
				if e = value(); e != nil {
					return e
				}
			}
			_, err = d.Token()
		case json.Delim('['):
			for d.More() {
				if err = value(); err != nil {
					return err
				}
			}
			_, err = d.Token()
		}
		return err
	}
	if err := value(); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return errors.New("trailing content")
	}
	return nil
}

// AGENTV1 FILE END
