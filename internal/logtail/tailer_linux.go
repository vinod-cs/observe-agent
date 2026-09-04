//go:build linux

// AGENTV1 FILE START: narrow secure Linux file tailer with durable-admission checkpoints.
package logtail

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/agent-i/agent/internal/config"
	"golang.org/x/sys/unix"
)

type Record struct {
	AdmissionID            string
	Body                   string
	RelativePath           string
	FileIdentity           string
	StartOffset, EndOffset int64
	ObservedAt             time.Time
}
type Admit func(context.Context, Record) error
type Activate func(context.Context, string) error
type Counters struct{ FilesDiscovered, FilesOpen, RecordsRead, RecordsRejectedLocal, PermissionErrors, CheckpointErrors, MultilineFlushes uint64 }
type checkpoint struct {
	Offset       int64  `json:"offset"`
	Generation   uint64 `json:"generation"`
	Fingerprint  string `json:"fingerprint"`
	RelativePath string `json:"relative_path"`
	UpdatedAt    int64  `json:"updated_at"`
	AdmissionID  string `json:"admission_id,omitempty"`
}
type checkpointState struct {
	Version  int                   `json:"version"`
	SourceID string                `json:"source_id"`
	Files    map[string]checkpoint `json:"files"`
}
type pending struct {
	body       []byte
	start, end int64
	lines      int
	since      time.Time
}
type openState struct {
	file                            *os.File
	identity, relative, fingerprint string
	committed, readOffset           int64
	generation                      uint64
	pending                         pending
	lostAt                          time.Time
}

type Tailer struct {
	mu            sync.Mutex
	source        config.LogSource
	checkpointDir string
	maxFiles      int
	poll          time.Duration
	admit         Admit
	activate      Activate
	root          *os.File
	states        map[string]*openState
	checkpoints   checkpointState
	start         *regexp.Regexp
	cancel        context.CancelFunc
	done          chan struct{}
	stats         Counters
	now           func() time.Time
}

func New(source config.LogSource, checkpointDir string, maxFiles int, poll time.Duration, admit Admit, activate Activate) *Tailer {
	return &Tailer{source: source, checkpointDir: checkpointDir, maxFiles: maxFiles, poll: poll, admit: admit, activate: activate, states: map[string]*openState{}, now: time.Now}
}

func (t *Tailer) Start(parent context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cancel != nil {
		return errors.New("log source already started")
	}
	if t.admit == nil {
		return errors.New("log durable admission unavailable")
	}
	if t.source.Multiline.Enabled {
		r, err := regexp.Compile(t.source.Multiline.StartPattern)
		if err != nil {
			return errors.New("log multiline pattern invalid")
		}
		t.start = r
	}
	root, err := openRoot(t.source.Root)
	if err != nil {
		return redactOpen(err)
	}
	if err = ensurePrivateDir(t.checkpointDir); err != nil {
		root.Close()
		return err
	}
	t.root = root
	if err = t.loadCheckpoint(); err != nil {
		root.Close()
		t.root = nil
		return err
	}
	t.recoverAdmissions(parent)
	ctx, cancel := context.WithCancel(parent)
	t.cancel = cancel
	t.done = make(chan struct{})
	go func() {
		defer close(t.done)
		ticker := time.NewTicker(t.poll)
		defer ticker.Stop()
		t.pollOnce(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				t.pollOnce(ctx)
			}
		}
	}()
	return nil
}

