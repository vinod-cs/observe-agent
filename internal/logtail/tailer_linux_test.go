//go:build linux

// AGENTV1 FILE START: secure tailing, durable checkpoint, rotation and multiline tests.
package logtail

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/agent-i/agent/internal/config"
)

func source(root string) config.LogSource {
	return config.LogSource{ID: "application", Root: root, Include: []string{"*.log"}, Exclude: []string{"*.gz"}, StartAt: "beginning", MaxOpenFiles: 8, MaxLineBytes: 4096, Multiline: config.LogMultiline{FlushTimeoutMillis: 100, MaxLines: 20, MaxBytes: 4096}}
}
func setup(t *testing.T, src config.LogSource, admit Admit) *Tailer {
	t.Helper()
	tailer := New(src, filepath.Join(t.TempDir(), "checkpoints"), 32, time.Second, admit, nil)
	if src.Multiline.Enabled {
		tailer.start = regexp.MustCompile(src.Multiline.StartPattern)
	}
	root, err := openRoot(src.Root)
	if err != nil {
		t.Fatal(err)
	}
	tailer.root = root
	if err = ensurePrivateDir(tailer.checkpointDir); err != nil {
		t.Fatal(err)
	}
	if err = tailer.loadCheckpoint(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, s := range tailer.states {
			s.file.Close()
		}
		root.Close()
	})
	return tailer
}

func TestDurableAdmissionBeforeCheckpointAndStableReplayID(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "app.log"), []byte("first\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var ids []string
	fail := true
	tailer := setup(t, source(root), func(_ context.Context, r Record) error {
		ids = append(ids, r.AdmissionID)
		if fail {
			return errors.New("queue full")
		}
		return nil
	})
	tailer.pollOnce(context.Background())
	if len(ids) != 1 {
		t.Fatal("record not attempted")
	}
	for _, cp := range tailer.checkpoints.Files {
		if cp.Offset != 0 {
			t.Fatal("checkpoint advanced before durable admission")
		}
	}
	fail = false
	tailer.pollOnce(context.Background())
	if len(ids) != 2 || ids[0] != ids[1] {
		t.Fatal("crash-window admission identity changed")
	}
	for _, cp := range tailer.checkpoints.Files {
		if cp.Offset != 6 {
			t.Fatalf("checkpoint=%d", cp.Offset)
		}
	}
}

func TestRootAndFileSymlinkEscapesRejected(t *testing.T) {
	outside := t.TempDir()
	os.WriteFile(filepath.Join(outside, "secret.log"), []byte("secret\n"), 0600)
	parent := t.TempDir()
	rootLink := filepath.Join(parent, "root")
	if err := os.Symlink(outside, rootLink); err != nil {
		t.Skip(err)
	}
	if _, err := openRoot(rootLink); err == nil {
		t.Fatal("symlink root accepted")
	}
	root := t.TempDir()
	if err := os.Symlink(filepath.Join(outside, "secret.log"), filepath.Join(root, "escape.log")); err != nil {
		t.Skip(err)
	}
	count := 0
	tailer := setup(t, source(root), func(context.Context, Record) error { count++; return nil })
	tailer.pollOnce(context.Background())
	if count != 0 {
		t.Fatal("symlink target read")
	}
}

func TestReplacementRaceCannotRedirectOpenDescriptor(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	path := filepath.Join(root, "app.log")
	original := filepath.Join(root, "app.log.rotated")
	secret := filepath.Join(outside, "secret.log")
	os.WriteFile(path, []byte("safe-one\n"), 0600)
	os.WriteFile(secret, []byte("must-not-read\n"), 0600)
	var bodies []string
	tailer := setup(t, source(root), func(_ context.Context, r Record) error { bodies = append(bodies, r.Body); return nil })
	tailer.pollOnce(context.Background())
	if err := os.Rename(path, original); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(original, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("safe-two\n")
	_ = f.Close()
	if err = os.Symlink(secret, path); err != nil {
		t.Skip(err)
	}
	tailer.pollOnce(context.Background())
	if strings.Join(bodies, "|") != "safe-one|safe-two" {
		t.Fatalf("replacement race escaped root or lost retained descriptor: %v", bodies)
	}
}

func TestRenameReplacementAndCopytruncate(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "app.log")
	os.WriteFile(path, []byte("old-long-record\n"), 0600)
	var bodies []string
	tailer := setup(t, source(root), func(_ context.Context, r Record) error { bodies = append(bodies, r.Body); return nil })
	tailer.pollOnce(context.Background())
	os.Rename(path, filepath.Join(root, "app.log.1"))
	f, _ := os.OpenFile(filepath.Join(root, "app.log.1"), os.O_APPEND|os.O_WRONLY, 0600)
	f.WriteString("rotated\n")
	f.Close()
	os.WriteFile(path, []byte("replacement\n"), 0600)
	tailer.pollOnce(context.Background())
	joined := "|" + strings.Join(bodies, "|") + "|"
	if len(bodies) < 3 || !strings.Contains(joined, "|rotated|") || !strings.Contains(joined, "|replacement|") {
		t.Fatalf("rotation bodies=%v", bodies)
	}
	os.WriteFile(path, []byte("new\n"), 0600)
	tailer.pollOnce(context.Background())
	if bodies[len(bodies)-1] != "new" {
		t.Fatalf("copytruncate bodies=%v", bodies)
	}
}

func TestMultilineBoundedFlush(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "app.log"), []byte("START one\n continuation\nSTART two\n"), 0600)
	src := source(root)
	src.Multiline.Enabled = true
	src.Multiline.StartPattern = "^START"
	var bodies []string
	tailer := setup(t, src, func(_ context.Context, r Record) error { bodies = append(bodies, r.Body); return nil })
	tailer.pollOnce(context.Background())
	if len(bodies) != 1 || bodies[0] != "START one\n continuation" {
		t.Fatalf("multiline=%v", bodies)
	}
	tailer.now = func() time.Time { return time.Now().Add(time.Second) }
	tailer.pollOnce(context.Background())
	if len(bodies) != 2 || bodies[1] != "START two" {
		t.Fatalf("timeout flush=%v", bodies)
	}
}

func TestCRLFInvalidUTF8EmptyAndOversize(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "app.log"), []byte{'o', 'k', '\r', '\n', 0xff, '\n', '\n', '1', '2', '3', '4', '5', '\n'}, 0600)
	src := source(root)
	src.MaxLineBytes = 4
	var bodies []string
	tailer := setup(t, src, func(_ context.Context, r Record) error { bodies = append(bodies, r.Body); return nil })
	tailer.pollOnce(context.Background())
	if len(bodies) != 2 || bodies[0] != "ok" || bodies[1] != "�" {
		t.Fatalf("normalized=%q", bodies)
	}
	if tailer.stats.RecordsRejectedLocal != 2 {
		t.Fatalf("rejections=%d", tailer.stats.RecordsRejectedLocal)
	}
}

// AGENTV1 FILE END
