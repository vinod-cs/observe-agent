// AGENTV1 FILE START: compatible Agent ID and canonical host identity resolution.
package identity

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"github.com/agent-i/agent/internal/cloud"
)

type MachineReader interface {
	MachineID(context.Context) (string, error)
}
type Resolved struct {
	AgentID, HostID string
	Cloud           cloud.Evidence
}

var instanceID = regexp.MustCompile(`^i-([0-9a-f]{8}|[0-9a-f]{17})$`)

func safe(s string) bool { return len(s) > 0 && len(s) <= 256 && !strings.ContainsAny(s, "\x00\r\n") }

func Resolve(ctx context.Context, configured string, evidence cloud.Evidence, reader MachineReader) (Resolved, error) {
	configured = strings.TrimSpace(configured)
	if configured != "" && !safe(configured) {
		return Resolved{}, errors.New("configured agent identity invalid")
	}
	host := ""
	if evidence.Verified && evidence.Provider == "aws" && evidence.Platform == "aws_ec2" && instanceID.MatchString(evidence.InstanceID) {
		host = evidence.InstanceID
	} else {
		evidence = cloud.Evidence{}
		if reader != nil {
			if value, err := reader.MachineID(ctx); err == nil {
				value = strings.TrimSpace(value)
				if safe(value) {
					host = value
				}
			}
		}
	}
	agent := configured
	if agent == "" {
		agent = host
	}
	if agent == "" {
		return Resolved{}, errors.New("all supported agent identity sources unavailable")
	}
	// Preserve old explicit-agent_id behavior: a missing canonical host.id is not invented.
	// RequireHost is a separate collection preflight for host-correlated signals.
	return Resolved{agent, host, evidence}, nil
}
func (r Resolved) RequireHost() error {
	if r.HostID == "" {
		return errors.New("canonical host identity unavailable")
	}
	return nil
}
func (r Resolved) Resource(osType, arch, version, service string) map[string]string {
	if service == "" {
		service = r.AgentID
	}
	out := map[string]string{"host.name": r.AgentID, "service.name": service, "telemetry.distro.name": "agent-i", "telemetry.distro.version": version, "os.type": osType, "host.arch": arch}
	if r.HostID != "" {
		out["host.id"] = r.HostID
	}
	if r.Cloud.Verified {
		for k, v := range map[string]string{"cloud.provider": r.Cloud.Provider, "cloud.platform": r.Cloud.Platform, "cloud.account.id": r.Cloud.Account, "cloud.region": r.Cloud.Region, "cloud.availability_zone": r.Cloud.AvailabilityZone, "cloud.resource_id": r.Cloud.ResourceID, "host.type": r.Cloud.InstanceType} {
			if v != "" {
				out[k] = v
			}
		}
	}
	return out
}

// AGENTV1 FILE END
