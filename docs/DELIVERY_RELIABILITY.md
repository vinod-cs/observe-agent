<!-- AGENTV1 FILE START: persistent metrics delivery specification and evidence -->
# Persistent Linux metrics delivery

> **Current scope v2 update:** [QUEUE_SCOPE_V2.md](QUEUE_SCOPE_V2.md) supersedes the v1 control-state, endpoint-scoping and 16 KiB reserve descriptions below. Record format/delivery guarantees remain unchanged. V2 scope includes protected backend/org IDs, host.id, account and region, excludes endpoint/key, and reserves 24 KiB control space. Ambiguous pending/staging files are retained and fail closed. Historical v1 behavior below is retained for migration context.

This phase changes delivery reliability, not identity, metrics semantics or capability policy. No logs, trace receiver, Windows collector, installer or backend change was added. Reference repositories remain read-only.

## Queue architecture

`collectors.Metrics` now uses `queue.Durable`, implemented on Linux by `queue.Disk`. The previous memory queue remains as a separate tested utility; production does not use destructive `Pop`.

1. Normalize and serialize a bounded standard OTel metrics batch.
2. Allocate a monotonically increasing receipt in version-1 control state.
3. Write a version-1 record containing the payload and SHA-256 checksum to `.pending`, fsync it, rename it, and fsync the directory. Only then does `Put` succeed.
4. One sender peeks the oldest record. Repeated `Next` calls return the same head until acknowledgement.
5. Only a successful full remote acknowledgement permits `Ack`: unlink that exact head and fsync the directory. A duplicate or out-of-order Ack is refused.
6. Shutdown cancels collection/requests/waits, joins workers, then releases the exclusive spool lock. Backlog remains on disk.

Directory is service-owned **0700**, files **0600**. Symlinked directory components and unsafe record files are rejected. Linux `flock` permits only one process per spool. No API key or HTTP headers are serialized. The endpoint/host/account/region scope is hashed in control state; changing it refuses to replay old state under a different host/destination. API-key rotation does not affect this identity.

**Organization boundary:** the Agent cannot determine the organization behind an opaque API key locally. A spool belongs to one organizational deployment. Rotate keys only within that organization; use a separate state directory for another organization. Do not repoint an existing spool to a different tenant. No new authentication mechanism was introduced.

## Configuration and footprint

Existing JSON configurations still load. Effective defaults:

```json
{
  "delivery": {
    "state_directory": "/var/lib/observe-agent/metrics",
    "overflow_policy": "reject_new",
    "queue_items": 64,
    "batch_points": 500,
    "max_attempts": 4,
    "request_timeout_seconds": 15
  },
  "limits": { "queue_bytes": 67108864 }
}
```

This is an excerpt, not a complete configuration. `configs/agent.json` is the complete template. Legacy YAML import remains a separate prerequisite; deployed `/etc/agent-i/agent.yaml` and `/etc/agent-i/env` were not modified.

- Maximum record storage is governed by `limits.queue_bytes` (default 64 MiB) and `delivery.queue_items` (default 64; max 1024), whichever fills first. Each batch, not each metric point, is one record.
- Accounting rounds record sizes up to 4 KiB and reserves 16 KiB plus 256 bytes per configured item rounded to 4 KiB for control/directory work. Allocation units over 4 KiB are explicitly rejected. Filesystem-wide journal, inode and snapshot overhead are not an enforceable per-directory quota; use a filesystem quota for a hard physical-volume ceiling.
- Files are `manifest.json`, `manifest.backup`, `lock`, at most one transient `.pending`, and sequence-numbered `.rec` / quarantined `.bad` records. No ever-growing append WAL, telemetry log or unbounded quarantine is used.
- Corrupt records count against both limits. Overflow rejects and counts **new** batches; it never evicts previously accepted telemetry. Retention duration depends on points/batches per scrape, not just the byte limit. At eight batches/scrape, 64 slots cover about eight scrapes.
- Spool requires a local persistent Linux filesystem with working fsync/rename/flock semantics. Do not use NFS, a Windows bind mount, or volatile `/tmp`/tmpfs for production state. Tests intentionally use isolated local temporary directories.
- Default parent must be writable by the chosen service identity, or pre-created with appropriate ownership by an operator. This task does not install/chown a service directory. The runtime does not silently fall back to memory.

