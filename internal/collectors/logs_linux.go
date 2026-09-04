//go:build linux

// AGENTV1 FILE START: independent Linux file Logs lifecycle, durable admission and delivery.
package collectors

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/agent-i/agent/internal/cloud"
	"github.com/agent-i/agent/internal/config"
	"github.com/agent-i/agent/internal/exporter"
	"github.com/agent-i/agent/internal/identity"
	"github.com/agent-i/agent/internal/journaltail"
	"github.com/agent-i/agent/internal/logtail"
	"github.com/agent-i/agent/internal/pipeline"
	"github.com/agent-i/agent/internal/platform"
	"github.com/agent-i/agent/internal/policy"
	"github.com/agent-i/agent/internal/queue"
	"github.com/agent-i/agent/internal/security"
	"github.com/agent-i/agent/internal/selftelemetry"
	"github.com/agent-i/agent/internal/version"
)

type Logs struct {
	cfg        config.Config
	stats      *selftelemetry.LogCounters
	delivery   *selftelemetry.Counters
	detector   cloud.Detector
	httpClient *http.Client
	queue      *queue.LogDisk
	tailers    []*logtail.Tailer
	journal    *journaltail.Reader
	cancel     context.CancelFunc
	done       chan struct{}
	blocked    atomic.Bool
	resource   map[string]string
	mu         sync.Mutex
}

func NewLogs(cfg config.Config, stats *selftelemetry.LogCounters, delivery *selftelemetry.Counters) *Logs {
	return &Logs{cfg: cfg, stats: stats, delivery: delivery}
}

func (l *Logs) Start(parent context.Context) error {
	if !l.cfg.Policy.Enabled[policy.Logs] {
		return errors.New("logs capability disabled")
	}
	if err := l.cfg.Validate(); err != nil {
		return err
	}
	if err := l.cfg.ValidateQueueIdentity(); err != nil {
		return err
	}
	_, ec2, _ := l.cfg.Runtime()
	var evidence cloud.Evidence
	var err error
	if *ec2.Enabled {
		detector := l.detector
		if detector == nil {
			detector = cloud.NewEC2(time.Duration(ec2.TimeoutSeconds) * time.Second)
		}
		evidence, err = detector.Detect(parent)
		if err != nil && (ec2.Required || platform.EC2Expected() || errors.Is(err, cloud.ErrInvalid)) {
			return errors.New("EC2 identity required but unavailable")
		}
	}
	resolved, err := identity.Resolve(parent, l.cfg.AgentID, evidence, platform.Native{})
	if err != nil {
		return err
	}
	if err = resolved.RequireHost(); err != nil {
		return err
	}
	secret := l.cfg.SecretProvider()
	auth, err := secret.Authorization(parent, l.cfg.Exporter.HeadersEnv["Authorization"])
	if err != nil {
		return errors.New("ingestion authorization unavailable")
	}
	security.Clear(auth)
	l.resource = resolved.Resource("linux", runtime.GOARCH, version.Version, "")
	scope := queue.Scope{BackendID: l.cfg.Exporter.BackendID, OrganizationID: l.cfg.Exporter.OrganizationID, HostID: l.resource["host.id"], Account: l.resource["cloud.account.id"], Region: l.resource["cloud.region"]}
	logsCfg := l.cfg.LogsRuntime()
	if len(logsCfg.Sources) == 0 && !logsCfg.Journald.Enabled {
		return errors.New("logs enabled without file or journald sources")
	}
	spool, err := queue.OpenLogDisk(filepath.Join(logsCfg.StateDirectory, "queue"), scope, logsCfg.QueueBytes, logsCfg.QueueItems)
	if err != nil {
		return err
	}
	l.queue = spool
	ctx, cancel := context.WithCancel(parent)
	l.cancel = cancel
	l.done = make(chan struct{})
	sender := exporter.NewSignalSender(l.cfg, policy.Logs, secret, l.delivery, l.httpClient)
	go l.deliver(ctx, sender)
	checkpointDir := filepath.Join(logsCfg.StateDirectory, "checkpoints")
	for _, source := range logsCfg.Sources {
		source := source
		tailer := logtail.New(source, checkpointDir, logsCfg.MaxFiles, time.Duration(logsCfg.PollIntervalMillis)*time.Millisecond, func(ctx context.Context, record logtail.Record) error {
			raw, err := pipeline.LogJSON(pipeline.LogRecord{Body: record.Body, SourceID: source.ID, RelativePath: record.RelativePath, FileIdentity: record.FileIdentity, StartOffset: record.StartOffset, EndOffset: record.EndOffset, ObservedAt: record.ObservedAt, ServiceName: source.ServiceName, Environment: source.Environment}, l.resource, l.cfg.Limits.RequestBytes)
			if err != nil {
				l.stats.RecordsRejectedLocal.Add(1)
				return nil
			}
			_, existed, err := l.queue.PutAdmission(ctx, record.AdmissionID, raw)
			if err != nil {
				l.stats.QueueRejected.Add(1)
				return err
			}
			if !existed {
				l.stats.RecordsQueued.Add(1)
			}
			return nil
		}, l.queue.Activate)
		if err = tailer.Start(ctx); err != nil {
			l.stats.PermissionErrors.Add(1)
			continue
		}
		l.tailers = append(l.tailers, tailer)
	}
	if logsCfg.Journald.Enabled {
		journal := journaltail.New(logsCfg.Journald, checkpointDir, func(ctx context.Context, record journaltail.Record) error {
			raw, err := pipeline.LogJSON(pipeline.LogRecord{Body: record.Body, SourceID: "journald", SourceType: "journald", ObservedAt: record.ObservedAt, ServiceName: record.ServiceName, SeverityText: record.SeverityText, Attributes: record.Attributes}, l.resource, l.cfg.Limits.RequestBytes)
			if err != nil {
				l.stats.RecordsRejectedLocal.Add(1)
				return nil
			}
			_, existed, err := l.queue.PutAdmission(ctx, record.AdmissionID, raw)
			if err != nil {
				l.stats.QueueRejected.Add(1)
				return err
			}
			if !existed {
				l.stats.RecordsQueued.Add(1)
			}
			return nil
		}, l.queue.Activate)
		if err = journal.Start(ctx); err != nil {
			l.stats.PermissionErrors.Add(1)
		} else {
			l.journal = journal
		}
	}
	if len(l.tailers) == 0 && l.journal == nil {
		cancel()
		<-l.done
		l.queue.Close(context.Background())
		return errors.New("no configured log source could start")
	}
	return nil
}