func (t *Tailer) Stop(ctx context.Context) error {
	t.mu.Lock()
	cancel, done := t.cancel, t.done
	t.cancel = nil
	t.mu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	select {
	case <-done:
	case <-ctx.Done():
		return errors.New("log source did not stop before deadline")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, state := range t.states {
		state.file.Close()
	}
	t.states = map[string]*openState{}
	if t.root != nil {
		t.root.Close()
		t.root = nil
	}
	return nil
}
func (t *Tailer) Stats() Counters {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := t.stats
	out.FilesOpen = uint64(len(t.states))
	return out
}

func (t *Tailer) pollOnce(ctx context.Context) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if ctx.Err() != nil || t.root == nil {
		return
	}
	t.recoverAdmissions(ctx)
	names, err := readRootNames(int(t.root.Fd()), t.maxFiles+1)
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			t.stats.PermissionErrors++
		}
		return
	}
	if len(names) > t.maxFiles {
		names = names[:t.maxFiles]
	}
	sort.Strings(names)
	seen := map[string]bool{}
	for _, name := range names {
		if !t.matches(name) {
			continue
		}
		file, st, err := openBeneath(int(t.root.Fd()), name)
		if err != nil {
			if errors.Is(err, os.ErrPermission) {
				t.stats.PermissionErrors++
			}
			continue
		}
		identity := fmt.Sprintf("%x:%x", st.Dev, st.Ino)
		seen[identity] = true
		if existing := t.states[identity]; existing != nil {
			existing.relative = name
			existing.lostAt = time.Time{}
			file.Close()
			continue
		}
		if len(t.states) >= t.source.MaxOpenFiles {
			file.Close()
			continue
		}
		fp, err := fingerprint(file)
		if err != nil {
			file.Close()
			continue
		}
		cp, known := t.checkpoints.Files[identity]
		generation, offset := cp.Generation, cp.Offset
		if known && cp.Fingerprint != fp {
			generation++
			known = false
		}
		if !known {
			if t.source.StartAt == "end" {
				offset = st.Size
			}
			cp = checkpoint{Offset: offset, Generation: generation, Fingerprint: fp, RelativePath: name, UpdatedAt: t.now().UnixNano()}
			t.checkpoints.Files[identity] = cp
			if t.saveCheckpoint() != nil {
				t.stats.CheckpointErrors++
				file.Close()
				continue
			}
		}
		t.states[identity] = &openState{file: file, identity: identity, relative: name, fingerprint: fp, committed: offset, readOffset: offset, generation: generation}
		t.stats.FilesDiscovered++
	}
	now := t.now()
	for id, state := range t.states {
		if !seen[id] && state.lostAt.IsZero() {
			state.lostAt = now
		}
		t.consume(ctx, state, now)
		if !state.lostAt.IsZero() && now.Sub(state.lostAt) > time.Minute && state.pending.lines == 0 {
			state.file.Close()
			delete(t.states, id)
		}
	}
}

