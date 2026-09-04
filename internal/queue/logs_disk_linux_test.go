//go:build linux

// AGENTV1 FILE START: Logs spool scope, FIFO and crash-window admission regression tests.
package queue

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func testLogScope() Scope {
	return Scope{BackendID: "backend", OrganizationID: "org", HostID: "host", Account: "123", Region: "us-east-2"}
}

func TestLogDiskRecoversInterruptedAdmissionAndActivation(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	id := hashScope("recover")
	q, err := OpenLogDisk(dir, testLogScope(), 1<<20, 8)
	if err != nil {
		t.Fatal(err)
	}
	receipt, _, err := q.PutAdmission(context.Background(), id, []byte(`{"resourceLogs":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if err = q.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Simulate a crash after manifest reservation but before the pending record
	// was renamed into its sequence filename.
	if err = os.Rename(filepath.Join(dir, string(receipt)), filepath.Join(dir, ".pending")); err != nil {
		t.Fatal(err)
	}
	q, err = OpenLogDisk(dir, testLogScope(), 1<<20, 8)
	if err != nil {
		t.Fatal(err)
	}
	if err = q.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	// Simulate a crash after the ready replacement was fsynced but before its
	// atomic rename over the unready admitted record.
	raw, err := os.ReadFile(filepath.Join(dir, string(receipt)))
	if err != nil {
		t.Fatal(err)
	}
	var record logRecord
	if err = json.Unmarshal(raw, &record); err != nil {
		t.Fatal(err)
	}
	record.Ready = true
	replacement, _ := json.Marshal(record)
	if err = os.WriteFile(filepath.Join(dir, ".replace.pending"), replacement, 0600); err != nil {
		t.Fatal(err)
	}
	q, err = OpenLogDisk(dir, testLogScope(), 1<<20, 8)
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close(context.Background())
	item, err := q.Next(context.Background())
	if err != nil || item.Receipt != receipt {
		t.Fatalf("recovered item=%v error=%v", item.Receipt, err)
	}
}

func TestLogDiskRecoversInterruptedManifestReplacement(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	q, err := OpenLogDisk(dir, testLogScope(), 1<<20, 8)
	if err != nil {
		t.Fatal(err)
	}
	if err = q.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(filepath.Join(dir, "manifest.json"), filepath.Join(dir, ".manifest.pending")); err != nil {
		t.Fatal(err)
	}
	q, err = OpenLogDisk(dir, testLogScope(), 1<<20, 8)
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close(context.Background())
}

func TestLogDiskAdmissionSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	q, err := OpenLogDisk(dir, testLogScope(), 1<<20, 8)
	if err != nil {
		t.Fatal(err)
	}
	id := hashScope("admission")
	first, existed, err := q.PutAdmission(context.Background(), id, []byte(`{"resourceLogs":[]}`))
	if err != nil || existed {
		t.Fatal("first admission")
	}
	second, existed, err := q.PutAdmission(context.Background(), id, []byte(`{"resourceLogs":[{"different":true}]}`))
	if err != nil || !existed || second != first {
		t.Fatal("duplicate admission was not idempotent")
	}
	if err = q.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	q, err = OpenLogDisk(dir, testLogScope(), 1<<20, 8)
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close(context.Background())
	third, existed, err := q.PutAdmission(context.Background(), id, []byte(`{"resourceLogs":[]}`))
	if err != nil || !existed || third != first {
		t.Fatal("admission identity not durable")
	}
	if err = q.Activate(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	item, err := q.Next(context.Background())
	if err != nil || item.Receipt != first {
		t.Fatal("FIFO replay")
	}
	if err = q.Ack(context.Background(), item.Receipt); err != nil {
		t.Fatal(err)
	}
}

func TestLogDiskIndependentScopeAndQuota(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	q, err := OpenLogDisk(dir, testLogScope(), 32768, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close(context.Background())
	if _, _, err = q.PutAdmission(context.Background(), hashScope("one"), []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	if _, _, err = q.PutAdmission(context.Background(), hashScope("two"), []byte(`{}`)); !errors.Is(err, ErrFull) {
		t.Fatal("reject_new quota not enforced")
	}
	metrics, _ := testLogScope().Hash()
	logs, _ := testLogScope().LogsHash()
	if metrics == logs {
		t.Fatal("metrics and Logs scopes collided")
	}
}

func TestLogDiskQuarantinesCorruptionWithoutDeletingOtherBacklog(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	q, err := OpenLogDisk(dir, testLogScope(), 1<<20, 8)
	if err != nil {
		t.Fatal(err)
	}
	bad, _, err := q.PutAdmission(context.Background(), hashScope("bad"), []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	goodID := hashScope("good")
	good, _, err := q.PutAdmission(context.Background(), goodID, []byte(`{"resourceLogs":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if err = q.Activate(context.Background(), goodID); err != nil {
		t.Fatal(err)
	}
	if err = q.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, string(bad)), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	q, err = OpenLogDisk(dir, testLogScope(), 1<<20, 8)
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close(context.Background())
	if q.Corrupt() != 1 {
		t.Fatal("corrupt record was not quarantined")
	}
	item, err := q.Next(context.Background())
	if err != nil || item.Receipt != good {
		t.Fatalf("valid backlog lost: %v %v", item.Receipt, err)
	}
}

// AGENTV1 FILE END
