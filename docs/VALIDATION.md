<!-- AGENTV1 FILE START: measured local validation, not deployment evidence -->
# Foundation validation

> Historical foundation results. See [current metrics-slice validation](METRICS_VALIDATION.md) for the new native Linux and exporter tests.

Validation date: 2026-09-02. Host: Windows AMD64. Go: `go1.26.7 windows/amd64`, `GOTOOLCHAIN=local`. Go caches and build outputs were directed into this repository's ignored `dist/` tree. No real API key, AWS call, backend mutation, installed Agent change or live telemetry was used.

## Results

| Check | Result |
|---|---|
| `gofmt -w` then `gofmt -l` on all Go files | PASS; zero unformatted files |
| `go test ./...` | PASS, native Windows |
| Final uncached `go test -count=1 -json ./...` | PASS; 22 top-level tests, 37 including subtests, 8 tested packages |
| Interface-only/boundary packages | 9 packages have no test files; not counted as passed test packages |
| `go vet ./...` | PASS, native Windows |
| `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build ./...` and CLI build | PASS, cross-build only |
| `GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build ./...` and CLI build | PASS, cross-build only |
| `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./...` and CLI build | PASS |
| `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...` and CLI build | PASS, placeholder build only |
| Linux platform tests `go test -c ... ./internal/platform` | PASS compilation; NOT native execution |
| Windows binary `--version` | PASS; `foundation-local` |
| Windows binary `--check --config configs/agent.json` | PASS; explicitly reports no collection/service runtime |
| Source trailing-whitespace scan | PASS; zero findings before final documentation handoff |
| GitHub Actions execution | NOT RUN; workflow supplied, no Git sync/commit |
| Native Linux, ARM64, macOS runtime tests | NOT RUN |
| Live backend/EC2/Agent verification | NOT RUN; out of foundation scope |

## Behaviors covered

- Empty Agent label accepted; explicit label wins without becoming a fabricated host ID.
- Trusted EC2 identity overrides machine fallback; different EC2 identities stay distinct.
- Unverified cloud evidence ignored; stable non-cloud identity; failure when all identity sources unavailable.
- Application service identity preserved separately from host identity.
- Unknown/duplicate/trailing/oversize config rejected; HTTPS endpoint and environment-reference constraints.
- Disabled collectors never constructed; unsupported capability preflight has no startup side effects.
- Enable/disable preserves unrelated collectors; cancellation, same-version idempotency, stale-version rejection, concurrent applies, start rollback and failed stop/degraded state.
- LKG round-trip; persistence failure cannot claim successful committed configuration.
- Remote verifier requirement, organization/installation binding, expiry, local opt-in and payload bound.
- Signal-specific request paths, JSON content type, redacted authorization failures, redirect rejection and no secret read for disabled/invalid requests.
- No fabricated zero telemetry from platform placeholders.

Linux private state directory/file identity tests are present and compile. Their real Linux permission/fsync behavior still requires native execution; cross-build is not evidence of that behavior. The code's authenticated-policy verifier is an interface; test doubles are not proof of a production cryptographic protocol.

## Reproduce locally (PowerShell)

```powershell
Set-Location 'D:\ob-cs-repo\Updated-Agent-v1'
$agentRoot = (Get-Location).Path
$env:GOCACHE = "$agentRoot\dist\go-cache"
$env:GOTMPDIR = "$agentRoot\dist\tmp"
$env:GOMODCACHE = "$agentRoot\dist\go-mod"
$env:GOTOOLCHAIN = 'local'
& 'C:\Program Files\Go\bin\go.exe' test -count=1 ./...
& 'C:\Program Files\Go\bin\go.exe' vet ./...
$env:CGO_ENABLED = '0'
$env:GOOS = 'linux'
$env:GOARCH = 'amd64' # repeat arm64; then windows/amd64 and darwin/arm64
& 'C:\Program Files\Go\bin\go.exe' build ./...
& 'C:\Program Files\Go\bin\go.exe' build -trimpath -ldflags '-X github.com/agent-i/agent/internal/version.Version=foundation-local' -o dist/bin/observe-agent_linux_amd64 ./cmd/observe-agent
Remove-Item Env:GOOS, Env:GOARCH # restore native target before native tests
```

For a new checkout, create the listed `dist` directories first. Never run a cross-target test executable and report it as a native test. `go test ./...` should run on Linux and Windows runners in the supplied CI workflow; production support additionally requires native ARM64/Windows service validation.

## Binary verification

CLI artifacts were checked by magic bytes: Linux ELF, Windows PE/MZ, Darwin Mach-O. No artifact is a production Agent or an approved release.

| Artifact | SHA256 |
|---|---|
| `observe-agent_linux_amd64` | `8C7C46105FF6C855009E9947897EBA306AA5A6D68E316223C33F3F952A36BA21` |
| `observe-agent_linux_arm64` | `8CDA1DA47F35AF3224B39EC16CB646E6EC4AAF1A50F184F12DD3DEB5B309C551` |
| `observe-agent_windows_amd64.exe` | `561A3AD438A45A7B90D3ABF6418629667C84799BDF2CDE6C3438CD8AE1C353FD` |
| `observe-agent_darwin_arm64` | `0E0DD06E529B51125F954A1D80B3AFEBA6C61C711EC24183A5CC934ECBD9F3EA` |

## Remaining gates

No real collector, IMDS detector, sender loop, durable queue, memory limiter, remote policy transport/verifier, startup LKG recovery, Windows SCM/machine identity, installer or updater is implemented. The JSON foundation is not a drop-in legacy YAML replacement. The inspected backend still has Linux-only installation platform handling. See [upgrades](UPGRADES.md) before any release.
<!-- AGENTV1 FILE END -->
