// AGENTV1 FILE START: protected deployment identity; no default tenant/backend guesses.
package config

import (
	"errors"
	"net/url"
	"regexp"
)

var logicalID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

func (c Config) ValidateQueueIdentity() error {
	if !logicalID.MatchString(c.Exporter.BackendID) || !logicalID.MatchString(c.Exporter.OrganizationID) {
		return errors.New("queue_scope_identity_required: configure stable observe.backend_id and observe.organization_id (JSON: exporter fields)")
	}
	if c.Exporter.PreviousEndpoint != "" {
		u, e := url.Parse(c.Exporter.PreviousEndpoint)
		if e != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" || (u.Path != "/api/v1/otlp" && u.Path != "/api/v1/otlp/") {
			return errors.New("queue_scope_previous_endpoint_invalid: use the exact original HTTPS OTLP base without credentials/query")
		}
	}
	return nil
}

// AGENTV1 FILE END
