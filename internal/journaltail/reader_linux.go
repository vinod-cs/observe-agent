//go:build linux

// AGENTV1 FILE START: bounded Linux journald reader with durable-admission cursor checkpoints.
package journaltail

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/agent-i/agent/internal/config"
)

const maxJournalRecordBytes = 1 << 20

type Record struct {
	AdmissionID  string
	Body         string
	ObservedAt   time.Time
	ServiceName  string
	SeverityText string
	Attributes   map[string]string
}

type Stats struct {
	RecordsRead, RecordsRejectedLocal, PermissionErrors, CheckpointErrors, ReaderErrors uint64
}

type Admit func(context.Context, Record) error
type Activate func(context.Context, string) error

type process struct {
	stdout io.ReadCloser
	wait   func() error
}
type starter func(context.Context, []string) (*process, error)

type checkpoint struct {
	Version     int    `json:"version"`
	SourceID    string `json:"source_id"`
	Cursor      string `json:"cursor"`
	SinceMicros int64  `json:"since_micros,omitempty"`
	AdmissionID string `json:"admission_id,omitempty"`
	UpdatedAt   int64  `json:"updated_at"`
}

type Reader struct {
	cfg           config.JournaldLogSource
	checkpointDir string
	admit         Admit
	activate      Activate
	start         starter
	now           func() time.Time
	retryDelay    time.Duration
	cancel        context.CancelFunc
	done          chan struct{}
	mu            sync.Mutex
	stats         Stats
	cp            checkpoint
}

func New(cfg config.JournaldLogSource, checkpointDir string, admit Admit, activate Activate) *Reader {
	return &Reader{cfg: cfg, checkpointDir: checkpointDir, admit: admit, activate: activate, now: time.Now, retryDelay: 5 * time.Second}
}

func (r *Reader) Start(parent context.Context) error {
	if !r.cfg.Enabled {
		return errors.New("journald source disabled")
	}
	if err := ensurePrivateDir(r.checkpointDir); err != nil {
		return err
	}
	if err := r.loadCheckpoint(); err != nil {
		return err
	}
	// start_at=end is represented by a durable initial time watermark. This is
	// not an acknowledged record cursor: it preserves the selection boundary
	// so a crash before the first admitted record cannot skip that record.
	if r.cfg.StartAt == "end" && r.cp.Cursor == "" && r.cp.SinceMicros == 0 {
		r.cp.SinceMicros = r.now().UTC().UnixMicro()
		r.cp.UpdatedAt = r.now().UnixNano()
		if err := r.saveCheckpoint(); err != nil {
			return errors.New("journald initial checkpoint unavailable")
		}
	}
	if r.activate != nil && r.cp.AdmissionID != "" {
		if err := r.activate(parent, r.cp.AdmissionID); err != nil {
			return errors.New("journald admission recovery failed")
		}
	}
	if r.start == nil {
		path, err := exec.LookPath("journalctl")
		if err != nil {
			return errors.New("journalctl unavailable")
		}
		r.start = commandStarter(path)
	}
	ctx, cancel := context.WithCancel(parent)
	r.cancel = cancel
	r.done = make(chan struct{})
	go r.run(ctx)
	return nil
}

func commandStarter(path string) starter {
	return func(ctx context.Context, args []string) (*process, error) {
		cmd := exec.CommandContext(ctx, path, args...)
		cmd.Stderr = io.Discard
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return nil, errors.New("journal reader unavailable")
		}
		if err = cmd.Start(); err != nil {
			if errors.Is(err, os.ErrPermission) {
				return nil, os.ErrPermission
			}
			return nil, errors.New("journal reader unavailable")
		}
		return &process{stdout: stdout, wait: cmd.Wait}, nil
	}
}

