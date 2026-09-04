// AGENTV1 FILE START: stable non-secret deployment/host scope; no endpoint or credentials.
package queue

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
)

type Scope struct{ BackendID, OrganizationID, HostID, Account, Region string }

func (s Scope) Hash() (string, error) {
	values := []string{s.BackendID, s.OrganizationID, s.HostID, s.Account, s.Region}
	for i, v := range values {
		if len(v) > 256 || strings.ContainsAny(v, "\x00\r\n") || (i < 3 && strings.TrimSpace(v) == "") {
			return "", errors.New("queue_scope_identity_required: backend_id, organization_id and canonical host identity are required")
		}
	}
	b, _ := json.Marshal(values)
	return hashScope("observe-metrics-scope-v2\x00" + string(b)), nil
}

// LogsHash intentionally uses a separate namespace without changing the
// existing metrics Hash representation or its migration contract.
func (s Scope) LogsHash() (string, error) {
	values := []string{s.BackendID, s.OrganizationID, s.HostID, s.Account, s.Region}
	for i, v := range values {
		if len(v) > 256 || strings.ContainsAny(v, "\x00\r\n") || (i < 3 && strings.TrimSpace(v) == "") {
			return "", errors.New("logs_queue_scope_identity_required: backend_id, organization_id and canonical host identity are required")
		}
	}
	b, _ := json.Marshal(values)
	return hashScope("observe-logs-scope-v2\x00" + string(b)), nil
}
func hashScope(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
func (s Scope) legacyHash(endpoint string) string {
	return hashScope(endpoint + "\x00" + s.HostID + "\x00" + s.Account + "\x00" + s.Region)
}

// AGENTV1 FILE END
