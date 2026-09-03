// AGENTV1 FILE START: OTel processor limits contract, no running processor.
package processors

type Limits struct {
	MemoryMiB         int
	BatchBytes        int
	MaxAttributes     int
	MaxAttributeBytes int
}

// AGENTV1 FILE END
