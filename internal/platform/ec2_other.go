//go:build !linux

// AGENTV1 FILE START: no unsupported-platform cloud probing.
package platform

func EC2Expected() bool { return false }

// AGENTV1 FILE END