func (r *Reader) run(ctx context.Context) {
	defer close(r.done)
	for ctx.Err() == nil {
		p, err := r.start(ctx, r.arguments())
		if err == nil {
			err = r.consume(ctx, p.stdout)
			_ = p.stdout.Close()
			waitErr := p.wait()
			if err == nil {
				err = waitErr
			}
		}
		if ctx.Err() != nil {
			return
		}
		r.mu.Lock()
		r.stats.ReaderErrors++
		if errors.Is(err, os.ErrPermission) {
			r.stats.PermissionErrors++
		}
		r.mu.Unlock()
		timer := time.NewTimer(r.retryDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (r *Reader) arguments() []string {
	args := []string{"--output=json", "--follow", "--no-pager", "--quiet"}
	if r.cp.Cursor != "" {
		args = append(args, "--after-cursor="+r.cp.Cursor)
	} else if r.cp.SinceMicros > 0 {
		args = append(args, "--since=@"+strconv.FormatInt(r.cp.SinceMicros/1_000_000, 10))
	} else if r.cfg.StartAt == "beginning" {
		args = append(args, "--lines=all")
	} else {
		args = append(args, "--lines=0")
	}
	for _, unit := range r.cfg.Units {
		args = append(args, "--unit="+unit)
	}
	for _, identifier := range r.cfg.Identifiers {
		args = append(args, "--identifier="+identifier)
	}
	if r.cfg.Priority != "" {
		args = append(args, "--priority="+r.cfg.Priority)
	}
	return args
}

func (r *Reader) consume(ctx context.Context, input io.Reader) error {
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64<<10), maxJournalRecordBytes)
	for scanner.Scan() {
		record, cursor, ok := parseRecord(scanner.Bytes(), r.now())
		if !ok {
			r.mu.Lock()
			r.stats.RecordsRejectedLocal++
			r.mu.Unlock()
			continue
		}
		if err := r.admit(ctx, record); err != nil {
			return err
		}
		old := r.cp
		r.cp = checkpoint{Version: 1, SourceID: "journald", Cursor: cursor, AdmissionID: record.AdmissionID, UpdatedAt: r.now().UnixNano()}
		if err := r.saveCheckpoint(); err != nil {
			r.cp = old
			r.mu.Lock()
			r.stats.CheckpointErrors++
			r.mu.Unlock()
			return err
		}
		if r.activate != nil {
			if err := r.activate(ctx, record.AdmissionID); err != nil {
				return err
			}
		}
		r.mu.Lock()
		r.stats.RecordsRead++
		r.mu.Unlock()
	}
	if err := scanner.Err(); err != nil {
		return errors.New("journal record stream invalid or exceeds bound")
	}
	return nil
}

func parseRecord(raw []byte, fallback time.Time) (Record, string, bool) {
	var values map[string]any
	if len(raw) == 0 || len(raw) > maxJournalRecordBytes || json.Unmarshal(raw, &values) != nil {
		return Record{}, "", false
	}
	cursor := stringValue(values["__CURSOR"])
	body := stringValue(values["MESSAGE"])
	if cursor == "" || len(cursor) > 4096 || body == "" || len(body) > maxJournalRecordBytes {
		return Record{}, "", false
	}
	observed := fallback.UTC()
	if micros, err := strconv.ParseInt(stringValue(values["__REALTIME_TIMESTAMP"]), 10, 64); err == nil && micros > 0 {
		observed = time.UnixMicro(micros).UTC()
	}
	unit := bounded(stringValue(values["_SYSTEMD_UNIT"]), 256)
	identifier := bounded(stringValue(values["SYSLOG_IDENTIFIER"]), 256)
	service := identifier
	if service == "" {
		service = strings.TrimSuffix(unit, ".service")
	}
	attrs := map[string]string{}
	put := func(key string, value any, max int) {
		if v := bounded(stringValue(value), max); v != "" {
			attrs[key] = v
		}
	}
	put("systemd.unit", values["_SYSTEMD_UNIT"], 256)
	put("log.syslog.identifier", values["SYSLOG_IDENTIFIER"], 256)
	put("log.syslog.priority", values["PRIORITY"], 8)
	put("process.pid", values["_PID"], 32)
	put("process.executable.name", values["_COMM"], 256)
	put("journald.transport", values["_TRANSPORT"], 64)
	h := sha256.Sum256([]byte("observe-journald-admission-v1\x00journald\x00" + cursor))
	return Record{AdmissionID: hex.EncodeToString(h[:]), Body: body, ObservedAt: observed, ServiceName: service, SeverityText: severity(stringValue(values["PRIORITY"])), Attributes: attrs}, cursor, true
}

func stringValue(value any) string {
	s, _ := value.(string)
	return s
}
func bounded(value string, max int) string {
	if len(value) > max || strings.ContainsRune(value, '\x00') {
		return ""
	}
	return value
}
func severity(priority string) string {
	return map[string]string{"0": "FATAL", "1": "FATAL", "2": "FATAL", "3": "ERROR", "4": "WARN", "5": "INFO", "6": "INFO", "7": "DEBUG"}[priority]
}

func (r *Reader) checkpointPath() string { return filepath.Join(r.checkpointDir, "journald.json") }
func (r *Reader) loadCheckpoint() error {
	r.cp = checkpoint{Version: 1, SourceID: "journald"}
	fd, err := syscall.Open(r.checkpointPath(), syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	f := os.NewFile(uintptr(fd), "journald checkpoint")
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || st.Uid != uint32(os.Geteuid()) || st.Nlink != 1 || info.Size() > 16<<10 {
		return errors.New("journald checkpoint is unsafe; retained")
	}
	raw, err := io.ReadAll(io.LimitReader(f, 16<<10))
	if err != nil {
		return err
	}
	var cp checkpoint
	if json.Unmarshal(raw, &cp) != nil || cp.Version != 1 || cp.SourceID != "journald" || len(cp.Cursor) > 4096 || cp.SinceMicros < 0 {
		return errors.New("journald checkpoint invalid; retained")
	}
	r.cp = cp
	return nil
}
func (r *Reader) saveCheckpoint() error {
	raw, _ := json.Marshal(r.cp)
	target, pending := r.checkpointPath(), r.checkpointPath()+".pending"
	f, err := os.OpenFile(pending, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return err
	}
	if _, err = f.Write(raw); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(pending, target); err != nil {
		return err
	}
	d, err := os.Open(r.checkpointDir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

func ensurePrivateDir(dir string) error {
	if !filepath.IsAbs(dir) {
		return errors.New("journald checkpoint directory must be absolute")
	}
	for p := filepath.Clean(dir); ; p = filepath.Dir(p) {
		info, err := os.Lstat(p)
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return errors.New("journald checkpoint path contains symlink")
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
		return errors.New("journald checkpoint directory must be private")
	}
	return nil
}

func (r *Reader) Stop(ctx context.Context) error {
	if r.cancel == nil {
		return nil
	}
	r.cancel()
	select {
	case <-r.done:
		return nil
	case <-ctx.Done():
		return errors.New("journal reader did not stop")
	}
}
func (r *Reader) Stats() Stats { r.mu.Lock(); defer r.mu.Unlock(); return r.stats }

// AGENTV1 FILE END
