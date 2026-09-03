//go:build linux

// AGENTV1 FILE START: scope changes, byte-preserving migration and real process-exit recovery.
package queue

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func testScope() Scope {
	return Scope{"backend-a", "org-a", "i-0123456789abcdef0", "127696279140", "us-east-2"}
}

const oldEndpoint = "https://old.example.test/api/v1/otlp"

func legacyFixture(t *testing.T, dir string) Receipt {
	t.Helper()
	s := testScope()
	q, e := OpenDisk(dir, oldEndpoint+"\x00"+s.HostID+"\x00"+s.Account+"\x00"+s.Region, 1<<20, 8)
	if e != nil {
		t.Fatal(e)
	}
	id, e := q.Put(context.Background(), []byte(`{"resourceMetrics":[]}`))
	if e != nil {
		t.Fatal(e)
	}
	q.Close(context.Background())
	return id
}
func TestStableScopeAndMigration(t *testing.T) {
	dir := privateTestDir(t)
	id := legacyFixture(t, dir)
	before, _ := os.ReadFile(filepath.Join(dir, string(id)))
	if q, e := OpenScopedDisk(dir, testScope(), "https://wrong.example.test/api/v1/otlp", 1<<20, 8); e == nil {
		q.Close(context.Background())
		t.Fatal("unverified previous endpoint accepted")
	}
	q, e := OpenScopedDisk(dir, testScope(), oldEndpoint, 1<<20, 8)
	if e != nil {
		t.Fatal(e)
	}
	q.Close(context.Background())
	after, _ := os.ReadFile(filepath.Join(dir, string(id)))
	if string(before) != string(after) {
		t.Fatal("record rewritten")
	}
	q, e = OpenScopedDisk(dir, testScope(), "https://new.example.test/api/v1/otlp", 1<<20, 8)
	if e != nil {
		t.Fatal(e)
	}
	item, e := q.Next(context.Background())
	if e != nil || item.Receipt != id {
		t.Fatal("replay lost", e)
	}
	q.Close(context.Background())
	changes := map[string]func(*Scope){"backend": func(s *Scope) { s.BackendID = "other" }, "org": func(s *Scope) { s.OrganizationID = "other" }, "host": func(s *Scope) { s.HostID = "other" }, "account": func(s *Scope) { s.Account = "other" }, "region": func(s *Scope) { s.Region = "other" }}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			s := testScope()
			change(&s)
			q, e := OpenScopedDisk(dir, s, oldEndpoint, 1<<20, 8)
			if e == nil {
				q.Close(context.Background())
				t.Fatal("scope change accepted")
			}
			if !strings.Contains(e.Error(), "queue_scope_v2_mismatch") {
				t.Fatal(e)
			}
		})
	}
	if q, e := OpenDisk(dir, "anything", 1<<20, 8); e == nil {
		q.Close(context.Background())
		t.Fatal("v1 downgrade accepted")
	}
}
func TestMigrationCrashRecovery(t *testing.T) {
	if dir := os.Getenv("SCOPE_CRASH_DIR"); dir != "" {
		_, e := openScopedDisk(dir, testScope(), oldEndpoint, 1<<20, 8, func(stage string) {
			if stage == os.Getenv("SCOPE_CRASH_STAGE") {
				os.Exit(29)
			}
		})
		if e != nil {
			os.Exit(2)
		}
		os.Exit(3)
	}
	for _, stage := range []string{"staged_migration.json", "journal", "staged_manifest.backup", "backup", "staged_manifest.json", "primary"} {
		t.Run(stage, func(t *testing.T) {
			dir := privateTestDir(t)
			id := legacyFixture(t, dir)
			before, _ := os.ReadFile(filepath.Join(dir, string(id)))
			c := exec.Command(os.Args[0], "-test.run=^TestMigrationCrashRecovery$")
			c.Env = append(os.Environ(), "SCOPE_CRASH_DIR="+dir, "SCOPE_CRASH_STAGE="+stage)
			if e := c.Run(); e == nil {
				t.Fatal("no crash")
			} else if x, ok := e.(*exec.ExitError); !ok || x.ExitCode() != 29 {
				t.Fatal(e)
			}
			previous := ""
			if stage == "staged_migration.json" {
				previous = oldEndpoint
			}
			// After journal commit, recovery needs only the original logical deployment identity.
			q, e := OpenScopedDisk(dir, testScope(), previous, 1<<20, 8)
			if e != nil {
				t.Fatal(e)
			}
			item, e := q.Next(context.Background())
			if e != nil || item.Receipt != id {
				t.Fatal("record lost", e)
			}
			q.Close(context.Background())
			after, _ := os.ReadFile(filepath.Join(dir, string(id)))
			if string(before) != string(after) {
				t.Fatal("payload changed")
			}
			q, e = OpenScopedDisk(dir, testScope(), "", 1<<20, 8)
			if e != nil {
				t.Fatal(e)
			}
			q.Close(context.Background())
		})
	}
}
func TestAmbiguousPendingIsRetained(t *testing.T) {
	dir := privateTestDir(t)
	id := legacyFixture(t, dir)
	p := filepath.Join(dir, ".pending")
	os.WriteFile(p, []byte("retained ambiguous bytes"), 0600)
	before, _ := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if q, e := OpenScopedDisk(dir, testScope(), oldEndpoint, 1<<20, 8); e == nil {
		q.Close(context.Background())
		t.Fatal("ambiguous pending accepted")
	}
	b, _ := os.ReadFile(p)
	after, _ := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if string(b) != "retained ambiguous bytes" || string(before) != string(after) {
		t.Fatal("state modified")
	}
	if _, e := os.Stat(filepath.Join(dir, string(id))); e != nil {
		t.Fatal(e)
	}
}