## Delivery guarantee and failure semantics

**At-least-once for durably accepted, uncorrupted records, conditional on eventual remote acceptance and retained state. Not exactly-once.** A crash after remote success but before durable local Ack can replay an identical batch. Original metric identities/timestamps remain unchanged. Backend deduplication must be verified live; no claim is made that generic HTTP retry creates exactly-once storage.

| Failure | Behavior |
|---|---|
| Process crash/restart | Replay retained records oldest-first; exclusive lock is released by the kernel on process exit |
| Network / transient 5xx | Bounded attempts per cycle, exponential jitter, then a cancellation-aware 30–31s cycle cooldown; head stays on disk |
| 429 / Retry-After | Numeric/date server delay honored, including the final attempt in a cycle; long waits are cancelable |
| 401/403, invalid credentials, normal permanent 4xx, 501/505 | Pause delivery and scraping for this process; retain backlog; operator corrects the issue then explicitly restarts |
| Invalid TLS trust/hostname or refused redirect | Pause; never disable certificate validation or forward credentials to redirect destination |
| Partial success / malformed successful response | Pause with original batch retained; do not blindly replay an accepted subset. Operator must investigate before restarting |
| Queue full | Reject new records, increment rejection/drop counters; existing records continue draining |
| Corrupt record | Rename to bounded `.bad` quarantine, count corruption, continue with later valid records; corrupted evidence is not silently deleted |
| Malformed primary manifest | Recover from durable backup; a valid incompatible version/scope is not bypassed |
| Both manifests unusable, unsafe path or unknown state | Fail closed with an error, retain all files for recovery; no panic or destructive reset. Operator repair is required |
| Shutdown during retry | Cancel promptly; no dequeue or dropped count for retained records |
| Disk write/fsync/ack error | Report failure/pause rather than claim delivery; disk may contain an ambiguous record after an interrupted commit |

`Retry-After` waiting is process-local: restarting the Agent can initiate a new request before the previous server deadline. Avoid restart loops during throttling. Persisting server cooldown across process restarts is a remaining refinement, not a delivery loss.

Self-telemetry (also emitted as host-scoped metrics and redacted stderr summaries): `accepted_records` counts batches offered for enqueue; `queued_records` counts successful durable puts; `retried_records` counts retry attempts/cycles, not unique points; `delivered_records` counts successful local Ack after remote acceptance; `dropped_batches` counts rejected new batches; `corrupt_records` counts quarantines; `delivery_paused` identifies remediation state. Existing HTTP/throttle/auth/rejected-point counters remain. Counters reset on process restart; queued telemetry does not. Credentials and payloads are never diagnostic fields.

## Metrics-only behavior

OS/identity reads and metric names remain as documented in [Linux metrics](LINUX_METRICS.md). The only new runtime reads/writes are the private spool and filesystem metadata checks. No application log files, trace listener, inbound port, package installation, shell subprocess, cloud API collector or backend identity change was introduced. IMDSv2 remains link-local and proxy-free; export remains outbound verified HTTPS with `Authorization: ApiKey …` supplied by the named environment variable.

## Validation

Tests exercise abrupt child-process exit without Close, restart/FIFO replay, duplicate Ack rejection, exclusive writer lock, full item/byte limits, permissions, malformed manifest backup recovery, corrupt-record quarantine, scope mismatch, real TLS 401/422/429/503 failures, cancellation during a long server delay, and successful worker restart without duplicate local dequeue. Existing exporter tests cover refused endpoints, TLS verification, Retry-After/date/jitter and partial success. Existing identity and policy tests remain unchanged.

Live AWS authentication, actual IMDS, canonical UUID reuse and ClickHouse persistence are **not locally proven** by those tests. See [single-host live validation](LIVE_EC2_VALIDATION.md). No deployed Agent was changed.
<!-- AGENTV1 FILE END -->