func (t *Tailer) consume(ctx context.Context, state *openState, now time.Time) {
	info, err := state.file.Stat()
	if err != nil {
		return
	}
	if info.Size() < state.committed {
		state.generation++
		state.committed = 0
		state.readOffset = 0
		state.pending = pending{}
		fp, e := fingerprint(state.file)
		if e == nil {
			state.fingerprint = fp
		}
		if !t.commit(state, 0, "") {
			return
		}
	}
	for state.readOffset < info.Size() {
		line, end, complete, tooLarge, err := readLine(state.file, state.readOffset, t.source.MaxLineBytes)
		if err != nil || !complete {
			break
		}
		start := state.readOffset
		state.readOffset = end
		t.stats.RecordsRead++
		line = bytesWithoutLineEnding(line)
		if tooLarge {
			t.stats.RecordsRejectedLocal++
			if !t.commit(state, end, "") {
				return
			}
			continue
		}
		if !utf8.Valid(line) {
			line = []byte(strings.ToValidUTF8(string(line), "�"))
		}
		if !t.source.Multiline.Enabled {
			if len(line) == 0 {
				t.stats.RecordsRejectedLocal++
				if !t.commit(state, end, "") {
					return
				}
				continue
			}
			if !t.emit(ctx, state, start, end, line, now) {
				state.readOffset = state.committed
				return
			}
			continue
		}
		if t.start.Match(line) && state.pending.lines > 0 {
			if !t.flush(ctx, state, now) {
				state.readOffset = start
				return
			}
		}
		if state.pending.lines == 0 {
			state.pending = pending{start: start, since: now}
		}
		additional := len(line)
		if state.pending.lines > 0 {
			additional++
		}
		if state.pending.lines >= t.source.Multiline.MaxLines || len(state.pending.body)+additional > t.source.Multiline.MaxBytes {
			if !t.flush(ctx, state, now) {
				state.readOffset = start
				return
			}
			state.pending = pending{start: start, since: now}
		}
		if state.pending.lines > 0 {
			state.pending.body = append(state.pending.body, '\n')
		}
		state.pending.body = append(state.pending.body, line...)
		state.pending.lines++
		state.pending.end = end
	}
	if state.pending.lines > 0 && now.Sub(state.pending.since) >= time.Duration(t.source.Multiline.FlushTimeoutMillis)*time.Millisecond {
		t.flush(ctx, state, now)
	}
}
func (t *Tailer) flush(ctx context.Context, state *openState, now time.Time) bool {
	if state.pending.lines == 0 {
		return true
	}
	p := state.pending
	if !t.emit(ctx, state, p.start, p.end, p.body, now) {
		return false
	}
	state.pending = pending{}
	t.stats.MultilineFlushes++
	return true
}
func (t *Tailer) emit(ctx context.Context, state *openState, start, end int64, body []byte, now time.Time) bool {
	id := admissionID(t.source.ID, state.identity, state.generation, start, end)
	if t.admit(ctx, Record{AdmissionID: id, Body: string(body), RelativePath: state.relative, FileIdentity: state.identity, StartOffset: start, EndOffset: end, ObservedAt: now.UTC()}) != nil {
		return false
	}
	if !t.commit(state, end, id) {
		return false
	}
	if t.activate != nil && t.activate(ctx, id) != nil {
		return false
	}
	return true
}
func (t *Tailer) commit(state *openState, offset int64, admissionID string) bool {
	old, existed := t.checkpoints.Files[state.identity]
	t.checkpoints.Files[state.identity] = checkpoint{Offset: offset, Generation: state.generation, Fingerprint: state.fingerprint, RelativePath: state.relative, UpdatedAt: t.now().UnixNano(), AdmissionID: admissionID}
	if err := t.saveCheckpoint(); err != nil {
		if existed {
			t.checkpoints.Files[state.identity] = old
		} else {
			delete(t.checkpoints.Files, state.identity)
		}
		t.stats.CheckpointErrors++
		return false
	}
	state.committed = offset
	if state.readOffset < offset {
		state.readOffset = offset
	}
	return true
}
func (t *Tailer) recoverAdmissions(ctx context.Context) {
	if t.activate == nil {
		return
	}
	for _, cp := range t.checkpoints.Files {
		if cp.AdmissionID != "" {
			_ = t.activate(ctx, cp.AdmissionID)
		}
	}
}
func (t *Tailer) matches(name string) bool {
	included := false
	for _, pattern := range t.source.Include {
		if ok, _ := path.Match(pattern, name); ok {
			included = true
			break
		}
	}
	if !included {
		return false
	}
	for _, pattern := range t.source.Exclude {
		if ok, _ := path.Match(pattern, name); ok {
			return false
		}
	}
	return true
}
func admissionID(source, identity string, generation uint64, start, end int64) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("observe-log-admission-v1\x00%s\x00%s\x00%d\x00%d\x00%d", source, identity, generation, start, end)))
	return hex.EncodeToString(h[:])
}
func fingerprint(f *os.File) (string, error) {
	b := make([]byte, 256)
	n, err := f.ReadAt(b, 0)
	if err != nil && err != io.EOF {
		return "", err
	}
	h := sha256.Sum256(b[:n])
	return hex.EncodeToString(h[:]), nil
}
func readLine(f *os.File, offset int64, max int) ([]byte, int64, bool, bool, error) {
	out := make([]byte, 0, min(max, 4096))
	pos := offset
	buf := make([]byte, 4096)
	tooLarge := false
	for {
		n, err := f.ReadAt(buf, pos)
		if n > 0 {
			for i := 0; i < n; i++ {
				pos++
				if !tooLarge {
					if len(out) >= max {
						tooLarge = true
					} else {
						out = append(out, buf[i])
					}
				}
				if buf[i] == '\n' {
					return out, pos, true, tooLarge, nil
				}
			}
		}
		if err == io.EOF {
			return out, pos, false, tooLarge, nil
		}
		if err != nil {
			return nil, offset, false, false, err
		}
	}
}
func bytesWithoutLineEnding(b []byte) []byte {
	if len(b) > 0 && b[len(b)-1] == '\n' {
		b = b[:len(b)-1]
	}
	if len(b) > 0 && b[len(b)-1] == '\r' {
		b = b[:len(b)-1]
	}
	return b
}

