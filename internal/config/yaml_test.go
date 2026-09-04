// AGENTV1 FILE START: strict YAML, precedence and redaction regressions.
package config

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const yamlValid = `observe:
  backend_id: fixture-backend
  organization_id: fixture-org
  endpoint: https://ingest.example.com/api/v1/otlp
  api_key: fixture-key-not-real
collection:
  metrics:
    enabled: true
  logs:
    enabled: false
  traces:
    enabled: false
`

func TestYAMLAndRedaction(t *testing.T) {
	c, e := Parse(strings.NewReader(yamlValid))
	if e != nil {
		t.Fatal(e)
	}
	if !c.Policy.Enabled["metrics"] || c.Policy.Enabled["logs"] || c.Policy.Enabled["traces"] {
		t.Fatal("policy mapping")
	}
	b, e := c.SecretProvider().Authorization(context.Background(), "")
	if e != nil || string(b) != "ApiKey fixture-key-not-real" {
		t.Fatal("inline credential contract")
	}
	encoded, _ := json.Marshal(c)
	for _, s := range []string{string(encoded), fmt.Sprintf("%v", c), fmt.Sprintf("%+v", c), fmt.Sprintf("%#v", c)} {
		if strings.Contains(s, "fixture-key-not-real") {
			t.Fatal("credential exposed")
		}
	}
	if e = c.CheckCredentials(context.Background()); e != nil {
		t.Fatal(e)
	}
}
func TestYAMLStrictFailures(t *testing.T) {
	for name, s := range map[string]string{
		"duplicate":      strings.Replace(yamlValid, "  api_key:", "  api_key: other\n  api_key:", 1),
		"unknown":        yamlValid + "unknown: fixture-key-not-real\n",
		"alias":          strings.Replace(yamlValid, "fixture-key-not-real", "&secret fixture-key-not-real", 1),
		"multiple":       yamlValid + "---\nobserve: {}\n",
		"wrong bool":     strings.Replace(yamlValid, "enabled: true", "enabled: yes", 1),
		"traces enabled": strings.Replace(yamlValid, "traces:\n    enabled: false", "traces:\n    enabled: true", 1),
		"merge":          yamlValid + "<<: {malicious: fixture-key-not-real}\n",
		"bad tag":        strings.Replace(yamlValid, "fixture-key-not-real", "!secret fixture-key-not-real", 1),
	} {
		t.Run(name, func(t *testing.T) {
			_, e := Parse(strings.NewReader(s))
			if e == nil {
				t.Fatal("accepted")
			}
			if strings.Contains(e.Error(), "fixture-key-not-real") {
				t.Fatal("error exposed secret")
			}
		})
	}
}

func TestLinuxLogsYAMLSchema(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux Logs configuration")
	}
	raw := strings.Replace(yamlValid, "logs:\n    enabled: false", `logs:
    enabled: true
    state_directory: /var/lib/observe-agent/logs
    queue_bytes: 1048576
    queue_items: 32
    poll_interval: 1s
    max_files: 16
    files:
      - id: application
        root: /var/log/application
        include: ["*.log"]
        exclude: ["*.gz"]
        start_at: beginning
        service_name: checkout
        environment: production
        max_open_files: 8
        max_line_bytes: 4096
        multiline:
          enabled: true
          start_pattern: '^START'
          flush_timeout: 2s
          max_lines: 20
          max_bytes: 4096`, 1)
	c, err := Parse(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	l := c.LogsRuntime()
	if !c.Policy.Enabled["logs"] || len(l.Sources) != 1 || l.Sources[0].Root != "/var/log/application" || l.Sources[0].Multiline.FlushTimeoutMillis != 2000 {
		t.Fatal("logs mapping")
	}
	for _, invalid := range []string{"../*.log", "/etc/passwd", "sub/*.log", "bad\x00.log"} {
		candidate := strings.Replace(raw, `include: ["*.log"]`, `include: ["`+invalid+`"]`, 1)
		if _, err = Parse(strings.NewReader(candidate)); err == nil {
			t.Fatalf("unsafe pattern accepted: %q", invalid)
		}
	}
}
func TestYAMLReferencePrecedence(t *testing.T) {
	s := strings.Replace(yamlValid, "  api_key:", "  api_key_env: YAML_TEST_KEY\n  api_key_file: "+filepath.ToSlash(filepath.Join(t.TempDir(), "not-read"))+"\n  api_key:", 1)
	c, e := Parse(strings.NewReader(s))
	if e != nil {
		t.Fatal(e)
	}
	t.Setenv("YAML_TEST_KEY", "")
	if c.CheckCredentials(context.Background()) == nil {
		t.Fatal("fell back to inline")
	}
	t.Setenv("YAML_TEST_KEY", "environment-only-fixture")
	b, e := c.SecretProvider().Authorization(context.Background(), "")
	if e != nil || string(b) != "ApiKey environment-only-fixture" {
		t.Fatal("env precedence")
	}
}

// AGENTV1 FILE END
