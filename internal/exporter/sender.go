// AGENTV1 FILE START: bounded TLS OTLP delivery, retry classification and redacted failures.
package exporter

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"github.com/agent-i/agent/internal/config"
	"github.com/agent-i/agent/internal/policy"
	"github.com/agent-i/agent/internal/security"
	"github.com/agent-i/agent/internal/selftelemetry"
	"io"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Outcome string

const (
	Accepted     Outcome = "accepted"
	Rejected     Outcome = "rejected"
	Unauthorized Outcome = "unauthorized"
	Exhausted    Outcome = "exhausted"
	Cancelled    Outcome = "cancelled"
)

type Sender struct {
	cfg     config.Config
	client  *http.Client
	secrets security.SecretProvider
	stats   *selftelemetry.Counters
	sleep   func(context.Context, time.Duration) bool
}

func NewSender(cfg config.Config, secrets security.SecretProvider, stats *selftelemetry.Counters, client *http.Client) *Sender {
	if client == nil {
		client = Client()
	}
	copy := *client
	_, _, d := cfg.Runtime()
	copy.Timeout = time.Duration(d.RequestTimeoutSeconds) * time.Second
	copy.CheckRedirect = Client().CheckRedirect
	return &Sender{cfg, &copy, secrets, stats, wait}
}
func wait(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
func retryDelay(header string, attempt int, now time.Time) time.Duration {
	if seconds, e := strconv.ParseInt(header, 10, 64); e == nil && seconds >= 0 {
		// Honor server delay; do not retry earlier by clamping Retry-After.
		// The single timer is cancellation-aware and the queue remains bounded.
		const maxDuration = time.Duration(1<<63 - 1)
		if seconds > int64(maxDuration/time.Second) {
			return maxDuration
		}
		return time.Duration(seconds) * time.Second
	}
	if date, e := http.ParseTime(header); e == nil && date.After(now) {
		return date.Sub(now)
	}
	return min(time.Duration(1<<attempt)*time.Second, 30*time.Second) + time.Duration(rand.IntN(500))*time.Millisecond
}
func (s *Sender) Send(ctx context.Context, payload []byte) Outcome {
	_, _, options := s.cfg.Runtime()
	for attempt := 0; attempt < options.MaxAttempts; attempt++ {
		// AGENTV1 START: retry attempts count the retained serialized record.
		if attempt > 0 {
			s.stats.RetriedRecords.Add(1)
		}
		// AGENTV1 END: retry record accounting
		if ctx.Err() != nil {
			return Cancelled
		}
		req, err := Request(ctx, s.cfg, policy.Metrics, bytes.NewReader(payload), s.secrets)
		if err != nil {
			s.stats.AuthFailures.Add(1)
			s.stats.ExportFailures.Add(1)
			return Unauthorized
		}
		response, err := s.client.Do(req)
		req.Header.Del("Authorization")
		if err != nil {
			s.stats.ExportFailures.Add(1)
			// AGENTV1 START: invalid trust/hostname or redirect is remediation, not an endless partition retry.
			var certificate *tls.CertificateVerificationError
			var authority x509.UnknownAuthorityError
			var hostname x509.HostnameError
			if errors.Is(err, errRedirectRefused) || errors.As(err, &certificate) || errors.As(err, &authority) || errors.As(err, &hostname) {
				return Rejected
			}
			// AGENTV1 END: permanent TLS/configuration classification
			if ctx.Err() != nil {
				return Cancelled
			}
			if attempt+1 == options.MaxAttempts {
				return Exhausted
			}
			s.stats.Retries.Add(1)
			if !s.sleep(ctx, retryDelay("", attempt, time.Now())) {
				return Cancelled
			}
			continue
		}
		raw, readErr := io.ReadAll(io.LimitReader(response.Body, 65537))
		response.Body.Close()
		status := response.StatusCode
		if status == 401 || status == 403 {
			s.stats.AuthFailures.Add(1)
			s.stats.ExportFailures.Add(1)
			return Unauthorized
		}
		if status >= 200 && status < 300 {
			if readErr != nil || len(raw) > 65536 {
				s.stats.ExportFailures.Add(1)
				return Rejected
			}
			if len(bytes.TrimSpace(raw)) > 0 {
				// Support standard OTLP response and the existing Observe data envelope.
				var body map[string]json.RawMessage
				if json.Unmarshal(raw, &body) != nil {
					s.stats.ExportFailures.Add(1)
					return Rejected
				}
				if wrapped, ok := body["data"]; ok {
					if json.Unmarshal(wrapped, &body) != nil {
						s.stats.ExportFailures.Add(1)
						return Rejected
					}
				}
				if part, ok := body["partialSuccess"]; ok {
					var partial struct {
						Rejected json.Number `json:"rejectedDataPoints"`
					}
					if json.Unmarshal(part, &partial) != nil {
						s.stats.ExportFailures.Add(1)
						return Rejected
					}
					if partial.Rejected != "" {
						n, e := strconv.ParseUint(string(partial.Rejected), 10, 64)
						if e != nil {
							s.stats.ExportFailures.Add(1)
							return Rejected
						}
						if n > 0 {
							s.stats.PointsRejected.Add(n)
							s.stats.ExportFailures.Add(1)
							return Rejected
						}
					}
				}
			}
			s.stats.BatchesAccepted.Add(1)
			return Accepted
		}
		s.stats.ExportFailures.Add(1)
		retry := status == 429 || (status >= 500 && status != 501 && status != 505)
		if status == 429 {
			s.stats.Throttles.Add(1)
		}
		if !retry {
			return Rejected
		}
		// AGENTV1 START: honor Retry-After even at the last per-cycle attempt,
		// preventing the persistent worker from retrying earlier on its next cycle.
		if attempt+1 == options.MaxAttempts {
			if !s.sleep(ctx, retryDelay(strings.TrimSpace(response.Header.Get("Retry-After")), attempt, time.Now())) {
				return Cancelled
			}
			return Exhausted
		}
		// AGENTV1 END: persistent Retry-After boundary
		s.stats.Retries.Add(1)
		if !s.sleep(ctx, retryDelay(strings.TrimSpace(response.Header.Get("Retry-After")), attempt, time.Now())) {
			return Cancelled
		}
	}
	return Exhausted
}

// AGENTV1 FILE END
