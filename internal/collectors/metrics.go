// AGENTV1 FILE START: metrics-only collector owns readers, one bounded queue and one sender.
package collectors

import (
	"context"
	"errors"
	"github.com/agent-i/agent/internal/cloud"
	"github.com/agent-i/agent/internal/config"
	"github.com/agent-i/agent/internal/exporter"
	"github.com/agent-i/agent/internal/identity"
	"github.com/agent-i/agent/internal/pipeline"
	"github.com/agent-i/agent/internal/platform"
	"github.com/agent-i/agent/internal/queue"
	"github.com/agent-i/agent/internal/security"
	"github.com/agent-i/agent/internal/selftelemetry"
	"github.com/agent-i/agent/internal/version"
	"math/rand/v2"
	"net/http"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

type metricSender interface {
	Send(context.Context, []byte) exporter.Outcome
}
type Metrics struct {
	cfg    config.Config
	stats  *selftelemetry.Counters
	reader platform.HostMetrics
	sender metricSender
	// Private injection seam for a certificate-pinned localhost test server.
	httpClient *http.Client
	detector   cloud.Detector
	resource   map[string]string
	// AGENTV1 START: acknowledgement-based durable delivery.
	queue queue.Durable
	// AGENTV1 END: acknowledgement-based durable delivery
	cancel   context.CancelFunc
	done     chan struct{}
	blocked  atomic.Bool
	finalize sync.Once
}

func NewMetrics(cfg config.Config, stats *selftelemetry.Counters) *Metrics {
	return &Metrics{cfg: cfg, stats: stats}
}
func (m *Metrics) Start(ctx context.Context) error {
	if !m.cfg.Policy.Enabled["metrics"] {
		return errors.New("metrics capability disabled")
	}
	if err := m.cfg.Validate(); err != nil {
		return err
	}
	// AGENTV1 START: fail before identity/network/state work if deployment identity is missing.
	if err := m.cfg.ValidateQueueIdentity(); err != nil {
		return err
	}
	// AGENTV1 END: logical scope preflight
	limits, ec2, delivery := m.cfg.Runtime()
	// Fail unsupported OS before metadata, secrets or host reads.
	reader, err := platform.NewHostMetrics(limits)
	if err != nil {
		return err
	}
	var evidence cloud.Evidence
	if *ec2.Enabled {
		detector := m.detector
		if detector == nil {
			detector = cloud.NewEC2(time.Duration(ec2.TimeoutSeconds) * time.Second)
		}
		evidence, err = detector.Detect(ctx)
		if err != nil && (ec2.Required || platform.EC2Expected() || errors.Is(err, cloud.ErrInvalid)) {
			return errors.New("EC2 identity required but unavailable")
		}
	}
	resolved, err := identity.Resolve(ctx, m.cfg.AgentID, evidence, platform.Native{})
	if err != nil {
		return err
	}
	if err = resolved.RequireHost(); err != nil {
		return err
	}
	// AGENTV1 START: YAML inline/reference credentials use the same ApiKey exporter contract.
	secret := m.cfg.SecretProvider()
	// AGENTV1 END: unified credential provider
	auth, err := secret.Authorization(ctx, m.cfg.Exporter.HeadersEnv["Authorization"])
	if err != nil {
		return errors.New("ingestion authorization unavailable")
	}
	security.Clear(auth)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	m.reader = reader
	m.sender = exporter.NewSender(m.cfg, secret, m.stats, m.httpClient)
	m.resource = resolved.Resource("linux", runtime.GOARCH, version.Version, "")
	// AGENTV1 START: retained memory queue implementation remains available for tests;
	// production now spools to private disk before export instead of destructive Pop.
	// AGENTV1 START: endpoint/key excluded; original URL is verification-only for v1 migration.
	previous := m.cfg.Exporter.PreviousEndpoint
	if previous == "" {
		previous = m.cfg.Exporter.Endpoint
	}
	scope := queue.Scope{BackendID: m.cfg.Exporter.BackendID, OrganizationID: m.cfg.Exporter.OrganizationID, HostID: m.resource["host.id"], Account: m.resource["cloud.account.id"], Region: m.resource["cloud.region"]}
	spool, err := queue.OpenScopedDisk(delivery.StateDirectory, scope, previous, m.cfg.Limits.QueueBytes, delivery.QueueItems)
	// AGENTV1 END: stable queue scope
	if err != nil {
		return err
	}
	m.queue = spool
	m.stats.CorruptRecords.Store(spool.Corrupt())
	// AGENTV1 END: durable production spool
	return m.launch(ctx, time.Duration(limits.IntervalSeconds)*time.Second)
}
func (m *Metrics) launch(parent context.Context, interval time.Duration) error {
	ctx, cancel := context.WithCancel(parent)
	m.cancel = cancel
	m.done = make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		timer := time.NewTicker(interval)
		defer timer.Stop()
		m.scrape(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				m.scrape(ctx)
			}
		}
	}()
	go func() {
		defer wg.Done()
		for {
			// AGENTV1 START: peek retains FIFO head until remote acceptance + durable ack.
			item, err := m.queue.Next(ctx)
			if err != nil {
				if ctx.Err() == nil {
					m.stats.ExportFailures.Add(1)
					m.stats.DeliveryPaused.Store(1)
					m.blocked.Store(true)
				}
				return
			}
			if spool, ok := m.queue.(*queue.Disk); ok {
				m.stats.CorruptRecords.Store(spool.Corrupt())
			}
			outcome := m.sender.Send(ctx, item.Data)
			switch outcome {
			case exporter.Accepted:
				if err = m.queue.Ack(ctx, item.Receipt); err != nil {
					m.stats.ExportFailures.Add(1)
					m.blocked.Store(true)
					return
				}
				m.stats.DeliveredRecords.Add(1)
			case exporter.Unauthorized, exporter.Rejected:
				// Pause until explicit operator restart after remediation. Retain even
				// partially accepted batches; do not blindly replay their accepted subset.
				m.blocked.Store(true)
				m.stats.DeliveryPaused.Store(1)
				return
			case exporter.Cancelled:
				return
			case exporter.Exhausted:
				m.stats.RetriedRecords.Add(1)
				timer := time.NewTimer(30*time.Second + time.Duration(rand.IntN(1000))*time.Millisecond)
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
			}
			// AGENTV1 END: acceptance-only dequeue
		}
	}()
	go func() { wg.Wait(); close(m.done) }()
	return nil
}
func (m *Metrics) scrape(ctx context.Context) {
	if m.blocked.Load() || ctx.Err() != nil {
		return
	}
	if spool, ok := m.queue.(*queue.Disk); ok {
		m.stats.CorruptRecords.Store(spool.Corrupt())
	}
	snapshot, err := m.reader.Sample(ctx)
	m.stats.Scrapes.Add(1)
	m.stats.ReadErrors.Add(uint64(len(snapshot.Issues)))
	if err != nil {
		m.stats.ReadErrors.Add(1)
		return
	}
	seenIssues := map[string]bool{}
	for _, issue := range snapshot.Issues {
		key := issue.Capability + ":" + issue.Code
		if seenIssues[key] {
			continue
		}
		seenIssues[key] = true
		snapshot.Values = append(snapshot.Values, platform.Measurement{Name: "host.agent.collection.issue", Unit: "1", Kind: "gauge", Value: 1, Attributes: map[string]string{"collector": issue.Capability, "reason": issue.Code}})
	}
	// Self-telemetry remains host-scoped; no additional service entity is created.
	for name, value := range m.stats.Snapshot() {
		snapshot.Values = append(snapshot.Values, platform.Measurement{Name: "host.agent." + name, Unit: "{events}", Kind: "gauge", Value: float64(value)})
	}
	_, _, d := m.cfg.Runtime()
	batches, err := pipeline.Batches(snapshot, m.resource, d.BatchPoints, m.cfg.Limits.RequestBytes)
	if err != nil {
		m.stats.OversizePoints.Add(1)
		return
	}
	for _, batch := range batches {
		// AGENTV1 START: persist before counting queued; reject_new is explicit loss.
		m.stats.AcceptedRecords.Add(1)
		if _, err = m.queue.Put(ctx, batch); err != nil {
			m.stats.QueueRejected.Add(1)
			m.stats.DroppedBatches.Add(1)
			if !errors.Is(err, queue.ErrFull) && ctx.Err() == nil {
				m.stats.ExportFailures.Add(1)
				m.stats.DeliveryPaused.Store(1)
				m.blocked.Store(true)
				return
			}
		} else {
			m.stats.QueuedRecords.Add(1)
		}
		// AGENTV1 END: durable enqueue counters
	}
}
func (m *Metrics) Stop(ctx context.Context) error {
	if m.cancel == nil {
		return nil
	}
	m.cancel()
	select {
	case <-m.done:
		// AGENTV1 START: retain backlog through shutdown, release lock after workers stop.
		m.finalize.Do(func() { _ = m.queue.Close(context.Background()) })
		// AGENTV1 END: retained backlog
		return nil
	case <-ctx.Done():
		return errors.New("metric workers did not stop before deadline")
	}
}

// AGENTV1 FILE END
