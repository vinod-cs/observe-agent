//go:build linux

// AGENTV1 FILE START: Linux file/state tests use only temporary test storage.
package platform

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPrivateStateAndFileIdentity(t *testing.T) {
	dir := t.TempDir()
	if e := os.Chmod(dir, 0700); e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(dir, "policy.json")
	n := Native{}
	if e := n.Replace(context.Background(), path, []byte(`{"version":1}`)); e != nil {
		t.Fatal(e)
	}
	b, e := n.Read(context.Background(), path)
	if e != nil || string(b) != `{"version":1}` {
		t.Fatal("state roundtrip")
	}
	f, e := os.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	id, e := n.ID(f)
	if e != nil || id == "" {
		t.Fatal("file id")
	}
	if e = os.Chmod(dir, 0755); e != nil {
		t.Fatal(e)
	}
	if n.Replace(context.Background(), path, []byte("bad")) == nil {
		t.Fatal("public state dir accepted")
	}
}

// AGENTV1 FILE END
