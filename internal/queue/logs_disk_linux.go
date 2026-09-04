//go:build linux

// AGENTV1 FILE START: independent durable Logs spool with idempotent admission IDs.
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

type logManifest struct {
	Version int    `json:"version"`
	Scope   string `json:"scope"`
	Next    uint64 `json:"next"`
}
type logRecord struct {
	Version     int             `json:"version"`
	AdmissionID string          `json:"admission_id"`
	Ready       bool            `json:"ready"`
	Hash        string          `json:"hash"`
	Payload     json.RawMessage `json:"payload"`
}

type LogDisk struct {
	mu              sync.Mutex
	dir             string
	lock            *os.File
	manifest        logManifest
	names           []string
	admissions      map[string]string
	bytes           int64
	count, maxItems int
	maxBytes        int64
	closed          bool
	wake            chan struct{}
	corrupt         uint64
}

func OpenLogDisk(dir string, scope Scope, maxBytes int64, maxItems int) (*LogDisk, error) {
	h, err := scope.LogsHash()
	if err != nil {
		return nil, err
	}
	if !filepath.IsAbs(dir) || maxBytes < 32768 || maxItems < 1 || maxItems > 4096 {
		return nil, errors.New("invalid logs spool options")
	}
	for p := filepath.Clean(dir); ; p = filepath.Dir(p) {
		info, e := os.Lstat(p)
		if e == nil && info.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("logs spool path contains symlink")
		}
		if e != nil && !os.IsNotExist(e) {
			return nil, e
		}
		if filepath.Dir(p) == p {
			break
		}
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return nil, err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.IsDir() || info.Mode().Perm() != 0700 || st.Uid != uint32(os.Geteuid()) {
		return nil, errors.New("logs spool directory must be owned by current user with mode 0700")
	}
	fd, err := syscall.Open(filepath.Join(dir, "lock"), syscall.O_CREAT|syscall.O_RDWR|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0600)
	if err != nil {
		return nil, err
	}
	lock := os.NewFile(uintptr(fd), "logs spool lock")
	if err = syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		return nil, errors.New("logs spool already in use")
	}
	q := &LogDisk{dir: dir, lock: lock, maxBytes: maxBytes, maxItems: maxItems, admissions: map[string]string{}, bytes: 16384 + charge(int64(maxItems)*256), wake: make(chan struct{}, 1)}
	fail := func(e error) (*LogDisk, error) { lock.Close(); return nil, e }
	manifestPath := filepath.Join(dir, "manifest.json")
	if err = recoverLogManifest(dir, h); err != nil {
		return fail(err)
	}
	raw, err := readPrivateFile(manifestPath, 1<<20)
	if os.IsNotExist(err) {
		q.manifest = logManifest{Version: 1, Scope: h, Next: 1}
		if err = q.saveManifest(); err != nil {
			return fail(err)
		}
	} else if err != nil {
		return fail(err)
	} else if json.Unmarshal(raw, &q.manifest) != nil || q.manifest.Version != 1 || q.manifest.Scope != h || q.manifest.Next == 0 {
		return fail(errors.New("logs spool manifest invalid or scope changed"))
	}
	if err = q.recoverPendingRecord(); err != nil {
		return fail(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fail(err)
	}
	if len(entries) > maxItems+16 {
		return fail(errors.New("too many logs spool entries"))
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == "lock" || name == "manifest.json" || name == ".replace.pending" || strings.HasSuffix(name, ".bad") {
			continue
		}
		if len(name) != 25 || !strings.HasSuffix(name, ".lrec") {
			return fail(errors.New("unexpected logs spool entry retained"))
		}
		b, e := q.read(name)
		if e != nil {
			return fail(e)
		}
		r, e := decodeLogRecord(b)
		if e != nil {
			if e = q.quarantine(name); e != nil {
				return fail(e)
			}
			continue
		}
		if prior := q.admissions[r.AdmissionID]; prior != "" {
			return fail(errors.New("duplicate admission identity in logs spool"))
		}
		if r.Ready {
			q.names = append(q.names, name)
		}
		q.admissions[r.AdmissionID] = name
		q.count++
		q.bytes += charge(int64(len(b)))
	}
	if err = q.recoverPendingActivation(); err != nil {
		return fail(err)
	}
	if q.bytes > maxBytes || q.count > maxItems {
		return fail(errors.New("existing logs spool exceeds configured budget"))
	}
	sort.Strings(q.names)
	return q, nil
}

func decodeLogManifest(raw []byte, scope string) (logManifest, error) {
	var manifest logManifest
	if json.Unmarshal(raw, &manifest) != nil || manifest.Version != 1 || manifest.Scope != scope || manifest.Next == 0 {
		return manifest, errors.New("logs spool pending manifest invalid or scope changed")
	}
	return manifest, nil
}

