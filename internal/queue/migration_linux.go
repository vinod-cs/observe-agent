//go:build linux

// AGENTV1 FILE START: migration stages contain control metadata only; payload files are untouched.
package queue

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type scopeUpgrade struct {
	hash, legacy string
	hook         func(string)
}
type migration struct {
	Version            int
	OldScope, NewScope string
	Next               uint64
}

func manifestValid(b []byte) bool {
	var m diskManifest
	return json.Unmarshal(b, &m) == nil && m.Next > 0 && (m.Version == 1 || m.Version == 2) && scopeHashValid(m.Scope)
}
func scopeHashValid(s string) bool { b, e := hex.DecodeString(s); return e == nil && len(b) == 32 }

// Only exact, previously staged metadata may be resumed. Never truncate/delete a staging file.
func (q *Disk) scopeWrite(name string, b []byte, u *scopeUpgrade) error {
	stage := name + ".next"
	existing, e := q.read(stage, 4096)
	if e == nil {
		if !bytes.Equal(existing, b) {
			return errors.New("queue_scope_staging_conflict: metadata stage retained for recovery")
		}
	} else if os.IsNotExist(e) {
		f, e := os.OpenFile(filepath.Join(q.dir, stage), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			return e
		}
		_, e = f.Write(b)
		if e == nil {
			e = f.Sync()
		}
		ce := f.Close()
		if e != nil {
			return e
		}
		if ce != nil {
			return ce
		}
	} else {
		return e
	}
	if u.hook != nil {
		u.hook("staged_" + name)
	}
	if e = os.Rename(filepath.Join(q.dir, stage), filepath.Join(q.dir, name)); e != nil {
		return e
	}
	return q.syncDir()
}
func (q *Disk) loadManifest(entries []os.DirEntry, legacy string, u *scopeUpgrade) error {
	if u != nil {
		if _, e := os.Lstat(filepath.Join(q.dir, ".pending")); e == nil {
			return errors.New("queue_scope_pending_ambiguous: pending telemetry retained; recovery required")
		} else if !os.IsNotExist(e) {
			return e
		}
	}
	journal, je := q.read("migration.json", 4096)
	if je == nil {
		if u == nil {
			return errors.New("queue_scope_migration_requires_v2: retain state and use upgraded Agent")
		}
		var tx migration
		if json.Unmarshal(journal, &tx) != nil || tx.Version != 2 || tx.Next == 0 || !scopeHashValid(tx.OldScope) || tx.NewScope != u.hash {
			return errors.New("queue_scope_migration_mismatch: backend/organization/host scope differs or journal invalid; records retained")
		}
		valid := 0
		for _, name := range []string{"manifest.json", "manifest.backup"} {
			b, e := q.read(name, 4096)
			if e != nil {
				return errors.New("queue_scope_migration_control_unavailable: retain state for recovery")
			}
			if !manifestValid(b) {
				continue
			}
			var m diskManifest
			_ = json.Unmarshal(b, &m)
			if m.Next != tx.Next || !((m.Version == 1 && m.Scope == tx.OldScope) || (m.Version == 2 && m.Scope == tx.NewScope)) {
				return errors.New("queue_scope_migration_conflict: inconsistent control state; records retained")
			}
			valid++
		}
		if valid == 0 {
			return errors.New("queue_scope_migration_ambiguous: no verified control copy; records retained")
		}
		return q.finishMigration(tx, u)
	}
	if !os.IsNotExist(je) {
		return errors.New("queue_scope_migration_journal_unsafe: records retained")
	}
	raw, e := q.read("manifest.json", 4096)
	if e != nil || json.Unmarshal(raw, &diskManifest{}) != nil {
		raw, e = q.read("manifest.backup", 4096)
	}
	if os.IsNotExist(e) {
		for _, entry := range entries {
			if entry.Name() != "lock" {
				return errors.New("missing spool manifest; retain files for recovery")
			}
		}
		q.manifest = diskManifest{1, legacy, 1}
		if u != nil {
			q.manifest.Version = 2
			q.manifest.Scope = u.hash
		}
		return q.saveManifest()
	}
	if e != nil {
		return e
	}
	if !manifestValid(raw) {
		return errors.New("queue_scope_manifest_invalid: unsupported/corrupt control state; records retained")
	}
	_ = json.Unmarshal(raw, &q.manifest)
	if u == nil {
		if q.manifest.Version != 1 || q.manifest.Scope != legacy {
			return errors.New("queue_scope_mismatch: legacy scope differs or v2 requires upgraded Agent; records retained")
		}
		return nil
	}
	if q.manifest.Version == 2 {
		if q.manifest.Scope != u.hash {
			return errors.New("queue_scope_v2_mismatch: backend_id, organization_id or canonical host/account/region changed; records retained")
		}
		return nil
	}
	if q.manifest.Scope != u.legacy {
		return errors.New("queue_scope_v1_unverified: verify previous_endpoint and original host/account/region; records retained")
	}
	backup, e := q.read("manifest.backup", 4096)
	var other diskManifest
	if e != nil || json.Unmarshal(backup, &other) != nil || other != q.manifest {
		return errors.New("queue_scope_v1_ambiguous: control copies differ; records retained for recovery")
	}
	tx := migration{2, q.manifest.Scope, u.hash, q.manifest.Next}
	b, _ := json.Marshal(tx)
	if e = q.scopeWrite("migration.json", b, u); e != nil {
		return e
	}
	if u.hook != nil {
		u.hook("journal")
	}
	return q.finishMigration(tx, u)
}
func (q *Disk) finishMigration(tx migration, u *scopeUpgrade) error {
	q.manifest = diskManifest{2, tx.NewScope, tx.Next}
	b, _ := json.Marshal(q.manifest)
	if e := q.scopeWrite("manifest.backup", b, u); e != nil {
		return e
	}
	if u.hook != nil {
		u.hook("backup")
	}
	if e := q.scopeWrite("manifest.json", b, u); e != nil {
		return e
	}
	if u.hook != nil {
		u.hook("primary")
	}
	// Delete only the now-committed control journal, never records or ambiguous pending files.
	if e := os.Remove(filepath.Join(q.dir, "migration.json")); e != nil {
		return e
	}
	return q.syncDir()
}

// AGENTV1 FILE END
