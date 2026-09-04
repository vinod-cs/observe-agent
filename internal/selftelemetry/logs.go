// AGENTV1 FILE START: bounded signal-specific file Logs counters.
package selftelemetry

import "sync/atomic"

type LogCounters struct {
	FilesDiscovered, FilesOpen, RecordsRead, RecordsQueued, RecordsDelivered atomic.Uint64
	RecordsRetried, RecordsRejectedLocal, PermissionErrors, CheckpointErrors atomic.Uint64
	QueueRejected, AuthenticationPaused, MultilineFlushes                    atomic.Uint64
	JournalRecordsRead, JournalReaderErrors, JournalCheckpointErrors         atomic.Uint64
}

func (c *LogCounters) Snapshot(delivery *Counters) map[string]uint64 {
	localRejected := c.RecordsRejectedLocal.Load()
	serverRejected := delivery.PointsRejected.Load()
	return map[string]uint64{
		"files_discovered": c.FilesDiscovered.Load(), "files_open": c.FilesOpen.Load(), "records_read": c.RecordsRead.Load(),
		"records_accepted": c.RecordsQueued.Load(), "records_queued": c.RecordsQueued.Load(), "records_delivered": c.RecordsDelivered.Load(), "records_retried": delivery.RetriedRecords.Load() + c.RecordsRetried.Load(),
		"records_dropped": localRejected + serverRejected, "records_rejected_local": localRejected, "records_rejected_server": serverRejected,
		"permission_errors": c.PermissionErrors.Load(), "checkpoint_errors": c.CheckpointErrors.Load(), "queue_rejected": c.QueueRejected.Load(),
		"authentication_paused": c.AuthenticationPaused.Load(), "multiline_flushes": c.MultilineFlushes.Load(),
		"journal_records_read": c.JournalRecordsRead.Load(), "journal_reader_errors": c.JournalReaderErrors.Load(), "journal_checkpoint_errors": c.JournalCheckpointErrors.Load(),
	}
}

// AGENTV1 FILE END
