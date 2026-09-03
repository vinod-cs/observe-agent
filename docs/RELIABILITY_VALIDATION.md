<!-- AGENTV1 FILE START: delivery-reliability verification and exact change manifest -->
# Reliability phase verification

Verified 2026-09-02. Work was confined to `D:/ob-cs-repo/Updated-Agent-v1`. Reference Agent/Observe files were read-only. No commit, package installation, deployed file change, real credential use or fleet deployment occurred.

## Results

| Check | Result |
|---|---|
| Native Windows `go test -count=1 ./...` | PASS — 33 top-level tests, 11 test-bearing packages; Linux collector/spool tests are build-tagged |
| Native Linux AMD64 execution of all package test binaries | PASS — 45 top-level tests, including five persistent-worker failure/restart subcases |
| `go vet ./...` Windows / Linux-target | PASS |
| Linux AMD64 / Linux ARM64 / Windows AMD64 `go build ./...` and CLI binaries | PASS — CGO disabled |
| Crash without Close -> reopen -> FIFO replay | PASS — separate child process exits after durable enqueue; same receipts/data recovered |
| Endpoint unavailable, 401, 422, 429, 503 -> restart | PASS — real durable worker retains its head; successful recovery acknowledges it once |
| 429 final attempt Retry-After | PASS — 30-minute requested delay observed by test sleeper; actual worker wait canceled by shutdown |
| Full byte/item limits and private ownership | PASS — reject_new; no overwrite of accepted backlog; non-private directory refused |
| Corrupt record / malformed manifest | PASS — bounded retained quarantine and valid manifest-backup recovery |
| Duplicate Ack / exclusive spool writer / changed host scope | PASS — rejected |
| Existing IMDSv2, identity, policy, TLS/partial-response regressions | PASS |
| Real Linux OS -> standard pdata -> disk -> localhost TLS | PASS — 219 metric points; actual OS reads with test-only EC2 identity fixture because the VM has no machine-id |
| Current Observe backend normalization, read-only | PASS — 2,001 points, zero rejected; exact EC2 resource identity and metric units preserved |
| Windows/Linux binary `--check` of current example config | PASS — no collectors/listeners/secrets/network started |
| Module checksum verification | PASS |
| Source trailing whitespace | PASS — zero findings |
| Git diff check | NOT APPLICABLE — this target has no `.git`; Git was not initialized |
| Real AWS IMDS + organization key + live persistence/UI | NOT RUN — runbook prepared |
| Existing canonical EC2 UUID / no duplicate entity / live source rows | NOT VERIFIED — requires authorized live evidence |
| Native ARM64 or Windows collection | NOT RUN / NOT IMPLEMENTED — compile safety only; Windows collectors remain unsupported |

Linux runs used the already-existing Docker Desktop WSL Linux VM, with test binaries cross-compiled by the Windows Go 1.26.7 toolchain. Linux tests actually executed under Linux; they were not merely cross-compiled. No Linux Go toolchain was installed, so native `go test` orchestration itself was replaced by execution of each `go test -c` binary. CI already runs ordinary `go test ./...` on Linux and Windows.

Local TLS/IMDS and backend-normalizer fixtures are test data only and were never sent to a real Observe endpoint. A filesystem-cardinality issue was correctly reported on the Linux VM's many mounts. No synthetic telemetry was inserted into a development database.

## Candidate build outputs (not releases)

| Path under repository | SHA256 |
|---|---|
| `dist/bin/linux_amd64/observe-agent` | `5B9307481E09F99FFD315BF7E59D3D74E5BCAC3E9D63DD1A158EDDFBFD4977B2` |
| `dist/bin/linux_arm64/observe-agent` | `AADD62FF6E7BB980FD721E56DA925BFDFB74CF94DF298699691FD993823C4841` |
| `dist/bin/windows_amd64/observe-agent.exe` | `3F15AD19DD414E62B054FF7EF7358403D45A069C4068B718D65F7464D02DEB35` |

Build/cache/test outputs remain under ignored `dist/`. Version/release metadata was not promoted to a new release.

## Files changed

New files:

```text
internal/queue/disk_linux.go
internal/queue/disk_other.go
internal/queue/disk_linux_test.go
internal/collectors/reliability_linux_test.go
internal/exporter/reliability_test.go
docs/DELIVERY_RELIABILITY.md
docs/LIVE_EC2_VALIDATION.md
docs/RELIABILITY_VALIDATION.md
```

Updated files:

```text
internal/queue/queue.go
internal/config/runtime.go
internal/collectors/metrics.go
internal/collectors/metrics_test.go
internal/collectors/metrics_linux_test.go
internal/exporter/exporter.go
internal/exporter/sender.go
internal/exporter/sender_test.go
internal/selftelemetry/metrics.go
configs/agent.json
README.md
docs/ARCHITECTURE.md
docs/LINUX_METRICS.md
docs/METRICS_VALIDATION.md
```

Collector/identity/normalization models, existing metric names/units, cloud detector and platform backend were not changed. The existing in-memory queue utility was retained. Source/test/docs changes carry AGENTV1 markers; the strict JSON example cannot contain comments and is listed here.

## Remaining packaging/replacement gates

1. Execute [single-host live EC2 validation](LIVE_EC2_VALIDATION.md): real authentication, existing canonical UUID, no duplicate Host/EC2, ClickHouse persistence and independent Agent/AWS provenance. No backend identity change is permitted to manufacture a pass.
2. Test legacy YAML/env import and upgrade/rollback, and account for existing logs/traces before replacing a multi-signal deployed Agent. This metrics-only binary is not a drop-in replacement.
3. Long-running restricted-account and filesystem durability/ENOSPC/soak testing. Fsync guarantees depend on the local filesystem/storage stack. Kernel-blocked filesystem calls are not forcibly interruptible. Memory configuration is not a hard RSS cap.
4. Operational handling of retained permanent failures, partial-acceptance batches and bounded quarantine; both corrupt manifests fail closed rather than reconstructing an untrusted scope. The Agent exposes pause/drop counters but no repair UI/tool in this phase.
5. Use one spool per organizational deployment. Opaque key organization cannot be validated locally; key rotation must remain within the same tenant. Server cooldown is not persisted across process restart, so do not restart-loop on throttling.
6. Signed release artifacts, service account/state directory ownership, install/upgrade/rollback and packaging remain deliberately deferred.

Implementation and local validation are complete for this bounded metrics delivery phase; **live EC2 acceptance is not complete**.
<!-- AGENTV1 FILE END -->
