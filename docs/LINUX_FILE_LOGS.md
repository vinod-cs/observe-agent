<!-- AGENTV1 FILE START: implemented Linux file/journald Logs v1 security, durability, and operations. -->
# Linux Logs v1: files and journald

Linux Logs is an opt-in collector. It is disabled by default and does not open
application files or start a journal reader when disabled. It adds no listener
or inbound port. File and journald sources feed the same standard OTLP Logs
pipeline and independent durable Logs queue.
Metrics configuration, Metrics queue format/scope/migration, and Metrics failure
handling remain independent.

## Configuration

Each source authorizes one absolute, non-root directory and direct-child filename
patterns. Recursive traversal and patterns containing path separators are rejected.

```yaml
collection:
  logs:
    enabled: true
    state_directory: /var/lib/observe-agent/logs
    queue_bytes: 67108864
    queue_items: 1024
    overflow_policy: reject_new
    poll_interval: 1s
    max_files: 256
    journald:
      enabled: true
      units: ["sshd.service", "my-application.service"]
      identifiers: ["sshd", "my-application"]
      priority: warning # 0..7 or emerg..debug; empty means all
      start_at: end     # beginning or end on first start; cursor thereafter
    files:
      - id: application
        root: /var/log/my-application
        include: ["*.log"]
        exclude: ["*.gz"]
        start_at: end # beginning or end
        service_name: my-application
        environment: production
        max_open_files: 32
        max_line_bytes: 262144
        multiline:
          enabled: false
          start_pattern: "^\\d{4}-\\d{2}-\\d{2}"
          flush_timeout: 5s
          max_lines: 200
          max_bytes: 262144
```

The restricted `observe-agent` account needs directory traversal and read access
only for the configured roots/files. Grant a targeted group or ACL; do not run the
Agent as root and do not broaden permissions on unrelated logs.

### Journald access

The DEB does not add the service account to a privileged group. An operator must
grant only the journal access required by local policy. On common systemd Linux
distributions, the explicit group-based option is:

```bash
sudo usermod -aG systemd-journal observe-agent
sudo systemctl restart observe-agent
sudo -u observe-agent journalctl --unit=sshd.service --lines=1 --no-pager
```

An administrator may use a narrower journal ACL instead. If access is absent,
the reader records a permission/reader error and retries with a bounded delay;
Metrics and file Logs remain independent. The Agent never changes journal group
membership or ACLs itself.

Journald invokes the installed `journalctl` executable directly with fixed,
validated argument values. No shell is used and values are not interpolated.
`units`, `identifiers`, and `priority` are optional allow/filter inputs. The
collector emits only a bounded metadata allowlist (`systemd.unit`, syslog
identifier/priority, PID, command name, and transport); it never emits the cursor.

## Security boundary

The tailer opens configured roots using Linux `openat2` with `RESOLVE_BENEATH`,
`RESOLVE_NO_SYMLINKS`, and `RESOLVE_NO_MAGICLINKS`. Kernels without `openat2` use
a component-by-component `openat(O_NOFOLLOW)` root open and a direct-child
`openat(O_NOFOLLOW)` file open. Only regular files are accepted. Hostname, IP, DNS,
file content, and API-key identity are never used as canonical host identity.

The source root, state, queue, and checkpoint paths reject symlinks. Queue and
checkpoint directories are service-owned `0700`; state files are `0600`.

## Rotation and multiline

Device/inode identity distinguishes files. Rename-and-replace retains the renamed
descriptor long enough to finish appended data while opening the replacement as a
new file. Copytruncate increments a generation and resumes at byte zero. Admission
identity is a SHA-256 over source ID, device/inode, generation, and byte range.

Multiline assembly is optional and bounded by maximum bytes, lines, and flush time.
Oversized and empty records are rejected locally and counted. Invalid UTF-8 is
replaced deterministically rather than crashing the pipeline.

## Durability and delivery

Logs has an independent versioned FIFO spool and checkpoint store. For each record:

1. standard OTLP `plog.Logs` JSON is constructed;
2. an unready queue record is fsynced;
3. the file checkpoint with admission ID is fsynced;
4. the queue record is atomically activated for delivery.

The checkpoint never advances when admission fails. Interrupted manifest,
admission, and activation renames are recovered without deleting backlog; ambiguous
conflicts fail closed. HTTP 429 and transient 5xx/network failures retain records
and retry with bounded, cancellation-aware delay. HTTP 401/403 pauses Logs delivery
with backlog retained. Permanent payload/configuration failures stop that Logs
delivery path. Metrics continues to use its separate collector and queue.

Observe's OTLP Logs partial-success response has no per-record rejection identity.
When `rejectedLogRecords` is present, the server response is final: the durable
batch is acknowledged, rejected records are counted, and the batch is not retried.

Delivery is at-least-once. A remote acceptance followed by local acknowledgement
loss may replay a record after restart; exactly-once delivery is not claimed.

Journald uses the same ordering. Its stable admission identity is derived from
the journal cursor. The cursor is fsynced to
`/var/lib/observe-agent/logs/checkpoints/journald.json` only after the OTLP record
has been durably admitted; the queue record is then activated. On restart the
saved cursor is supplied as `--after-cursor`, and any admitted-but-not-activated
record is activated before reading resumes. Queue-full and checkpoint failures
never advance the cursor.

## OTLP and provenance

Serialization uses `go.opentelemetry.io/collector/pdata/plog` v1.60.0. Resource
attributes come from the same trusted EC2/machine identity resolver as Metrics.
File records add source ID, basename, device/inode identity, and byte offsets.
Optional `service_name` and `environment` are emitted as resource attributes.
Credentials are used only in the Authorization header and are never queued,
checkpointed, logged, or placed in telemetry.

## Current limits

- Linux only; Windows and macOS compile-safe builds reject Logs at runtime.
- Linux files and systemd journald only. Containers, Windows Event Log, macOS
  unified logging, compression, recursive globs, encodings, and general
  transformation pipelines remain out of scope.
- Journal access varies by distribution and local retention policy. `start_at:
  beginning` can read only entries still retained by journald.
- Per-record severity/JSON parsing is deferred; the complete line/multiline message
  is preserved for backend normalization.
- Package validation is local/isolated. Live production Logs acceptance still
  requires an approved canary and is not claimed by this document.

<!-- AGENTV1 FILE END -->
