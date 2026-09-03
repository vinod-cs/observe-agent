//go:build linux

// AGENTV1 FILE START: versioned, exclusive, fsync-backed metrics spool; no secrets.
package queue

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
)

// Disk counts 4KiB record units plus control and per-entry directory reserves.
// Corrupt records consume the same budget after quarantine; reject_new is the
// only overflow policy. Accepted records are never evicted to make room.
type Disk struct {
	mu       sync.Mutex
	dir      string
	lock     *os.File
	manifest diskManifest
	names    []string
	bytes    int64
	count    int
	maxBytes int64
	maxItems int
	closed   bool
	wake     chan struct{}
	corrupt  uint64
}
type diskManifest struct {
	Version int
	Scope   string
	Next    uint64
}
type diskRecord struct {
	Version int
	Hash    string
	Payload json.RawMessage
}

func charge(n int64) int64 { return ((n + 4095) / 4096) * 4096 }

func OpenDisk(dir, scope string, maxBytes int64, maxItems int) (*Disk, error) {
	// AGENTV1 START: retained v1 primitive; production switches to OpenScopedDisk.
	return openDisk(dir, scope, maxBytes, maxItems, nil)
}
func OpenScopedDisk(dir string, scope Scope, previousEndpoint string, maxBytes int64, maxItems int) (*Disk, error) {
	return openScopedDisk(dir, scope, previousEndpoint, maxBytes, maxItems, nil)
}
func openScopedDisk(dir string, scope Scope, previousEndpoint string, maxBytes int64, maxItems int, hook func(string)) (*Disk, error) {
	h, e := scope.Hash()
	if e != nil {
		return nil, e
	}
	return openDisk(dir, h, maxBytes, maxItems, &scopeUpgrade{hash: h, legacy: scope.legacyHash(previousEndpoint), hook: hook})
}
func openDisk(dir, scope string, maxBytes int64, maxItems int, upgrade *scopeUpgrade) (*Disk, error) {
	// AGENTV1 END: v2 entry point
	if !filepath.IsAbs(dir) || scope == "" || maxBytes < 32768 || maxItems < 1 || maxItems > 1024 {
		return nil, errors.New("invalid spool options")
	}
	// Refuse symlinked path components, including an existing state directory.
	for p := filepath.Clean(dir); ; p = filepath.Dir(p) {
		info, e := os.Lstat(p)
		if e == nil && info.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("spool path contains symlink")
		}
		if e != nil && !os.IsNotExist(e) {
			return nil, e
		}
		if filepath.Dir(p) == p {
			break
		}
	}
	if e := os.MkdirAll(dir, 0700); e != nil {
		return nil, e
	}
	info, e := os.Lstat(dir)
	if e != nil {
		return nil, e
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.IsDir() || info.Mode().Perm() != 0700 || st.Uid != uint32(os.Geteuid()) {
		return nil, errors.New("spool directory must be owned by current user with mode 0700")
	}
	var fs syscall.Statfs_t
	if e = syscall.Statfs(dir, &fs); e != nil {
		return nil, e
	}
	if fs.Bsize > 4096 {
		return nil, errors.New("spool filesystem allocation units above 4KiB unsupported")
	}
	fd, e := syscall.Open(filepath.Join(dir, "lock"), syscall.O_CREAT|syscall.O_RDWR|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0600)
	if e != nil {
		return nil, e
	}
	lock := os.NewFile(uintptr(fd), "spool lock")
	if e = syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		lock.Close()
		return nil, errors.New("spool already in use")
	}
	q := &Disk{dir: dir, lock: lock, maxBytes: maxBytes, maxItems: maxItems, bytes: 16384 + charge(int64(maxItems)*256), wake: make(chan struct{}, 1)}
	// AGENTV1 START: reserve additional bounded space for migration control stages.
	if upgrade != nil {
		q.bytes += 8192
	}
	// AGENTV1 END: migration control reserve
	fail := func(err error) (*Disk, error) { lock.Close(); return nil, err }
	directory, e := os.Open(dir)
	if e != nil {
		return fail(e)
	}
	entries, e := directory.ReadDir(4097)
	directory.Close()
	if e != nil && e != io.EOF {
		return fail(e)
	}
	if len(entries) > 4096 {
		return fail(errors.New("too many spool entries"))
	}
	// AGENTV1 START: the previous inline v1-only check is now a transactional loader.
	if e = q.loadManifest(entries, hashScope(scope), upgrade); e != nil {
		return fail(e)
	}
	// Reload directory entries because migration atomically renames control files.
	directory, e = os.Open(dir)
	if e != nil {
		return fail(e)
	}
	entries, e = directory.ReadDir(4097)
	directory.Close()
	if e != nil && e != io.EOF {
		return fail(e)
	}
	if len(entries) > 4096 {
		return fail(errors.New("too many spool entries"))
	}
	// AGENTV1 END: scope control loading
	for _, entry := range entries {
		name := entry.Name()
		if name == "lock" || name == "manifest.json" || name == "manifest.backup" {
			continue
		}
		if name == ".pending" {
			// AGENTV1 START: never discard ambiguous pending telemetry on the v2 path.
			if upgrade != nil {
				return fail(errors.New("queue_scope_pending_ambiguous: pending file retained; recovery required"))
			}
			// AGENTV1 END: preserve ambiguous pending file
			if _, e = q.read(name, 5<<20); e != nil {
				return fail(e)
			}
			if e = os.Remove(filepath.Join(dir, name)); e != nil {
				return fail(e)
			}
			continue
		}
		var seq uint64
		if len(name) != 24 || (name[20:] != ".rec" && name[20:] != ".bad") {
			return fail(errors.New("unexpected spool entry; retained"))
		}
		if _, e = fmt.Sscanf(name[:20], "%d", &seq); e != nil || seq == 0 || seq >= q.manifest.Next || fmt.Sprintf("%020d", seq) != name[:20] {
			return fail(errors.New("invalid spool sequence"))
		}
		data, e := q.read(name, 5<<20)
		if e != nil {
			return fail(e)
		}
		q.count++
		q.bytes += charge(int64(len(data)))
		if strings.HasSuffix(name, ".rec") {
			if _, e = decodeRecord(data); e != nil {
				if e = q.quarantine(name); e != nil {
					return fail(e)
				}
			} else {
				q.names = append(q.names, name)
			}
		} else {
			q.corrupt++
		}
	}
	if q.bytes > maxBytes || q.count > maxItems {
		return fail(errors.New("existing spool exceeds configured budget; increase budget to drain"))
	}
	sort.Strings(q.names)
	if e = q.syncDir(); e != nil {
		return fail(e)
	}
	return q, nil
}
func (q *Disk) read(name string, limit int64) ([]byte, error) {
	info, e := os.Lstat(filepath.Join(q.dir, name))
	if e != nil {
		return nil, e
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || st.Uid != uint32(os.Geteuid()) || st.Nlink != 1 || info.Size() > limit {
		return nil, errors.New("unsafe or oversized spool file")
	}
	fd, e := syscall.Open(filepath.Join(q.dir, name), syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if e != nil {
		return nil, e
	}
	f := os.NewFile(uintptr(fd), "spool record")
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, limit+1))
	if int64(len(b)) > limit {
		return nil, errors.New("spool file grew beyond bound")
	}
	return b, e
}
func (q *Disk) syncDir() error {
	f, e := os.Open(q.dir)
	if e != nil {
		return e
	}
	defer f.Close()
	return f.Sync()
}
func (q *Disk) write(name string, data []byte) error {
	path := filepath.Join(q.dir, ".pending")
	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	defer os.Remove(path)
	if _, e = f.Write(data); e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	if e = os.Rename(path, filepath.Join(q.dir, name)); e != nil {
		return e
	}
	return q.syncDir()
}
func (q *Disk) saveManifest() error {
	b, _ := json.Marshal(q.manifest)
	if e := q.write("manifest.backup", b); e != nil {
		return e
	}
	return q.write("manifest.json", b)
}
func decodeRecord(b []byte) ([]byte, error) {
	var r diskRecord
	if json.Unmarshal(b, &r) != nil || r.Version != 1 || !json.Valid(r.Payload) {
		return nil, errors.New("corrupt record")
	}
	hash := sha256.Sum256(r.Payload)
	if hex.EncodeToString(hash[:]) != r.Hash {
		return nil, errors.New("record checksum mismatch")
	}
	return r.Payload, nil
}
func (q *Disk) quarantine(name string) error {
	if e := os.Rename(filepath.Join(q.dir, name), filepath.Join(q.dir, strings.TrimSuffix(name, ".rec")+".bad")); e != nil {
		return e
	}
	q.corrupt++
	return q.syncDir()
}
func (q *Disk) Put(ctx context.Context, data []byte) (Receipt, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if q.closed {
		return "", errors.New("spool closed")
	}
	if len(data) > 4<<20 || !json.Valid(data) {
		return "", errors.New("invalid spool payload")
	}
	// Marshal compact JSON before hashing so RawMessage serialization is stable.
	var compact json.RawMessage = data
	normalized, e := json.Marshal(compact)
	if e != nil {
		return "", e
	}
	hash := sha256.Sum256(normalized)
	b, _ := json.Marshal(diskRecord{1, hex.EncodeToString(hash[:]), normalized})
	cost := charge(int64(len(b)))
	if q.count >= q.maxItems || q.bytes+cost > q.maxBytes {
		return "", ErrFull
	}
	if q.manifest.Next == ^uint64(0) {
		return "", errors.New("spool sequence exhausted")
	}
	name := fmt.Sprintf("%020d.rec", q.manifest.Next)
	q.manifest.Next++
	if e = q.saveManifest(); e != nil {
		q.closed = true
		return "", e
	}
	if e = q.write(name, b); e != nil {
		q.closed = true
		return "", e
	}
	q.names = append(q.names, name)
	q.count++
	q.bytes += cost
	select {
	case q.wake <- struct{}{}:
	default:
	}
	return Receipt(name), nil
}
func (q *Disk) Next(ctx context.Context) (Item, error) {
	for {
		if ctx.Err() != nil {
			return Item{}, ctx.Err()
		}
		q.mu.Lock()
		if q.closed {
			q.mu.Unlock()
			return Item{}, errors.New("spool closed")
		}
		if len(q.names) > 0 {
			name := q.names[0]
			b, e := q.read(name, 5<<20)
			if e != nil {
				q.mu.Unlock()
				return Item{}, e
			}
			data, e := decodeRecord(b)
			if e != nil {
				e = q.quarantine(name)
				if e == nil {
					q.names = q.names[1:]
				}
				q.mu.Unlock()
				if e != nil {
					return Item{}, e
				}
				continue
			}
			q.mu.Unlock()
			return Item{Receipt(name), data}, nil
		}
		q.mu.Unlock()
		select {
		case <-ctx.Done():
			return Item{}, ctx.Err()
		case <-q.wake:
		}
	}
}
func (q *Disk) Ack(ctx context.Context, id Receipt) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if q.closed {
		return errors.New("spool closed")
	}
	if len(q.names) == 0 || q.names[0] != string(id) {
		return errors.New("receipt is not current FIFO head")
	}
	info, e := os.Stat(filepath.Join(q.dir, string(id)))
	if e != nil {
		return e
	}
	if e = os.Remove(filepath.Join(q.dir, string(id))); e != nil {
		return e
	}
	if e = q.syncDir(); e != nil {
		q.closed = true
		return e
	}
	q.names = q.names[1:]
	q.count--
	q.bytes -= charge(info.Size())
	return nil
}
func (q *Disk) Close(context.Context) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.closed = true
	select {
	case q.wake <- struct{}{}:
	default:
	}
	if q.lock == nil {
		return nil
	}
	e := q.lock.Close()
	q.lock = nil
	return e
}
func (q *Disk) Corrupt() uint64 { q.mu.Lock(); defer q.mu.Unlock(); return q.corrupt }

// AGENTV1 FILE END
