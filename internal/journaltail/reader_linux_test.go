//go:build linux

// AGENTV1 FILE START: journald cursor, filters, permission and durable-admission ordering tests.
package journaltail

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agent-i/agent/internal/config"
)

const journalFixture = `{"MESSAGE":"accepted body","__CURSOR":"s=fixture;i=1","__REALTIME_TIMESTAMP":"1700000000000000","_SYSTEMD_UNIT":"checkout.service","SYSLOG_IDENTIFIER":"checkout","PRIORITY":"4","_PID":"42","_COMM":"checkoutd","_TRANSPORT":"stdout"}` + "\n"

func privateDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "checkpoints")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestJournalFiltersAreLiteralArguments(t *testing.T) {
	r := New(config.JournaldLogSource{Enabled: true, Units: []string{"sshd.service"}, Identifiers: []string{"checkout"}, Priority: "warning", StartAt: "beginning"}, privateDir(t), nil, nil)
	got := strings.Join(r.arguments(), "\n")
	for _, want := range []string{"--output=json", "--follow", "--lines=all", "--unit=sshd.service", "--identifier=checkout", "--priority=warning"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing literal argument %s in %s", want, got)
		}
	}
}

func TestCursorAdvancesOnlyAfterDurableAdmission(t *testing.T) {
	dir := privateDir(t)
	r := New(config.JournaldLogSource{Enabled: true, StartAt: "beginning"}, dir, func(context.Context, Record) error { return errors.New("queue full") }, nil)
	if err := r.consume(context.Background(), strings.NewReader(journalFixture)); err == nil {
		t.Fatal("queue failure ignored")
	}
	if _, err := os.Stat(r.checkpointPath()); !os.IsNotExist(err) {
		t.Fatal("cursor advanced before queue admission")
	}

	var admitted, activated string
	r = New(config.JournaldLogSource{Enabled: true, StartAt: "beginning"}, dir, func(_ context.Context, record Record) error { admitted = record.AdmissionID; return nil }, func(_ context.Context, id string) error { activated = id; return nil })
	if err := r.consume(context.Background(), strings.NewReader(journalFixture)); err != nil {
		t.Fatal(err)
	}
	if admitted == "" || admitted != activated {
		t.Fatal("admission was not activated after checkpoint")
	}
	if err := r.loadCheckpoint(); err != nil || r.cp.Cursor != "s=fixture;i=1" {
		t.Fatalf("cursor not durable: %+v %v", r.cp, err)
	}
}

func TestCursorResumeAndRestartAdmissionRecovery(t *testing.T) {
	dir := privateDir(t)
	r := New(config.JournaldLogSource{Enabled: true, StartAt: "end"}, dir, func(context.Context, Record) error { return nil }, func(context.Context, string) error { return nil })
	if err := r.consume(context.Background(), strings.NewReader(journalFixture)); err != nil {
		t.Fatal(err)
	}
	var recovered string
	restarted := New(config.JournaldLogSource{Enabled: true, StartAt: "end"}, dir, func(context.Context, Record) error { return nil }, func(_ context.Context, id string) error { recovered = id; return nil })
	blockRead, blockWrite := io.Pipe()
	restarted.start = func(context.Context, []string) (*process, error) {
		return &process{stdout: blockRead, wait: func() error { return nil }}, nil
	}
	if err := restarted.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if recovered == "" {
		t.Fatal("durable unready admission was not recovered")
	}
	if !contains(restarted.arguments(), "--after-cursor=s=fixture;i=1") {
		t.Fatal("restart did not use journal cursor")
	}
	_ = blockWrite.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := restarted.Stop(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestStartAtEndPersistsCrashSafeInitialWatermark(t *testing.T) {
	dir := privateDir(t)
	r := New(config.JournaldLogSource{Enabled: true, StartAt: "end"}, dir, func(context.Context, Record) error { return nil }, nil)
	r.now = func() time.Time { return time.Unix(1700000000, 123).UTC() }
	blockRead, blockWrite := io.Pipe()
	r.start = func(context.Context, []string) (*process, error) {
		return &process{stdout: blockRead, wait: func() error { return nil }}, nil
	}
	if err := r.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !contains(r.arguments(), "--since=@1700000000") {
		t.Fatalf("missing durable start watermark: %v", r.arguments())
	}
	_ = blockWrite.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := r.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	restarted := New(config.JournaldLogSource{Enabled: true, StartAt: "end"}, dir, nil, nil)
	if err := restarted.loadCheckpoint(); err != nil || restarted.cp.SinceMicros == 0 {
		t.Fatalf("watermark not retained: %+v %v", restarted.cp, err)
	}
}

func TestJournalRecordNormalization(t *testing.T) {
	record, cursor, ok := parseRecord([]byte(strings.TrimSpace(journalFixture)), time.Unix(1, 0))
	if !ok || cursor != "s=fixture;i=1" || record.Body != "accepted body" || record.ServiceName != "checkout" || record.SeverityText != "WARN" {
		t.Fatalf("unexpected record: %+v", record)
	}
	if record.Attributes["systemd.unit"] != "checkout.service" || record.Attributes["process.pid"] != "42" {
		t.Fatal("bounded journal metadata missing")
	}
}

func TestPermissionDeniedIsCountedWithoutBodyDiagnostics(t *testing.T) {
	r := New(config.JournaldLogSource{Enabled: true, StartAt: "end"}, privateDir(t), func(context.Context, Record) error { return nil }, nil)
	r.retryDelay = 5 * time.Millisecond
	r.start = func(context.Context, []string) (*process, error) { return nil, os.ErrPermission }
	ctx, cancel := context.WithCancel(context.Background())
	if err := r.Start(ctx); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for r.Stats().PermissionErrors == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	cancel()
	stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Second)
	defer stopCancel()
	if err := r.Stop(stopCtx); err != nil {
		t.Fatal(err)
	}
	if r.Stats().PermissionErrors == 0 {
		t.Fatal("permission denial not counted")
	}
}

func TestDisabledJournalStartsNoProcess(t *testing.T) {
	called := false
	r := New(config.JournaldLogSource{Enabled: false}, filepath.Join(t.TempDir(), "must-not-exist"), nil, nil)
	r.start = func(context.Context, []string) (*process, error) { called = true; return nil, nil }
	if err := r.Start(context.Background()); err == nil {
		t.Fatal("disabled journal started")
	}
	if called {
		t.Fatal("disabled journal launched a process")
	}
	if _, err := os.Stat(r.checkpointDir); !os.IsNotExist(err) {
		t.Fatal("disabled journal touched checkpoint storage")
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// AGENTV1 FILE END
