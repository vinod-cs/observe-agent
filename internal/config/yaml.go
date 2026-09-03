// AGENTV1 FILE START: strict customer YAML frontend into the existing normalized config.
package config

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/agent-i/agent/internal/policy"
	"github.com/agent-i/agent/internal/security"
	"go.yaml.in/yaml/v3"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type yamlToggle struct {
	Enabled bool `json:"enabled"`
}
type yamlFrontend struct {
	AgentID string `json:"agent_id"`
	Observe struct {
		BackendID        string `json:"backend_id"`
		OrganizationID   string `json:"organization_id"`
		PreviousEndpoint string `json:"previous_endpoint"`
		Endpoint         string `json:"endpoint"`
		APIKey           string `json:"api_key"`
		APIKeyEnv        string `json:"api_key_env"`
		APIKeyFile       string `json:"api_key_file"`
	} `json:"observe"`
	Collection struct {
		Metrics  yamlToggle `json:"metrics"`
		Logs     yamlToggle `json:"logs"`
		Traces   yamlToggle `json:"traces"`
		Interval int        `json:"interval_seconds"`
	} `json:"collection"`
	EC2      EC2Metadata `json:"ec2_metadata"`
	Delivery Delivery    `json:"delivery"`
	Limits   Limits      `json:"limits"`
}

func parseYAML(raw []byte) (Config, error) {
	invalid := errors.New("YAML configuration invalid: check schema, unique keys and credential settings")
	var node yaml.Node
	d := yaml.NewDecoder(bytes.NewReader(raw))
	if d.Decode(&node) != nil {
		return Config{}, invalid
	}
	if d.Decode(&yaml.Node{}) != io.EOF {
		return Config{}, invalid
	}
	count := 0
	var convert func(*yaml.Node, int) (any, error)
	convert = func(n *yaml.Node, depth int) (any, error) {
		count++
		if count > 2048 || depth > 16 || n.Anchor != "" || n.Kind == yaml.AliasNode {
			return nil, invalid
		}
		switch n.Kind {
		case yaml.DocumentNode:
			if len(n.Content) != 1 {
				return nil, invalid
			}
			return convert(n.Content[0], depth+1)
		case yaml.MappingNode:
			out := map[string]any{}
			for i := 0; i < len(n.Content); i += 2 {
				k := n.Content[i]
				if k.Kind != yaml.ScalarNode || k.Tag != "!!str" || k.Value == "<<" {
					return nil, invalid
				}
				if _, ok := out[k.Value]; ok {
					return nil, invalid
				}
				v, e := convert(n.Content[i+1], depth+1)
				if e != nil {
					return nil, e
				}
				out[k.Value] = v
			}
			return out, nil
		case yaml.ScalarNode:
			if n.Tag != "!!str" && n.Tag != "!!bool" && n.Tag != "!!int" && n.Tag != "!!null" {
				return nil, invalid
			}
			var v any
			if n.Decode(&v) != nil {
				return nil, invalid
			}
			return v, nil
		default:
			return nil, invalid
		}
	}
	value, e := convert(&node, 0)
	if e != nil {
		return Config{}, invalid
	}
	data, e := json.Marshal(value)
	if e != nil {
		return Config{}, invalid
	}
	var f yamlFrontend
	j := json.NewDecoder(bytes.NewReader(data))
	j.DisallowUnknownFields()
	if j.Decode(&f) != nil {
		return Config{}, invalid
	}
	if f.Collection.Logs.Enabled || f.Collection.Traces.Enabled {
		return Config{}, errors.New("this package supports metrics only; logs and traces must be disabled")
	}
	if f.Observe.APIKeyEnv != "" && !envName.MatchString(f.Observe.APIKeyEnv) {
		return Config{}, invalid
	}
	if f.Observe.APIKeyFile != "" && !filepath.IsAbs(f.Observe.APIKeyFile) {
		return Config{}, invalid
	}
	if f.Observe.APIKey == "" && f.Observe.APIKeyEnv == "" && f.Observe.APIKeyFile == "" {
		return Config{}, errors.New("observe authentication must be configured")
	}
	c := Config{AgentID: f.AgentID, Collection: Collection{IntervalSeconds: f.Collection.Interval}, EC2: f.EC2, Delivery: f.Delivery, Limits: f.Limits, Exporter: Exporter{Type: "otlp_http", Endpoint: f.Observe.Endpoint, HeadersEnv: map[string]string{"Authorization": "OBSERVE_CONFIG_CREDENTIAL"}}, Policy: policy.Document{Version: 1, Enabled: map[policy.Capability]bool{policy.Metrics: f.Collection.Metrics.Enabled, policy.Logs: false, policy.Traces: false}}, yamlConfig: true, credential: security.NewCredential(f.Observe.APIKey, f.Observe.APIKeyEnv, f.Observe.APIKeyFile)}
	// AGENTV1 START: map YAML non-secret scope into the existing normalized exporter model.
	c.Exporter.BackendID = f.Observe.BackendID
	c.Exporter.OrganizationID = f.Observe.OrganizationID
	c.Exporter.PreviousEndpoint = f.Observe.PreviousEndpoint
	// AGENTV1 END: scope mapping
	if c.Limits == (Limits{}) {
		c.Limits = Limits{4 << 20, 64 << 20, 128, 15}
	}
	if e = c.Validate(); e != nil {
		return Config{}, e
	}
	return c, nil
}
func (c Config) SecretProvider() security.SecretProvider {
	if c.yamlConfig {
		return c.credential
	}
	return security.Environment{}
}
func (Config) String() string   { return "ObserveConfig{credentials:redacted}" }
func (Config) GoString() string { return "ObserveConfig{credentials:redacted}" }

// Check resolves YAML references without network/identity/collector startup. Legacy JSON remains reference-only.
func (c Config) CheckCredentials(ctx context.Context) error {
	// AGENTV1 START: offline configuration identity check; no spool access or migration.
	if c.Policy.Enabled["metrics"] {
		if e := c.ValidateQueueIdentity(); e != nil {
			return e
		}
	}
	// AGENTV1 END: required deployment identity
	if !c.yamlConfig {
		return nil
	}
	b, e := c.credential.Authorization(ctx, "")
	security.Clear(b)
	return e
}
func Load(path string) (Config, error) {
	// All YAML must be protected, including malformed YAML: never echo parser values.
	var f *os.File
	var e error
	if strings.EqualFold(filepath.Ext(path), ".json") {
		f, e = os.Open(path)
	} else {
		f, e = security.OpenPrivate(path)
	}
	if e != nil {
		return Config{}, errors.New("configuration unavailable or permissions unsafe")
	}
	defer f.Close()
	c, e := Parse(f)
	if e != nil {
		return Config{}, e
	}
	// Prevent YAML content disguised with a .json extension from bypassing protection.
	if c.yamlConfig && strings.EqualFold(filepath.Ext(path), ".json") {
		private, err := security.OpenPrivate(path)
		if err != nil {
			return Config{}, errors.New("YAML configuration permissions unsafe")
		}
		private.Close()
	}
	return c, nil
}

// AGENTV1 FILE END
