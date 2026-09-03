// AGENTV1 FILE START: HTTPS OTLP request boundary; no sender or protocol reimplementation.
package exporter

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/agent-i/agent/internal/config"
	"github.com/agent-i/agent/internal/policy"
	"github.com/agent-i/agent/internal/security"
)

func Request(ctx context.Context, cfg config.Config, signal policy.Capability, body io.Reader, secrets security.SecretProvider) (*http.Request, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if signal != policy.Metrics && signal != policy.Logs && signal != policy.Traces {
		return nil, errors.New("invalid OTLP signal")
	}
	if !cfg.Policy.Enabled[signal] {
		return nil, errors.New("signal disabled by policy")
	}
	if body == nil {
		return nil, errors.New("OTLP payload required")
	}
	raw, readErr := io.ReadAll(io.LimitReader(body, int64(cfg.Limits.RequestBytes)+1))
	if readErr != nil || len(raw) > cfg.Limits.RequestBytes || !json.Valid(raw) {
		return nil, errors.New("OTLP JSON payload invalid or exceeds bound")
	}
	if secrets == nil {
		return nil, errors.New("secret provider unavailable")
	}
	auth, err := secrets.Authorization(ctx, cfg.Exporter.HeadersEnv["Authorization"])
	if err != nil {
		return nil, errors.New("ingestion authorization unavailable")
	}
	defer security.Clear(auth)
	if !strings.HasPrefix(string(auth), "ApiKey ") || len(auth) <= 7 || len(auth) > 4096 || strings.ContainsAny(string(auth[7:]), " \t\r\n\x00") {
		return nil, errors.New("ingestion authorization invalid")
	}
	req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimSuffix(cfg.Exporter.Endpoint, "/")+"/v1/"+string(signal), bytes.NewReader(raw))
	if err != nil {
		return nil, errors.New("cannot construct OTLP request")
	}
	req.Header.Set("Authorization", string(auth))
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

// Client is a contract helper, not an active exporter. No network until a caller uses Do.
// Redirects are refused so ingestion credentials cannot cross hosts.
// AGENTV1 START: sentinel lets durable delivery pause permanent endpoint configuration errors.
var errRedirectRefused = errors.New("ingestion redirect refused")

// AGENTV1 END: redirect classification
func Client() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	return &http.Client{Timeout: 15 * time.Second, Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return errRedirectRefused }}
}

// AGENTV1 FILE END
