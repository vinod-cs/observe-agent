// AGENTV1 FILE START: bounded in-process, secret-free delivery counters.
package selftelemetry

import "sync/atomic"

type Counters struct {
	// AGENTV1 START: record means one serialized OTLP batch, counters reset per process.
	AcceptedRecords  atomic.Uint64
	QueuedRecords    atomic.Uint64
	RetriedRecords   atomic.Uint64
	DeliveredRecords atomic.Uint64
	CorruptRecords   atomic.Uint64
	DeliveryPaused   atomic.Uint64
	// AGENTV1 END: persistent delivery counters
	Scrapes         atomic.Uint64
	ReadErrors      atomic.Uint64
	QueueRejected   atomic.Uint64
	BatchesAccepted atomic.Uint64
	PointsRejected  atomic.Uint64
	ExportFailures  atomic.Uint64
	Retries         atomic.Uint64
	Throttles       atomic.Uint64
	AuthFailures    atomic.Uint64
	DroppedBatches  atomic.Uint64
	OversizePoints  atomic.Uint64
}

func (c *Counters) Snapshot() map[string]uint64 {
	return map[string]uint64{
		"accepted_records": c.AcceptedRecords.Load(), "queued_records": c.QueuedRecords.Load(), "retried_records": c.RetriedRecords.Load(), "delivered_records": c.DeliveredRecords.Load(), "corrupt_records": c.CorruptRecords.Load(), "delivery_paused": c.DeliveryPaused.Load(),
		"scrapes": c.Scrapes.Load(), "read_errors": c.ReadErrors.Load(), "queue_rejected": c.QueueRejected.Load(), "batches_accepted": c.BatchesAccepted.Load(), "points_rejected": c.PointsRejected.Load(), "export_failures": c.ExportFailures.Load(), "retries": c.Retries.Load(), "throttles": c.Throttles.Load(), "auth_failures": c.AuthFailures.Load(), "dropped_batches": c.DroppedBatches.Load(), "oversize_points": c.OversizePoints.Load(),
	}
}

// AGENTV1 FILE END
