<!-- AGENTV1 FILE START: release layout, platform gates and local verification -->
# Release/distribution layout

This is a **canary distribution foundation**, not production certification. No tag, commit, push, release or remote asset was created during this work. Runtime, identity, API-key handling, YAML, queue, DEB maintainer scripts and installed paths are unchanged. Existing generated artifacts were retained, not moved/deleted.

## Repository tree

```text
cmd/observe-agent/                 existing portable entry point
internal/                         existing runtime, unchanged
configs/                          existing normalized JSON examples
install.sh                        self-contained root GitHub bootstrap
installers/
  install.sh                      local-checkout wrapper to ../install.sh
  install.ps1                     explicit unsupported Windows boundary
  README.md
scripts/
  build-release.sh                 Linux Go + dpkg-deb builder
  build-release.ps1                Windows Go + isolated Docker DEB builder
  checksums.sh                     Linux SHA256SUMS generator
  checksums.ps1                    Windows UTF-8/LF manifest generator
packaging/
  deb/                            validated builder/template/unit/hooks/tests, unchanged
  rpm/README.md                    deferred
  windows/README.md                deferred
  macos/README.md                  deferred
.github/workflows/
  ci.yml                          reusable native/compile/package validation
  release.yml                     version-tag -> validated canary release
.gitattributes                    LF for Linux scripts/maintainer hooks
tests/release/
  installers.sh                    tag/platform/integrity/entry-point tests
  windows.ps1                     syntax, manifest and fail-closed tests
  bootstrap.sh                    full installer with local download fixtures
  upgrade.sh                      older-to-newer DEB lifecycle test
docs/
  RELEASE_DISTRIBUTION.md
  DEB_CANARY.md                    existing installation/security/EC2 procedure
dist/                             generated; /dist/ remains gitignored
  bin/<tag>/{linux_amd64,linux_arm64,windows_amd64}/
  packages/<tag>/
  checksums/<tag>/SHA256SUMS
```

No production files were moved. Additive boundaries avoid breaking current imports or the validated DEB builder. `dist/deb`, `dist/deb-bin`, earlier canary outputs and local test/cache directories are historical generated files, not sources; new release builds use the three normalized output areas above.

## Naming and version mapping

For tag `v0.1.0-canary.20260903.2`:

- Public asset version and embedded binary version: `0.1.0-canary.20260903.2`.
- Debian control Version: `0.1.0~canary.20260903.2`. Tilde sorts before the final `0.1.0`; using a hyphen as Debian revision would incorrectly make a prerelease sort after a final release.
- Existing builder still creates its original internal Debian filename. The release wrapper renames **only the generated archive**, not package contents, to the public asset name.
- Tag grammar: `vMAJOR.MINOR.PATCH` optionally followed by `-canary.ALPHANUMERIC.DOTS`. Other formats fail before building/downloading.

| Release asset contract | Status |
|---|---|
| `observe-agent_<version>_amd64.deb` | Built and locally validated |
| `observe-agent_linux_amd64.deb` | Identical convenience alias for stable latest URLs; included in checksums |
| `SHA256SUMS` | Built; relative filenames, lowercase hashes, UTF-8 without BOM, LF |
| `observe-agent_<version>_linux_amd64.tar.gz` | Planned, not emitted |
| `observe-agent_<version>_linux_arm64.tar.gz` | Planned, not emitted |
| `observe-agent_<version>_arm64.deb` | Planned; ARM64 binary compile-check only |
| `observe-agent-<version>.x86_64.rpm` | Deferred RPM distro/SELinux/package validation |
| `observe-agent-<version>.aarch64.rpm` | Deferred |
| `observe-agent_<version>_windows_amd64.zip` | Deferred runtime/service/security validation |
| `observe-agent_<version>_windows_amd64.msi` | Deferred; no placeholder MSI |

macOS ARM64 remains a compile check and future launchd/tar.gz/PKG boundary. No platform is marked installable merely because its executable cross-compiles. Compile-only CI artifacts are clearly named `compile-preview-not-for-installation-*`; they are not uploaded as customer GitHub Release assets.

## Local builds (no publication)

Linux requires Go 1.26.7, Bash, GNU coreutils and dpkg-deb. From the repository:

```sh
go test ./...
go vet ./...
bash tests/release/installers.sh
bash scripts/build-release.sh v0.1.0-canary.20260903.2
cd dist/packages/v0.1.0-canary.20260903.2
sha256sum --check SHA256SUMS
```

