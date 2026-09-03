//go:build linux

// AGENTV1 FILE START: YAML/secret files must be private even through disguised extensions.
package config

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestYAMLPrivateFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	if e := os.WriteFile(path, []byte(yamlValid), 0644); e != nil {
		t.Fatal(e)
	}
	if _, e := Load(path); e == nil {
		t.Fatal("public inline key accepted")
	}
	os.Chmod(path, 0600)
	c, e := Load(path)
	if e != nil {
		t.Fatal(e)
	}
	if e = c.CheckCredentials(context.Background()); e != nil {
		t.Fatal(e)
	}
	os.Rename(path, filepath.Join(dir, "agent.json"))
	os.Chmod(filepath.Join(dir, "agent.json"), 0644)
	if _, e = Load(filepath.Join(dir, "agent.json")); e == nil {
		t.Fatal("YAML disguise bypass")
	}
	key := filepath.Join(dir, "key")
	os.WriteFile(key, []byte("file-only-fixture\n"), 0600)
	c, e = Parse(strings.NewReader(strings.Replace(yamlValid, "  api_key:", "  api_key_file: "+key+"\n  api_key:", 1)))
	if e != nil {
		t.Fatal(e)
	}
	b, e := c.SecretProvider().Authorization(context.Background(), "")
	if e != nil || string(b) != "ApiKey file-only-fixture" {
		t.Fatal("file reference")
	}
	os.Chmod(key, 0644)
	if c.CheckCredentials(context.Background()) == nil {
		t.Fatal("public file reference")
	}
}

// AGENTV1 FILE END
