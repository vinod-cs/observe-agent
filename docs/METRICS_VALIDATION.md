<!-- AGENTV1 FILE START: actual metrics-slice verification and handoff -->
# Linux metrics slice validation

> Historical slice evidence. The former memory-queue limitation is superseded by [persistent delivery verification](RELIABILITY_VALIDATION.md). Live AWS acceptance remains unverified.

Date: 2026-09-02. Only `D:/ob-cs-repo/Updated-Agent-v1` was modified. Both reference repositories stayed read-only. No Agent service, package, real AWS endpoint, platform database or live ingest credential was changed. Nothing was committed or deployed.

## Pass/fail matrix

| Verification | Result |
|---|---|
| Windows native `go test -count=1 ./...` | PASS: 34 top-level tests / 62 including subtests, 12 test-bearing packages |
| Linux AMD64 native execution of every package test binary | PASS: 38 top-level tests; executed on existing Docker Desktop WSL Linux, not just cross-compiled |
| Linux real OS collection | PASS: 201 points; CPU/memory/load/network/disk/filesystem observed; filesystem cardinality cap explicitly reported |
| Full Linux metrics startup -> real OS reads -> pdata -> localhost TLS | PASS: 213 points sent and acknowledged; exact identity supplied by test-only EC2 detector fixture because the Linux test VM lacks a valid machine-id |
| IMDSv2 identity exchange | PASS with local HTTP fixtures: token, document, exact account/region/instance/ARN, malformed/oversize document, deadline, no IMDSv1 downgrade |
| Actual backend normalizer, read-only | PASS: 2,001 generated points across bounded batches, zero rejected, exact EC2 identity and units retained |
| Endpoint unavailable / 401 / 403 / 429 / 5xx / 422 | PASS via local TLS fault injection; retries bounded, permanent auth suspended, Retry-After respected |
| Partial success, malformed acknowledgement and untrusted TLS | PASS: rejected/recorded, no blind partial-batch replay, untrusted server receives no credential |
| Queue saturation | PASS: byte/item caps, ownership copy, closed state and cancellation |
| Disabled metrics / unimplemented signals | PASS: no reader/queue/sender for disabled metric collector; existing capability preflight prevents starting unimplemented logs/traces |
| `go vet ./...` | PASS on Windows plus Linux-target analysis |
| Linux AMD64 / ARM64 all-package and CLI builds | PASS, CGO disabled |
| Windows AMD64 all-package and CLI build | PASS; actual Windows collection remains unsupported |
| Darwin ARM64 compile safety | PASS; no native macOS runtime validation |
| `gofmt`, module verification and tidy | PASS |
| Real EC2 IMDS + production Observe HTTPS ingestion + database persistence | NOT RUN; no deployment or real key used |
| Legacy YAML import / DEB/RPM upgrade / durable registry migration | NOT IMPLEMENTED; design documented |

The full native startup test originally skipped because Docker Desktop's VM has no usable machine-id. It now uses the existing detector interface as a **private test seam**, while reading real Linux counters and using a certificate-pinned local TLS server. This does not fabricate a production identity or claim live EC2 verification. No `/etc/machine-id` or deployed config was created/modified.

The observed filesystem issue was `filesystem_cardinality`: the VM exposes more local mounts than the configured 64-mount cap. This is bounded-collection behavior, not an invented zero value or silent failure.

## Backend validation boundary

`tests/contract/verify-backend.mjs` directly imports the current platform's `normalizeOtlpMetricRequest`. It verifies accepted values, `app.system_disk_io` stable metric ID, byte unit, Agent classification and exact account/region/EC2 identity.

This is a real-code **normalization contract test**, not an authenticated API or persistence test. PostgreSQL entity reuse, ClickHouse deduplication, installation activity and frontend display require live testing with an approved endpoint/key. The test never imports a database service or calls an API.

The existing platform tsx installation lacked its esbuild dependency. Rather than modifying that read-only repository, the Agent test uses a small Node 24 built-in TypeScript stripping/resolution loader. Node reports the API's experimental warning; it is test tooling only.

## Reproduction commands

Windows, from the Agent repository:

```powershell
$env:GOTOOLCHAIN = 'local'
go test -count=1 ./...
go vet ./...
$env:AGENT_CONTRACT_OUTPUT = "$PWD/dist/backend-metrics-fixture.json"
go test -count=1 ./internal/pipeline
node --import ./tests/contract/backend-loader.mjs tests/contract/verify-backend.mjs D:/ob-cs-repo/ob-second-push/Observability dist/backend-metrics-fixture.json
```