func TestLegacyIdentityMismatchRetainsBacklog(t *testing.T) {
	for _, field := range []string{"host", "account", "region"} {
		t.Run(field, func(t *testing.T) {
			dir := privateTestDir(t)
			id := legacyFixture(t, dir)
			before, _ := os.ReadFile(filepath.Join(dir, "manifest.json"))
			s := testScope()
			switch field {
			case "host":
				s.HostID = "different"
			case "account":
				s.Account = "different"
			case "region":
				s.Region = "different"
			}
			if q, e := OpenScopedDisk(dir, s, oldEndpoint, 1<<20, 8); e == nil {
				q.Close(context.Background())
				t.Fatal("changed legacy identity accepted")
			}
			after, _ := os.ReadFile(filepath.Join(dir, "manifest.json"))
			if string(before) != string(after) {
				t.Fatal("manifest modified on mismatch")
			}
			if _, e := os.Stat(filepath.Join(dir, string(id))); e != nil {
				t.Fatal(e)
			}
		})
	}
}

func TestPartialMigrationStageAndJournalScopeFailClosed(t *testing.T) {
	dir := privateTestDir(t)
	id := legacyFixture(t, dir)
	stage := filepath.Join(dir, "migration.json.next")
	if e := os.WriteFile(stage, []byte(`{"Version":`), 0600); e != nil {
		t.Fatal(e)
	}
	if q, e := OpenScopedDisk(dir, testScope(), oldEndpoint, 1<<20, 8); e == nil {
		q.Close(context.Background())
		t.Fatal("partial stage accepted")
	}
	if b, _ := os.ReadFile(stage); string(b) != `{"Version":` {
		t.Fatal("partial stage discarded")
	}
	if _, e := os.Stat(filepath.Join(dir, string(id))); e != nil {
		t.Fatal(e)
	}
	// A committed transition cannot be resumed into another organization.
	dir = privateTestDir(t)
	id = legacyFixture(t, dir)
	c := exec.Command(os.Args[0], "-test.run=^TestMigrationCrashRecovery$")
	c.Env = append(os.Environ(), "SCOPE_CRASH_DIR="+dir, "SCOPE_CRASH_STAGE=journal")
	if e := c.Run(); e == nil {
		t.Fatal("expected abrupt exit")
	}
	s := testScope()
	s.OrganizationID = "other-org"
	if q, e := OpenScopedDisk(dir, s, oldEndpoint, 1<<20, 8); e == nil {
		q.Close(context.Background())
		t.Fatal("journal scope change accepted")
	}
	q, e := OpenScopedDisk(dir, testScope(), "", 1<<20, 8)
	if e != nil {
		t.Fatal(e)
	}
	defer q.Close(context.Background())
	item, e := q.Next(context.Background())
	if e != nil || item.Receipt != id {
		t.Fatal("retained journal/backlog unavailable", e)
	}
}

// AGENTV1 FILE END