Windows requires Go on PATH and Docker Desktop with Linux containers. The default builder image is `node:22-bookworm` (dpkg-deb is available; Node is not used for package assembly). Use a locally approved builder image override if needed:

```powershell
./tests/release/windows.ps1
./scripts/build-release.ps1 -Tag v0.1.0-canary.20260903.2
Get-Content ./dist/checksums/v0.1.0-canary.20260903.2/SHA256SUMS
```

The PowerShell builder performs native cross-compilation, mounts the repo read-only into an ephemeral network-disabled Linux builder, and mounts only the version's package output writable. It never starts an installed Agent or touches an existing Docker stack. No script invokes gh, Git, commits or uploads. Both builders refuse pre-existing output directories to prevent stale assets mixing into a release. Review partial outputs before retrying; no automatic recursive cleanup of existing dist data occurs.

The DEB is still a metrics-only canary with disabled/stopped service on install, restricted user, protected YAML, private durable state and preserved config/spool on remove/reinstall/upgrade. See [DEB installation/security guide](DEB_CANARY.md) and the unchanged `packaging/deb/observe-agent.service`.

## Linux bootstrap

The requested future entry point is self-contained, so it does not download and execute another unverified installer:

```sh
curl -fsSL https://raw.githubusercontent.com/vinod-cs/observe-agent/main/install.sh | sudo bash
```

**Not usable until this source and a suitable release are intentionally published.** Nothing was uploaded here. For canaries, select a published tag explicitly:

```sh
curl -fsSL https://raw.githubusercontent.com/vinod-cs/observe-agent/main/install.sh | \
  sudo bash -s -- --version v0.1.0-canary.20260903.2
```

Safer operator practice: download a **tag-pinned installer** to a file, review it, then execute with sudo. A mutable `main` script piped into a root shell is a repository-trust boundary; a package checksum does not authenticate that script independently.

Installer behavior:

1. Read Linux distro ID without executing os-release. Accept Debian/Ubuntu and x86_64/amd64 only. Reject ARM64, RPM distributions, Windows/macOS and unknown architectures before downloading.
2. Validate tag. Without a tag, resolve GitHub's latest release redirect, validate its repository/path/tag, then pin **both** asset and checksum downloads to that tag. This prevents a moving-latest race.
3. Require curl, sha256sum, dpkg tools, root and running systemd. No automatic dependency install or configuration/credential collection.
4. Refuse downgrades. Download HTTPS-only with TLS verification, bounded time/retries/size into a private temporary directory.
5. Require exactly one valid matching manifest entry, verify SHA256, and verify DEB Package/Architecture/Version. Untrusted manifest paths are never passed to a file-checking command.
6. Install with `dpkg --force-confdef --force-confold -i`, preserving local conffiles. No auto-start, boot enablement or credential changes. Existing queue/install behavior stays in the unchanged DEB hooks.
7. Print protected editor, offline check, systemctl and journalctl commands. Clean up only the installer-owned temporary directory.

`--dry-run --version <tag>` prints the plan without download/installation. No-argument latest resolution may perform a read-only HTTPS request even with dry-run. A 404/no stable release fails clearly and requests an explicit canary tag. No fallback package is selected.

`installers/install.ps1` currently throws an explicit unsupported error before any download, credential access, service creation or filesystem mutation. MSI/ZIP installation will not be enabled until Windows runtime support is real.

### Predictable URLs

```text
https://github.com/vinod-cs/observe-agent/releases/download/<tag>/observe-agent_<version>_amd64.deb
https://github.com/vinod-cs/observe-agent/releases/download/<tag>/SHA256SUMS
https://github.com/vinod-cs/observe-agent/releases/latest/download/observe-agent_linux_amd64.deb
https://github.com/vinod-cs/observe-agent/releases/latest/download/SHA256SUMS
```