The contract fixture is test data written only under `dist`; it is never sent to Observe. The script requires Node 24 and read access to the referenced platform normalizers.

Linux build/test compilation from Windows:

```powershell
$env:GOOS = 'linux'
$env:GOARCH = 'amd64'
$env:CGO_ENABLED = '0'
go build ./...
go test -c -o dist/linux-tests/collectors.test ./internal/collectors
wsl -d docker-desktop --cd /mnt/host/d/ob-cs-repo/Updated-Agent-v1/internal/collectors --exec /mnt/host/d/ob-cs-repo/Updated-Agent-v1/dist/linux-tests/collectors.test '-test.v' '-test.timeout=60s'
# Repeat test compilation/execution for every test-bearing package.
# Build linux/arm64 and windows/amd64 separately; these are not native ARM64 tests.
```

Native Linux CI can use ordinary `go test ./...` and `go vet ./...`. Local Windows compilation plus native Linux execution was used because Docker Desktop's minimal WSL distribution has no Go toolchain.

Generated caches remain in the target's ignored `dist` directory. An ignored `dist/go.mod` module boundary prevents `go test ./...` from scanning old dependencies without their own go.mod. This fixed a local cache traversal error; no application validation/rate-limit behavior was weakened.

## Files added

```text
go.sum
internal/app/runtime.go
internal/app/runtime_test.go
internal/cloud/ec2.go
internal/cloud/ec2_test.go
internal/collectors/metrics.go
internal/collectors/metrics_test.go
internal/collectors/metrics_linux_test.go
internal/config/runtime.go
internal/exporter/sender.go
internal/exporter/sender_test.go
internal/pipeline/metrics.go
internal/pipeline/metrics_test.go
internal/platform/ec2_linux.go
internal/platform/ec2_other.go
internal/platform/metrics_linux.go
internal/platform/metrics_other.go
internal/platform/metrics_linux_test.go
internal/queue/memory.go
internal/queue/memory_test.go
internal/selftelemetry/metrics.go
tests/contract/backend-loader.mjs
tests/contract/verify-backend.mjs
docs/LINUX_METRICS.md
docs/CONFIG_COMPATIBILITY.md
docs/METRICS_VALIDATION.md
```

Existing files updated:

- `go.mod`: pinned dependencies.
- `cmd/observe-agent/main.go`: explicit metrics `--run`, preserved no-I/O `--check`.
- `configs/agent.json`, `internal/config/config.go`: centralized collection/metadata/delivery options.
- `internal/platform/platform.go`: boot-time origin in snapshot model.
- `internal/platform/native_linux.go`: reject invalid/uninitialized machine identity.
- `README.md` and foundation architecture/security/backend/upgrade/validation/manifest docs: clear current-phase links while retaining earlier history.
- Ignored `dist/go.mod`: local development cache boundary; compiled binaries/fixtures/caches are not release artifacts.

## Remaining blockers before replacing the deployed Agent

1. Operator-approved live EC2 validation: real IMDSv2 -> current Observe HTTPS -> same canonical EC2 UUID -> persisted metrics/UI. Confirm CloudWatch provenance remains independent and no duplicate host/service appears.
2. Implement the legacy YAML importer and preserve protected env loading, prior collection selection, existing logs/traces and checkpoints. Do not replace a multi-signal Agent with this metrics-only build.
3. Implement/test service packaging, signed CI releases, upgrade/rollback and state compatibility. No installer or deployed service change exists here.
4. Agree the metrics memory-queue loss policy or add a durable spool. Current shutdown, process crash, queue overflow and exhausted retries can drop metrics; all are not exactly-once delivery.
5. Complete long-running fault/soak validation: kernel-blocked filesystem calls, memory/RSS budget enforcement, restricted service account coverage, network partitions, real 429 behavior and acknowledgement-loss deduplication.
6. On EC2 profiles set metadata required to avoid a machine-ID fallback where both IMDS and EC2 hardware hints are unavailable. The current detector is not cryptographic instance attestation.

See [exact runtime reads/permissions/network behavior](LINUX_METRICS.md) and [legacy configuration design](CONFIG_COMPATIBILITY.md).
<!-- AGENTV1 FILE END -->
