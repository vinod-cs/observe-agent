// AGENTV1 FILE START: closed capability declarations and redaction tests.
package policy

import (
	"strings"
	"testing"
)

func TestClosedPolicy(t *testing.T) {
	bad := Document{Version: 1, Enabled: map[Capability]bool{"secret-value-must-not-echo": true}}
	if err := bad.Validate(); err == nil || strings.Contains(err.Error(), "secret-value") {
		t.Fatal("unknown policy accepted or echoed")
	}
	if (Document{}).Validate() == nil {
		t.Fatal("zero version accepted")
	}
	good := Document{Version: 1, Enabled: map[Capability]bool{Metrics: true}}
	clone := good.Clone()
	clone.Enabled[Logs] = true
	if good.Enabled[Logs] || good.Equal(clone) {
		t.Fatal("policy clone not isolated")
	}
	if !good.Equal(Document{Version: 1, Enabled: map[Capability]bool{Metrics: true, Logs: false}}) {
		t.Fatal("omitted capability not disabled")
	}
}
func TestCatalogueDeclaresOnlyUnimplementedReaders(t *testing.T) {
	seen := map[Capability]bool{}
	for _, entry := range Catalogue() {
		if !Known(entry.Capability) || seen[entry.Capability] || entry.Implemented || len(entry.Access) == 0 {
			t.Fatal("invalid foundation declaration")
		}
		seen[entry.Capability] = true
		for _, access := range entry.Access {
			if access.Resource == "" || access.Privilege == "" || access.Writes {
				t.Fatal("missing read-only permission declaration")
			}
		}
	}
	if len(seen) != len(All()) {
		t.Fatal("capability permissions missing")
	}
}

// AGENTV1 FILE END