A versioned filename cannot remain a single stable latest URL across versions; the identical alias solves that. Canary releases are prereleases and deliberately not latest. Stable/latest remains unavailable until an operator explicitly approves a stable release policy. See [GitHub release-link documentation](https://docs.github.com/en/repositories/releasing-projects-on-github/linking-to-releases).

SHA256 detects corruption or a mismatch with the downloaded manifest; it is **not a signature** against a compromised publisher/repository. Independent package signing/attestation, action/image digest pinning and protected tags are further production hardening gates.

## GitHub Actions

`ci.yml` runs on push/PR/manual invocation and is reusable by `release.yml`:

1. Native Linux and Windows tests/vet; gofmt; legacy JSON check; installer/PowerShell tests.
2. Linux AMD64/ARM64, Windows AMD64 and macOS ARM64 compile checks.
3. Build the supported DEB/assets and SHA256SUMS on an ephemeral **GitHub-hosted** Ubuntu runner.
4. Validate real systemd package lifecycle, fixture-only complete bootstrap, and older-package-version upgrade semantics. Never use these destructive-to-runner tests on self-hosted/customer hosts.
5. Upload validated package assets within that workflow run, separately from compile previews.

`release.yml` responds to version tags only in `vinod-cs/observe-agent`, invokes all validation, and serializes publication per tag. The publish job downloads only its own validated run artifact, rechecks hashes, creates a **draft prerelease**, uploads the allowlisted assets, then exposes it as a prerelease with latest disabled. Existing releases/assets are not overwritten; a failed partial draft requires operator review before retry. This is intentionally not an automatic production-release path.

Only publish has `contents: write`; validation has read-only access. Authentication uses `${{ github.token }}` supplied as `GH_TOKEN`, never a hardcoded token/PAT. The publish job uses the `canary-release` environment: **configure required reviewers before enabling tag pushes**; environment naming alone does not enforce approval. Protect main/version tags and restrict who may publish. [GitHub token permissions](https://docs.github.com/en/actions/tutorials/authenticate-with-github_token).

No workflow was dispatched remotely. Local workflow validation cannot prove hosted-runner execution or remote release permissions; the first approved tag remains an integration gate.

## Local verification evidence

- Existing Windows Go tests/vet: PASS.
- Native Linux `go test -count=1 ./...` and `go vet ./...`: PASS in the official Go 1.26.7 Bookworm build container, repository read-only and only generated dist writable.
- PowerShell parser, checksum encoding, unsupported Windows installer: PASS.
- Linux installer platform/tag tests, checksum tamper/duplicate/path rejection and local wrapper: PASS.
- New Windows release builder: PASS; Linux AMD64/ARM64 and Windows AMD64 outputs plus DEB/alias/manifests generated under tag `v0.1.0-canary.20260903.2`.
- New Bash release builder: PASS end-to-end under `v0.1.0-canary.20260903.3`; all three binaries compiled and both package names passed SHA256SUMS verification. This additional build checks the Linux tooling; the `.2` artifact is the one exercised through the full systemd/upgrade suite below.
- DEB SHA256 for that tag: `c5caea7840653998ff1539f25a238eb52df94c8d90a430f4647b67f22b04aaf9` (both names are the same bytes).
- Existing real systemd lifecycle suite: PASS using that generated DEB; 3 initial accepted batches/364 real OS metric points; final 13 accepted batches/1576 points including replay, one host identity.
- Inline/env/file YAML authentication, private permissions, no journal key leakage, disabled-on-install, stop/restart/status, remove/reinstall, queue and conffile retention: PASS.
- Complete bootstrap: PASS with local fixture downloads; config retained on repeat install; tampered bytes and correctly checksummed wrong-version DEBs rejected. No GitHub/Observe contact.
- True `.1 -> .2` DEB upgrade: PASS with the service running beforehand; same service UID, conffile SHA and retained record checksums afterward; stopped with no automatic restart.
- Packaged YAML, systemd unit and all three lifecycle hooks are byte-for-byte identical to the earlier DEB.
- Bash/PowerShell checksum output parity and all new shell syntax checks: PASS. Changed source text has no trailing whitespace. This checkout has no `.git` metadata, so no Git diff/commit was performed or initialized.
- `actionlint` v1.7.7: local workflow syntax/expression validation; shell syntax checked separately. Remote Actions/release publication NOT RUN.

Local package tests used a dedicated network-disabled Debian/systemd container with no host mounts, copied fixture files and test-only credentials. Docker cgroup-v1 compatibility warnings remain a known local harness limitation, not a passed EC2 sandbox gate. The downloader stub lives under /opt because the test container mounts /tmp noexec. No production installer behavior was relaxed to accommodate that.

All identity/correlation and live EC2 acceptance gates in [LIVE_EC2_VALIDATION.md](LIVE_EC2_VALIDATION.md) remain unchanged and unclaimed. No packaging refactor substitutes for real EC2/backend validation.
<!-- AGENTV1 FILE END -->
