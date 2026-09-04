//go:build linux

// AGENTV1 FILE START: Linux Logs runtime, disabled-I/O, authentication and queue isolation tests.
package collectors

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/agent-i/agent/internal/cloud"
	"github.com/agent-i/agent/internal/config"
	"github.com/agent-i/agent/internal/policy"
	"github.com/agent-i/agent/internal/selftelemetry"
)

type fixedLogDetector struct{}

func (fixedLogDetector) Detect(context.Context) (cloud.Evidence, error) {
	return cloud.Evidence{Verified: true, Provider: "aws", Platform: "aws_ec2", Account: "123456789012", Region: "us-east-2", InstanceID: "i-0123456789abcdef0", ResourceID: "i-0123456789abcdef0"}, nil
}

func logsConfig(t *testing.T, server *httptest.Server, root, state string) config.Config {
	t.Helper()
	on := true
	return config.Config{AgentID: "fixture-agent", Exporter: config.Exporter{Type: "otlp_http", Endpoint: server.URL + "/api/v1/otlp", BackendID: "backend", OrganizationID: "org", HeadersEnv: map[string]string{"Authorization": "LOG_TEST_KEY"}}, Policy: policy.Document{Version: 1, Enabled: map[policy.Capability]bool{policy.Logs: true}}, EC2: config.EC2Metadata{Enabled: &on, TimeoutSeconds: 1}, Limits: config.Limits{RequestBytes: 1 << 20, QueueBytes: 1 << 20, MemoryMiB: 64, ShutdownSeconds: 5}, Logs: config.LogsConfig{StateDirectory: state, QueueBytes: 1 << 20, QueueItems: 16, PollIntervalMillis: 200, MaxFiles: 16, Sources: []config.LogSource{{ID: "app", Root: root, Include: []string{"*.log"}, StartAt: "beginning", MaxOpenFiles: 4, MaxLineBytes: 4096, Multiline: config.LogMultiline{FlushTimeoutMillis: 100, MaxLines: 20, MaxBytes: 4096}}}}}
}

func TestLogsDisabledPerformsNoFilesystemIO(t *testing.T) {
	off := false
	state := filepath.Join(t.TempDir(), "must-not-exist")
	cfg := config.Config{Policy: policy.Document{Version: 1, Enabled: map[policy.Capability]bool{policy.Logs: false}}, EC2: config.EC2Metadata{Enabled: &off}, Logs: config.LogsConfig{StateDirectory: state}}
	logs := NewLogs(cfg, &selftelemetry.LogCounters{}, &selftelemetry.Counters{})
	if logs.Start(context.Background()) == nil {
		t.Fatal("disabled Logs started")
	}
	if _, err := os.Stat(state); !os.IsNotExist(err) {
		t.Fatal("disabled Logs touched state")
	}
}

func TestLogsEndToEndAndPartialSuccessAck(t *testing.T) {
	received := make(chan string, 1)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		received <- string(raw)
		w.WriteHeader(201)
		_, _ = w.Write([]byte(`{"partialSuccess":{"rejectedLogRecords":0}}`))
	}))
	defer server.Close()
	t.Setenv("LOG_TEST_KEY", "ApiKey fixture-only")
	root, state := t.TempDir(), t.TempDir()
	os.Chmod(state, 0700)
	os.WriteFile(filepath.Join(root, "app.log"), []byte("hello\n"), 0600)
	stats, delivery := &selftelemetry.LogCounters{}, &selftelemetry.Counters{}
	logs := NewLogs(logsConfig(t, server, root, state), stats, delivery)
	logs.detector = fixedLogDetector{}
	logs.httpClient = server.Client()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := logs.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer logs.Stop(context.Background())
	select {
	case raw := <-received:
		if !strings.Contains(raw, "hello") || !strings.Contains(raw, "resourceLogs") {
			t.Fatal("invalid OTLP Logs")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("log was not delivered")
	}
	deadline := time.Now().Add(3 * time.Second)
	for stats.RecordsDelivered.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if stats.RecordsDelivered.Load() != 1 {
		t.Fatal("delivery not acknowledged")
	}
}

