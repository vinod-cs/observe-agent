// AGENTV1 FILE START: cloud evidence contract, no metadata network requests in foundation.
package cloud

import "context"

// Evidence must come from a trusted detector, NOT arbitrary config/OTLP claims.
// Verified denotes detector validation, not cryptographic instance attestation.
type Evidence struct {
	Verified                                              bool
	Provider, Platform, Account, Region, AvailabilityZone string
	InstanceID, InstanceType, ResourceID                  string
}
type Detector interface {
	Detect(context.Context) (Evidence, error)
}

// AGENTV1 FILE END