// recoverLogManifest completes an interrupted atomic manifest replacement. It
// never guesses scope and never removes a newer manifest.
func recoverLogManifest(dir, scope string) error {
	pendingPath := filepath.Join(dir, ".manifest.pending")
	pendingRaw, err := readPrivateFile(pendingPath, 1<<20)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	pending, err := decodeLogManifest(pendingRaw, scope)
	if err != nil {
		return err
	}
	manifestPath := filepath.Join(dir, "manifest.json")
	currentRaw, currentErr := readPrivateFile(manifestPath, 1<<20)
	if currentErr == nil {
		current, decodeErr := decodeLogManifest(currentRaw, scope)
		if decodeErr != nil {
			return decodeErr
		}
		if pending.Next < current.Next {
			return errors.New("logs spool pending manifest is older than current manifest")
		}
	} else if !os.IsNotExist(currentErr) {
		return currentErr
	}
	if err = os.Rename(pendingPath, manifestPath); err != nil {
		return err
	}
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

// recoverPendingRecord completes an interrupted queue admission after the
// manifest has advanced. The sequence is deterministic; a conflicting record
// is retained and fails closed.
func (q *LogDisk) recoverPendingRecord() error {
	pendingPath := filepath.Join(q.dir, ".pending")
	raw, err := q.read(".pending")
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	pending, err := decodeLogRecord(raw)
	if err != nil {
		return err
	}
	if q.manifest.Next <= 1 {
		return errors.New("logs spool pending record has no reserved sequence")
	}
	name := fmt.Sprintf("%020d.lrec", q.manifest.Next-1)
	targetRaw, targetErr := q.read(name)
	if targetErr == nil {
		target, decodeErr := decodeLogRecord(targetRaw)
		if decodeErr != nil || target.AdmissionID != pending.AdmissionID || target.Hash != pending.Hash || !bytesEqual(target.Payload, pending.Payload) {
			return errors.New("logs spool pending record conflicts with reserved sequence")
		}
		if err = os.Remove(pendingPath); err != nil {
			return err
		}
		return q.syncDir()
	}
	if !os.IsNotExist(targetErr) {
		return targetErr
	}
	if err = os.Rename(pendingPath, filepath.Join(q.dir, name)); err != nil {
		return err
	}
	return q.syncDir()
}

func bytesEqual(left, right []byte) bool {
	return string(left) == string(right)
}

// recoverPendingActivation completes the checkpoint-to-delivery handoff. The
// old unready record remains the source of truth unless the pending replacement
// proves it is the same admission and payload.
func (q *LogDisk) recoverPendingActivation() error {
	raw, err := q.read(".replace.pending")
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	pending, err := decodeLogRecord(raw)
	if err != nil || !pending.Ready {
		return errors.New("logs spool pending activation invalid")
	}
	name := q.admissions[pending.AdmissionID]
	if name == "" {
		return errors.New("logs spool pending activation has no admitted record")
	}
	currentRaw, err := q.read(name)
	if err != nil {
		return err
	}
	current, err := decodeLogRecord(currentRaw)
	if err != nil || current.Hash != pending.Hash || !bytesEqual(current.Payload, pending.Payload) {
		return errors.New("logs spool pending activation conflicts with admitted record")
	}
	if err = os.Rename(filepath.Join(q.dir, ".replace.pending"), filepath.Join(q.dir, name)); err != nil {
		return err
	}
	if !current.Ready {
		q.names = append(q.names, name)
	}
	return q.syncDir()
}

func validAdmission(v string) bool {
	if len(v) != 64 {
		return false
	}
	_, e := hex.DecodeString(v)
	return e == nil
}

func (q *LogDisk) PutAdmission(ctx context.Context, admissionID string, payload []byte) (Receipt, bool, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if ctx.Err() != nil {
		return "", false, ctx.Err()
	}
	if q.closed {
		return "", false, errors.New("logs spool closed")
	}
	if !validAdmission(admissionID) || len(payload) > 4<<20 || !json.Valid(payload) {
		return "", false, errors.New("invalid logs spool record")
	}
	if name := q.admissions[admissionID]; name != "" {
		return Receipt(name), true, nil
	}
	var compact json.RawMessage = payload
	normalized, err := json.Marshal(compact)
	if err != nil {
		return "", false, err
	}
	h := sha256.Sum256(normalized)
	b, _ := json.Marshal(logRecord{Version: 1, AdmissionID: admissionID, Ready: false, Hash: hex.EncodeToString(h[:]), Payload: normalized})
	cost := charge(int64(len(b)))
	if q.count >= q.maxItems || q.bytes+cost > q.maxBytes {
		return "", false, ErrFull
	}
	name := fmt.Sprintf("%020d.lrec", q.manifest.Next)
	q.manifest.Next++
	if err = q.saveManifest(); err != nil {
		q.closed = true
		return "", false, err
	}
	if err = q.write(name, b); err != nil {
		q.closed = true
		return "", false, err
	}
	q.admissions[admissionID] = name
	q.count++
	q.bytes += cost
	return Receipt(name), false, nil
}

// Activate makes a durable record deliverable only after its file checkpoint
// has been fsynced. Repeated activation is idempotent.
func (q *LogDisk) Activate(ctx context.Context, admissionID string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	name := q.admissions[admissionID]
	if name == "" {
		return nil
	}
	for _, ready := range q.names {
		if ready == name {
			return nil
		}
	}
	b, err := q.read(name)
	if err != nil {
		return err
	}
	r, err := decodeLogRecord(b)
	if err != nil {
		return err
	}
	r.Ready = true
	updated, _ := json.Marshal(r)
	if err = q.writeReplacement(name, updated); err != nil {
		q.closed = true
		return err
	}
	q.names = append(q.names, name)
	sort.Strings(q.names)
	select {
	case q.wake <- struct{}{}:
	default:
	}
	return nil
}

func (q *LogDisk) Next(ctx context.Context) (Item, error) {
	for {
		if ctx.Err() != nil {
			return Item{}, ctx.Err()
		}
		q.mu.Lock()
		if q.closed {
			q.mu.Unlock()
			return Item{}, errors.New("logs spool closed")
		}
		if len(q.names) > 0 {
			name := q.names[0]
			b, err := q.read(name)
			if err != nil {
				q.mu.Unlock()
				return Item{}, err
			}
			r, err := decodeLogRecord(b)
			if err != nil {
				err = q.quarantine(name)
				if err == nil {
					for admissionID, admittedName := range q.admissions {
						if admittedName == name {
							delete(q.admissions, admissionID)
							break
						}
					}
					q.names = q.names[1:]
					q.count--
					q.bytes -= charge(int64(len(b)))
				}
				q.mu.Unlock()
				if err != nil {
					return Item{}, err
				}
				continue
			}
			q.mu.Unlock()
			return Item{Receipt: Receipt(name), Data: r.Payload}, nil
		}
		q.mu.Unlock()
		select {
		case <-ctx.Done():
			return Item{}, ctx.Err()
		case <-q.wake:
		}
	}
}

func (q *LogDisk) Ack(ctx context.Context, receipt Receipt) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if q.closed || len(q.names) == 0 || q.names[0] != string(receipt) {
		return errors.New("logs receipt is not current FIFO head")
	}
	name := q.names[0]
	b, err := q.read(name)
	if err != nil {
		return err
	}
	r, err := decodeLogRecord(b)
	if err != nil {
		return err
	}
	if err = os.Remove(filepath.Join(q.dir, name)); err != nil {
		return err
	}
	if err = q.syncDir(); err != nil {
		q.closed = true
		return err
	}
	delete(q.admissions, r.AdmissionID)
	q.names = q.names[1:]
	q.count--
	q.bytes -= charge(int64(len(b)))
	return nil
}

