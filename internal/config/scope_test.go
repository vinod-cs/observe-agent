// AGENTV1 FILE START: protected config identity validation and redacted diagnostics.
package config

import (
	"context"
	"strings"
	"testing"
)

func TestQueueIdentityConfig(t *testing.T) {
	c, e := Parse(strings.NewReader(yamlValid + ""))
	if e != nil {
		t.Fatal(e)
	}
	c.Exporter.PreviousEndpoint = "https://old.example.test/api/v1/otlp"
	if e = c.CheckCredentials(context.Background()); e != nil {
		t.Fatal(e)
	}
	for _, bad := range []string{"https://user:supersecret@old.example.test/api/v1/otlp", "https://old.example.test/api/v1/otlp?key=supersecret", "http://old.example.test/api/v1/otlp"} {
		c.Exporter.PreviousEndpoint = bad
		e = c.ValidateQueueIdentity()
		if e == nil || strings.Contains(e.Error(), "supersecret") {
			t.Fatal("bad migration endpoint/redaction")
		}
	}
	c.Exporter.PreviousEndpoint = ""
	c.Exporter.OrganizationID = ""
	if c.ValidateQueueIdentity() == nil {
		t.Fatal("missing org accepted")
	}
	c.Exporter.OrganizationID = "org-a"
	c.Exporter.BackendID = ""
	if c.ValidateQueueIdentity() == nil {
		t.Fatal("missing backend accepted")
	}
}

// AGENTV1 FILE END
