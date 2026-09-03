<!-- AGENTV1 FILE START: local verification evidence; no live deployment claims -->
# Queue scope v2 — validation, 2026-09-03

Completed locally on Windows and isolated Linux, final verification approximately
11:17 UTC. No commit, publish, deployment, backend modification or reference-repo
modification was performed. See [design and operator procedure](QUEUE_SCOPE_V2.md).

## Results

| Gate | Result / evidence |
|---|---|
| Windows `go test ./...`, `go vet ./...` | PASS; Go 1.26.7; OS-specific Linux tests are build-tagged |
| Native Linux `go test -count=1 ./...`, `go vet ./...` | PASS; isolated golang:1.26.7-bookworm, network disabled |
| Queue/runtime repeat `go test -count=5 ./internal/queue ./internal/collectors` | PASS; includes fault/retry/identity/replay tests |
| Endpoint change, same backend/org/host | PASS; actual TLS endpoint A then endpoint B, retained backlog accepted |
| Key-only rotation after endpoint change | PASS; same v2 queue, replacement ApiKey observed by local fixture |
| Backend/org/host/account/region changes | PASS: v2 fails closed; v1 host/account/region mismatch also fails before migration |
| Correct/incorrect v1 previous endpoint | PASS: exact hash required; record bytes and receipt unchanged |
| Crash recovery | PASS: child exits abruptly at six staging/commit checkpoints; resume/restart works |
| Partial/ambiguous state | PASS: partial stage and existing `.pending` retained; changed org after journal commit refused |
| Package upgrade with pending v1 queue | PASS: actual older local .2 binary to .4; two original .rec checksums unchanged after install and migration, then replay/ack removes those records only after TLS acceptance |
| Restart without previous_endpoint | PASS in unit/runtime and installed package tests |
| Current-version package upgrade fixture | PASS; v2→v2 transport change, config/UID/state preservation |
| DEB lifecycle | PASS: disabled install; invalid defaults fail start; unprivileged service start/stop/restart/status; inline/env/file credentials; no fixture keys in journal; remove/reinstall preserves config/UID/backlog; purge retains state/account |
| Build matrix | PASS: Linux AMD64, Linux ARM64, Windows AMD64; latter two compile-only |
| Existing release tooling | PASS: PowerShell parser/checksums/fail-closed Windows stub, Linux installer guards/integrity, full offline bootstrap with checksum tamper rejection and no autostart |
| Formatting/shell | PASS: gofmt clean, bash syntax checks, changed-file trailing-whitespace/conflict-marker scan clean |
| Git diff | Not applicable: this working directory has no .git metadata; no Git repository was initialized |
| Live EC2 / Observe database | NOT RUN; no real credentials, backend traffic, canonical-UUID or deduplication claims |

Two disposable offline systemd containers exercised actual package hooks and the
restricted account. They emitted Docker compat-cgroup warnings but service commands
worked. This does not replace real Ubuntu/Debian host resource-control or EC2 tests.
The initial replay fixture waited for any delivered batch; it was corrected to wait
for the retained target batch. An initial package-fixture invocation used the wrong
container working directory; correcting the invocation made it pass. No product
policy/authentication was weakened to make tests pass.

## Generated local artifact (not published)

- Tag/build: `v0.1.0-canary.20260903.4` (embedded version without leading v).
- DEB: `dist/packages/v0.1.0-canary.20260903.4/observe-agent_0.1.0-canary.20260903.4_amd64.deb`.
- DEB SHA256: `6418bd6dce8e2de7b584cc1ae83d954ba1b7e6366206dc7493b537e6cae28582`.
- Linux AMD64 binary SHA256: `df35ae41db8694380dd6f12ad6e1de1a1926d7dcc2280b3b6a620b985cee3a0c`.
- Built with the existing `scripts/build-release.ps1`; all outputs remain ignored
  under dist. No release workflow, installer service path or packaging lifecycle
  implementation was changed.

## Files changed in this phase

New:

- `internal/queue/scope.go`
- `internal/queue/migration_linux.go`
- `internal/queue/scope_linux_test.go`
- `internal/config/scope.go`
- `internal/config/scope_test.go`
- `internal/collectors/scope_linux_test.go`
- `docs/QUEUE_SCOPE_V2.md`
- `docs/QUEUE_SCOPE_V2_VALIDATION.md`

Updated:

- `internal/queue/disk_linux.go`, `internal/queue/disk_other.go`
- `internal/config/config.go`, `internal/config/yaml.go`, `internal/config/yaml_test.go`
- `internal/collectors/metrics.go`, `internal/collectors/metrics_linux_test.go`
- `configs/agent.json`
- `packaging/deb/agent.yaml`, `packaging/deb/README.md`, `packaging/deb/validate.sh`
- `tests/release/upgrade.sh`
- `README.md`
- `docs/DEB_CANARY.md`, `docs/DELIVERY_RELIABILITY.md`, `docs/CONFIG_COMPATIBILITY.md`,
  `docs/LIVE_EC2_VALIDATION.md`

## Compatibility boundaries

One-time logical IDs are required; the original v1 combined hash cannot establish
the old tenant. Operator confirmation is essential. The configured organization
is not verified against the opaque API key by a new handshake. Wrong-tenant keys
or unrelated URLs deliberately mislabeled with the same IDs cannot be detected
locally. Existing backend authorization still applies.

At-least-once delivery remains unchanged; acknowledgement-loss duplication is
possible. Process-exit tests are not hardware power-loss tests. Ambiguous interrupted
files fail closed with preserved evidence; they are not silently discarded. A v1
binary cannot downgrade/open migrated v2 state. These are deliberate safety limits,
not reasons to erase a pending queue.
<!-- AGENTV1 FILE END -->