func (q *LogDisk) Close(context.Context) error {
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
	err := q.lock.Close()
	q.lock = nil
	return err
}
func (q *LogDisk) Corrupt() uint64 { q.mu.Lock(); defer q.mu.Unlock(); return q.corrupt }

func decodeLogRecord(b []byte) (logRecord, error) {
	var r logRecord
	if json.Unmarshal(b, &r) != nil || r.Version != 1 || !validAdmission(r.AdmissionID) || !json.Valid(r.Payload) {
		return r, errors.New("corrupt logs record")
	}
	h := sha256.Sum256(r.Payload)
	if hex.EncodeToString(h[:]) != r.Hash {
		return r, errors.New("logs record checksum mismatch")
	}
	return r, nil
}
func (q *LogDisk) read(name string) ([]byte, error) {
	return readPrivateFile(filepath.Join(q.dir, name), 5<<20)
}
func readPrivateFile(path string, max int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || st.Uid != uint32(os.Geteuid()) || st.Nlink != 1 || info.Size() > max {
		return nil, errors.New("unsafe logs spool file")
	}
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), "logs spool record")
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, max))
}
func (q *LogDisk) write(name string, b []byte) error {
	pending := filepath.Join(q.dir, ".pending")
	f, err := os.OpenFile(pending, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(pending, filepath.Join(q.dir, name)); err != nil {
		return err
	}
	return q.syncDir()
}
func (q *LogDisk) writeReplacement(name string, b []byte) error {
	pending := filepath.Join(q.dir, ".replace.pending")
	f, err := os.OpenFile(pending, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(pending, filepath.Join(q.dir, name)); err != nil {
		return err
	}
	return q.syncDir()
}
func (q *LogDisk) saveManifest() error {
	b, _ := json.Marshal(q.manifest)
	pending := filepath.Join(q.dir, ".manifest.pending")
	f, err := os.OpenFile(pending, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(pending, filepath.Join(q.dir, "manifest.json")); err != nil {
		return err
	}
	return q.syncDir()
}
func (q *LogDisk) syncDir() error {
	f, err := os.Open(q.dir)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
func (q *LogDisk) quarantine(name string) error {
	if err := os.Rename(filepath.Join(q.dir, name), filepath.Join(q.dir, strings.TrimSuffix(name, ".lrec")+".bad")); err != nil {
		return err
	}
	q.corrupt++
	return q.syncDir()
}

// AGENTV1 FILE END