func (t *Tailer) checkpointPath() string {
	h := sha256.Sum256([]byte(t.source.ID))
	return filepath.Join(t.checkpointDir, hex.EncodeToString(h[:])+".json")
}
func (t *Tailer) loadCheckpoint() error {
	t.checkpoints = checkpointState{Version: 1, SourceID: t.source.ID, Files: map[string]checkpoint{}}
	checkpointPath := t.checkpointPath()
	fd, err := syscall.Open(checkpointPath, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	f := os.NewFile(uintptr(fd), "log checkpoint")
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || st.Uid != uint32(os.Geteuid()) || st.Nlink != 1 || info.Size() > 4<<20 {
		return errors.New("log checkpoint is unsafe; retained")
	}
	b, err := io.ReadAll(io.LimitReader(f, 4<<20))
	if err != nil {
		return err
	}
	var state checkpointState
	if json.Unmarshal(b, &state) != nil || state.Version != 1 || state.SourceID != t.source.ID || state.Files == nil || len(state.Files) > t.maxFiles*4 {
		return errors.New("log checkpoint invalid; retained")
	}
	t.checkpoints = state
	return nil
}
func (t *Tailer) saveCheckpoint() error {
	b, _ := json.Marshal(t.checkpoints)
	target := t.checkpointPath()
	tmp := target + ".pending"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|syscall.O_NOFOLLOW, 0600)
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
	if err = os.Rename(tmp, target); err != nil {
		return err
	}
	d, err := os.Open(t.checkpointDir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

func openRoot(root string) (*os.File, error) {
	slash, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	defer unix.Close(slash)
	relative := strings.TrimPrefix(filepath.Clean(root), "/")
	how := &unix.OpenHow{Flags: unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC | unix.O_NOFOLLOW, Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_MAGICLINKS | unix.RESOLVE_NO_SYMLINKS}
	fd, err := unix.Openat2(slash, relative, how)
	if err == nil {
		return os.NewFile(uintptr(fd), "log root"), nil
	}
	if err != unix.ENOSYS && err != unix.EINVAL {
		return nil, err
	}
	current, err := unix.Dup(slash)
	if err != nil {
		return nil, err
	}
	for _, part := range strings.Split(relative, string(os.PathSeparator)) {
		if part == "" || part == "." || part == ".." {
			unix.Close(current)
			return nil, errors.New("unsafe log root")
		}
		next, e := unix.Openat(current, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		unix.Close(current)
		if e != nil {
			return nil, e
		}
		current = next
	}
	return os.NewFile(uintptr(current), "log root"), nil
}
func openBeneath(rootfd int, name string) (*os.File, *unix.Stat_t, error) {
	if strings.ContainsAny(name, "/\\\x00") || name == "." || name == ".." {
		return nil, nil, errors.New("unsafe log path")
	}
	how := &unix.OpenHow{Flags: unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW, Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_MAGICLINKS | unix.RESOLVE_NO_SYMLINKS}
	fd, err := unix.Openat2(rootfd, name, how)
	if err == unix.ENOSYS || err == unix.EINVAL {
		fd, err = unix.Openat(rootfd, name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	}
	if err != nil {
		return nil, nil, err
	}
	var st unix.Stat_t
	if err = unix.Fstat(fd, &st); err != nil || st.Mode&unix.S_IFMT != unix.S_IFREG {
		unix.Close(fd)
		if err == nil {
			err = errors.New("log source is not a regular file")
		}
		return nil, nil, err
	}
	return os.NewFile(uintptr(fd), "log source"), &st, nil
}
func readRootNames(rootfd, limit int) ([]string, error) {
	fd, err := unix.Openat(rootfd, ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), "log root scan")
	defer f.Close()
	names, err := f.Readdirnames(limit)
	if err == io.EOF {
		err = nil
	}
	return names, err
}
func ensurePrivateDir(dir string) error {
	if !filepath.IsAbs(dir) {
		return errors.New("checkpoint directory must be absolute")
	}
	for p := filepath.Clean(dir); ; p = filepath.Dir(p) {
		info, err := os.Lstat(p)
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return errors.New("checkpoint path contains symlink")
		}
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if filepath.Dir(p) == p {
			break
		}
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.IsDir() || info.Mode().Perm() != 0700 || st.Uid != uint32(os.Geteuid()) {
		return errors.New("checkpoint directory must be owned by current user with mode 0700")
	}
	return nil
}
func redactOpen(err error) error {
	if errors.Is(err, unix.EACCES) || errors.Is(err, unix.EPERM) {
		return os.ErrPermission
	}
	return errors.New("configured log root is unavailable or unsafe")
}

// AGENTV1 FILE END