func TestLogsUnauthorizedRetainsBacklog(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(401) }))
	defer server.Close()
	t.Setenv("LOG_TEST_KEY", "ApiKey fixture-only")
	root, state := t.TempDir(), t.TempDir()
	os.Chmod(state, 0700)
	os.WriteFile(filepath.Join(root, "app.log"), []byte("retain-me\n"), 0600)
	stats := &selftelemetry.LogCounters{}
	logs := NewLogs(logsConfig(t, server, root, state), stats, &selftelemetry.Counters{})
	logs.detector = fixedLogDetector{}
	logs.httpClient = server.Client()
	ctx, cancel := context.WithCancel(context.Background())
	if err := logs.Start(ctx); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for stats.AuthenticationPaused.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	_ = logs.Stop(context.Background())
	if stats.AuthenticationPaused.Load() != 1 {
		t.Fatal("authentication did not pause")
	}
	matches, _ := filepath.Glob(filepath.Join(state, "queue", "*.lrec"))
	if len(matches) != 1 {
		t.Fatalf("backlog not retained: %v", matches)
	}
}

func TestLogsPartialSuccessAcknowledgesWholeDurableRecord(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(201)
		_, _ = w.Write([]byte(`{"partialSuccess":{"rejectedLogRecords":1}}`))
	}))
	defer server.Close()
	t.Setenv("LOG_TEST_KEY", "ApiKey fixture-only")
	root, state := t.TempDir(), t.TempDir()
	os.Chmod(state, 0700)
	os.WriteFile(filepath.Join(root, "app.log"), []byte("server-will-reject\n"), 0600)
	stats, delivery := &selftelemetry.LogCounters{}, &selftelemetry.Counters{}
	logs := NewLogs(logsConfig(t, server, root, state), stats, delivery)
	logs.detector = fixedLogDetector{}
	logs.httpClient = server.Client()
	ctx, cancel := context.WithCancel(context.Background())
	if err := logs.Start(ctx); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for delivery.PointsRejected.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	_ = logs.Stop(context.Background())
	if delivery.PointsRejected.Load() != 1 || stats.RecordsDelivered.Load() != 0 {
		t.Fatalf("partial success accounting rejected=%d delivered=%d", delivery.PointsRejected.Load(), stats.RecordsDelivered.Load())
	}
	matches, _ := filepath.Glob(filepath.Join(state, "queue", "*.lrec"))
	if len(matches) != 0 {
		t.Fatalf("partial-success batch was retried: %v", matches)
	}
}

func TestLogsRuntimeResourceBound(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(201)
		_, _ = w.Write([]byte(`{"partialSuccess":{"rejectedLogRecords":0}}`))
	}))
	defer server.Close()
	t.Setenv("LOG_TEST_KEY", "ApiKey fixture-only")
	root, state := t.TempDir(), t.TempDir()
	os.Chmod(state, 0700)
	os.WriteFile(filepath.Join(root, "app.log"), nil, 0600)
	baselineGoroutines := runtime.NumGoroutine()
	baselineFDs, _ := os.ReadDir("/proc/self/fd")
	logs := NewLogs(logsConfig(t, server, root, state), &selftelemetry.LogCounters{}, &selftelemetry.Counters{})
	logs.detector = fixedLogDetector{}
	logs.httpClient = server.Client()
	ctx, cancel := context.WithCancel(context.Background())
	if err := logs.Start(ctx); err != nil {
		t.Fatal(err)
	}
	time.Sleep(400 * time.Millisecond)
	activeGoroutines := runtime.NumGoroutine()
	activeFDs, _ := os.ReadDir("/proc/self/fd")
	// The delivery worker may already have completed an accepted one-record
	// batch; the tailer remains active, so one bounded worker is sufficient.
	if delta := activeGoroutines - baselineGoroutines; delta < 1 || delta > 6 {
		t.Fatalf("unexpected Logs goroutine delta %d", delta)
	}
	if delta := len(activeFDs) - len(baselineFDs); delta < 2 || delta > 8 {
		t.Fatalf("unexpected Logs file-descriptor delta %d", delta)
	}
	t.Logf("Logs runtime delta: goroutines=%d file_descriptors=%d", activeGoroutines-baselineGoroutines, len(activeFDs)-len(baselineFDs))
	cancel()
	if err := logs.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

// AGENTV1 FILE END
