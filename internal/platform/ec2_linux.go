//go:build linux

// AGENTV1 FILE START: cloud hint prevents machine-ID fallback on recognizable EC2 hosts.
package platform

import (
	"os"
	"strings"
)

func EC2Expected() bool {
	for _, path := range []string{"/sys/devices/virtual/dmi/id/sys_vendor", "/sys/hypervisor/uuid"} {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		value := strings.ToLower(strings.TrimSpace(string(raw)))
		if strings.Contains(value, "amazon ec2") || strings.HasPrefix(value, "ec2") {
			return true
		}
	}
	return false
}

// AGENTV1 FILE END