func (l *Logs) deliver(ctx context.Context, sender *exporter.Sender) {
	defer close(l.done)
	for {
		item, err := l.queue.Next(ctx)
		if err != nil {
			return
		}
		before := l.delivery.PointsRejected.Load()
		outcome := sender.Send(ctx, item.Data)
		switch outcome {
		case exporter.Accepted:
			if err = l.queue.Ack(ctx, item.Receipt); err != nil {
				l.blocked.Store(true)
				return
			}
			if l.delivery.PointsRejected.Load() == before {
				l.stats.RecordsDelivered.Add(1)
			}
		case exporter.Unauthorized:
			l.stats.AuthenticationPaused.Store(1)
			l.blocked.Store(true)
			return
		case exporter.Rejected:
			l.blocked.Store(true)
			return
		case exporter.Cancelled:
			return
		case exporter.Exhausted:
			l.stats.RecordsRetried.Add(1)
			timer := time.NewTimer(30 * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}
}

func (l *Logs) Stop(ctx context.Context) error {
	l.mu.Lock()
	cancel := l.cancel
	l.cancel = nil
	l.mu.Unlock()
	if cancel == nil {
		return nil
	}
	var result error
	for _, tailer := range l.tailers {
		if err := tailer.Stop(ctx); err != nil {
			result = err
		}
	}
	if l.journal != nil {
		if err := l.journal.Stop(ctx); err != nil {
			result = err
		}
	}
	cancel()
	select {
	case <-l.done:
	case <-ctx.Done():
		result = errors.New("logs workers did not stop before deadline")
	}
	if err := l.queue.Close(context.Background()); err != nil && result == nil {
		result = err
	}
	return result
}

func (l *Logs) Snapshot() map[string]uint64 {
	var discovered, open, read, rejected, permissions, checkpoints, multiline uint64
	for _, tailer := range l.tailers {
		s := tailer.Stats()
		discovered += s.FilesDiscovered
		open += s.FilesOpen
		read += s.RecordsRead
		rejected += s.RecordsRejectedLocal
		permissions += s.PermissionErrors
		checkpoints += s.CheckpointErrors
		multiline += s.MultilineFlushes
	}
	if l.journal != nil {
		s := l.journal.Stats()
		read += s.RecordsRead
		rejected += s.RecordsRejectedLocal
		permissions += s.PermissionErrors
		checkpoints += s.CheckpointErrors
		l.stats.JournalRecordsRead.Store(s.RecordsRead)
		l.stats.JournalReaderErrors.Store(s.ReaderErrors)
		l.stats.JournalCheckpointErrors.Store(s.CheckpointErrors)
	}
	l.stats.FilesDiscovered.Store(discovered)
	l.stats.FilesOpen.Store(open)
	l.stats.RecordsRead.Store(read)
	l.stats.RecordsRejectedLocal.Store(rejected)
	l.stats.PermissionErrors.Store(permissions)
	l.stats.CheckpointErrors.Store(checkpoints)
	l.stats.MultilineFlushes.Store(multiline)
	return l.stats.Snapshot(l.delivery)
}

// AGENTV1 FILE END
