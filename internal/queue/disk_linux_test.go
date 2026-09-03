//go:build linux

// AGENTV1 FILE START: isolated native crash/durability/corruption fault injection.
package queue

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func openTest(t *testing.T, dir string) *Disk {
	t.Helper()
	q, e := OpenDisk(dir, "endpoint/host", 1<<20, 8)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { q.Close(context.Background()) })
	return q
}
func privateTestDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "spool")
	if e := os.Mkdir(dir, 0700); e != nil {
		t.Fatal(e)
	}
	return dir
}
func TestCrashReplayFIFOAndAck(t *testing.T) {
	if dir := os.Getenv("AGENT_QUEUE_CRASH_FIXTURE"); dir != "" {
		q, e := OpenDisk(dir, "endpoint/host", 1<<20, 8)
		if e != nil {
			os.Exit(2)
		}
		for _, s := range []string{`{"n":1}`, `{"n":2}`} {
			if _, e = q.Put(context.Background(), []byte(s)); e != nil {
				os.Exit(3)
			}
		}
		// Deliberately no Close/defer: model abrupt process termination after fsync.
		os.Exit(23)
	}
	dir := privateTestDir(t)
	child := exec.Command(os.Args[0], "-test.run=^TestCrashReplayFIFOAndAck$")
	child.Env = append(os.Environ(), "AGENT_QUEUE_CRASH_FIXTURE="+dir)
	if e := child.Run(); e == nil {
		t.Fatal("crash helper did not exit")
	} else if ee, ok := e.(*exec.ExitError); !ok || ee.ExitCode() != 23 {
		t.Fatalf("helper: %v", e)
	}
	q := openTest(t, dir)
	ctx := context.Background()
	first, e := q.Next(ctx)
	if e != nil || string(first.Data) != `{"n":1}` {
		t.Fatalf("first: %s %v", first.Data, e)
	}
	again, _ := q.Next(ctx)
	if again.Receipt != first.Receipt {
		t.Fatal("peek dequeued")
	}
	if e = q.Ack(ctx, first.Receipt); e != nil {
		t.Fatal(e)
	}
	if e = q.Ack(ctx, first.Receipt); e == nil {
		t.Fatal("duplicate ack accepted")
	}
	q.Close(ctx)
	q = openTest(t, dir)
	second, e := q.Next(ctx)
	if e != nil || string(second.Data) != `{"n":2}` {
		t.Fatal("FIFO restart failed", e)
	}
	if e = q.Ack(ctx, second.Receipt); e != nil {
		t.Fatal(e)
	}
	timeout, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer cancel()
	if _, e = q.Next(timeout); e == nil {
		t.Fatal("dequeued records reappeared")
	}
}
func TestFullCorruptAndExclusiveSpool(t *testing.T) {
	dir := privateTestDir(t)
	q, e := OpenDisk(dir, "scope", 32768, 2)
	if e != nil {
		t.Fatal(e)
	}
	defer q.Close(context.Background())
	if other, e := OpenDisk(dir, "scope", 32768, 2); e == nil {
		other.Close(context.Background())
		t.Fatal("two writers")
	}
	a, e := q.Put(context.Background(), []byte(`{"n":1}`))
	if e != nil {
		t.Fatal(e)
	}
	_, e = q.Put(context.Background(), []byte(`{"n":2}`))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = q.Put(context.Background(), []byte(`{"n":3}`)); e == nil {
		t.Fatal("overflow not rejected")
	}
	q.Close(context.Background())
	if e = os.WriteFile(filepath.Join(dir, string(a)), []byte("broken"), 0600); e != nil {
		t.Fatal(e)
	}
	q, e = OpenDisk(dir, "scope", 32768, 2)
	if e != nil {
		t.Fatal(e)
	}
	defer q.Close(context.Background())
	b, e := q.Next(context.Background())
	if e != nil || string(b.Data) != `{"n":2}` || q.Corrupt() != 1 {
		t.Fatal("corruption did not isolate record", e)
	}
	if _, e = os.Stat(filepath.Join(dir, string(a[:20])+".bad")); e != nil {
		t.Fatal("corrupt evidence lost")
	}
	if _, e = q.Put(context.Background(), []byte(`{}`)); e == nil {
		t.Fatal("quarantine bypasses bounds")
	}
}
func TestManifestRecoveryAndScopeProtection(t *testing.T) {
	dir := privateTestDir(t)
	q := openTest(t, dir)
	q.Put(context.Background(), []byte(`{}`))
	q.Close(context.Background())
	if e := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte("torn"), 0600); e != nil {
		t.Fatal(e)
	}
	q = openTest(t, dir)
	if _, e := q.Next(context.Background()); e != nil {
		t.Fatal(e)
	}
	q.Close(context.Background())
	if other, e := OpenDisk(dir, "other-host", 1<<20, 8); e == nil {
		other.Close(context.Background())
		t.Fatal("scope mismatch accepted")
	}
}
func TestDiskByteBudgetAndPermissions(t *testing.T) {
	dir := privateTestDir(t)
	q, e := OpenDisk(dir, "scope", 32768, 8)
	if e != nil {
		t.Fatal(e)
	}
	defer q.Close(context.Background())
	data := make([]byte, 20000)
	for i := range data {
		data[i] = ' '
	}
	data[0] = '['
	data[len(data)-1] = ']'
	// Non-whitespace JSON value forces actual storage rather than JSON compaction.
	data[0] = '"'
	data[len(data)-1] = '"'
	for i := 1; i < len(data)-1; i++ {
		data[i] = 'a'
	}
	if _, e = q.Put(context.Background(), data); e == nil {
		t.Fatal("byte budget ignored")
	}
	q.Close(context.Background())
	os.Chmod(dir, 0755)
	if other, e := OpenDisk(dir, "scope", 32768, 8); e == nil {
		other.Close(context.Background())
		t.Fatal("public spool accepted")
	}
}

// AGENTV1 FILE END
