// AGENTV1 FILE START: identity compatibility fixtures, no IMDS calls.
package identity

import (
	"context"
	"errors"
	"github.com/agent-i/agent/internal/cloud"
	"testing"
)

type machine struct {
	value string
	err   error
}

func (m machine) MachineID(context.Context) (string, error) { return m.value, m.err }
func TestIdentity(t *testing.T) {
	ctx := context.Background()
	ec2 := cloud.Evidence{Verified: true, Provider: "aws", Platform: "aws_ec2", Account: "127696279140", Region: "us-east-2", InstanceID: "i-0345d461c99a6da2f"}
	cases := []struct {
		name, configured string
		e                cloud.Evidence
		m                machine
		agent, host      string
		fail             bool
	}{
		{"explicit wins", "operator-label", ec2, machine{"local", nil}, "operator-label", ec2.InstanceID, false},
		{"empty uses EC2", "", ec2, machine{"local", nil}, ec2.InstanceID, ec2.InstanceID, false},
		{"noncloud stable", "", cloud.Evidence{}, machine{" machine-01\n", nil}, "machine-01", "machine-01", false},
		{"no sources", "", cloud.Evidence{}, machine{"", errors.New("absent")}, "", "", true},
		{"explicit is not host", "operator-label", cloud.Evidence{}, machine{}, "operator-label", "", false},
		{"unverified ignored", "", cloud.Evidence{Provider: "aws", Platform: "aws_ec2", InstanceID: ec2.InstanceID}, machine{"local", nil}, "local", "local", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, e := Resolve(ctx, c.configured, c.e, c.m)
			if (e != nil) != c.fail {
				t.Fatalf("unexpected failure: %v", e)
			}
			if !c.fail && (r.AgentID != c.agent || r.HostID != c.host) {
				t.Fatalf("wrong identity: %+v", r)
			}
		})
	}
	a, _ := Resolve(ctx, "", ec2, machine{})
	ec2.InstanceID = "i-08b0f19a7189543a0"
	b, _ := Resolve(ctx, "", ec2, machine{})
	if a.HostID == b.HostID {
		t.Fatal("two EC2 hosts collapsed")
	}
	if (Resolved{AgentID: "label"}).RequireHost() == nil {
		t.Fatal("missing host invented")
	}
}
func TestResourceContract(t *testing.T) {
	r := Resolved{AgentID: "label", HostID: "machine-01"}
	host := r.Resource("linux", "amd64", "test-version", "")
	app := r.Resource("linux", "amd64", "test-version", "checkout")
	if host["service.name"] != host["host.name"] || host["host.id"] != "machine-01" || host["telemetry.distro.name"] != "agent-i" {
		t.Fatal(host)
	}
	if app["service.name"] != "checkout" || app["host.id"] != host["host.id"] {
		t.Fatal(app)
	}
	if _, ok := app["service.version"]; ok {
		t.Fatal("agent version substituted for application")
	}
}

// AGENTV1 FILE END
