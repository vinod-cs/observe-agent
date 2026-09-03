// AGENTV1 FILE START: strict secret-free policy/config schema.
package config

import (
	"strings"
	"testing"
)

const valid = `{"agent_id":"","exporter":{"type":"otlp_http","endpoint":"https://ingest.example.com/api/v1/otlp","headers_env":{"Authorization":"OBSERVE_AUTH"}},"policy":{"version":1,"enabled":{"metrics":true}}}`

func TestConfig(t *testing.T) {
	c, e := Parse(strings.NewReader(valid))
	if e != nil {
		t.Fatal(e)
	}
	if c.AgentID != "" || c.Limits.RequestBytes != 4<<20 {
		t.Fatal("defaults")
	}
	cases := []string{
		strings.Replace(valid, "https://", "http://", 1), strings.Replace(valid, "ingest.example.com", "user:password@ingest.example.com", 1), strings.Replace(valid, "/api/v1/otlp", "/api/v1/otlp?secret=x", 1),
		strings.Replace(valid, `"OBSERVE_AUTH"`, `"ApiKey secret"`, 1), strings.Replace(valid, `"metrics":true`, `"metrics":true,"command":true`, 1), strings.Replace(valid, `"version":1`, `"version":1,"version":2`, 1),
		strings.Replace(valid, `"agent_id":""`, `"password":"do-not-echo"`, 1), valid + `{}`, strings.Repeat(" ", MaxConfigBytes+1),
	}
	for i, raw := range cases {
		if _, e := Parse(strings.NewReader(raw)); e == nil {
			t.Fatalf("case %d accepted", i)
		} else if strings.Contains(e.Error(), "do-not-echo") {
			t.Fatal("value leaked")
		}
	}
}
func TestUnknownFieldsFail(t *testing.T) {
	if _, e := Parse(strings.NewReader(strings.Replace(valid, `"version":1`, `"version":1,"shell":"x"`, 1))); e == nil {
		t.Fatal("remote execution field accepted")
	}
}

// AGENTV1 FILE END
