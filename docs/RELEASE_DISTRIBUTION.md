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

The PowerShell builder performs native cross-compilation, mounts the repo read-only into an ephemeral network-disabled Linux builder, and mounts only the version's package output writable. It never starts an installed Agent or touches an existing Docker stack. Neither build script invokes gh, Git, commits or uploads; the separate Actions-only publisher is described below. Both builders refuse pre-existing output directories to prevent stale assets mixing into a release. Review partial outputs before retrying; no automatic recursive cleanup of existing dist data occurs.

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

`release.yml` responds to canary tags only (`v*-canary.*`) in `vinod-cs/observe-agent`, invokes all validation, and serializes publication per tag. The publish job checks out the same tagged SHA with persisted credentials disabled, downloads only its own validated run artifact, and invokes `scripts/publish-release.py`. This helper is Actions/tag guarded and uses only the workflow-provided GitHub token via `gh`. Build scripts remain offline/local and never invoke publication.

The publisher validates exactly the versioned AMD64 DEB, identical `observe-agent_linux_amd64.deb` alias and `SHA256SUMS`; missing/extra/wrong names, unsafe files, duplicate/traversal manifest entries or hash mismatches fail before any release mutation. It creates a **draft prerelease** with `--verify-tag` (never creates a tag), uploads only that allowlist, reads back and verifies the bytes, then explicitly clears draft with prerelease/latest=false. Final success requires a non-draft prerelease at the expected repository/tag URL with all three verified assets. The Actions job summary links the actual Release and downloads. GitHub-generated source archives may additionally appear in the Release UI; the workflow uploads only the three requested files.

Safe reruns: an identical published prerelease is verified and left untouched. A draft created by this publisher (identified by its notes marker) resumes only missing uploads after verifying all existing bytes. Changed bytes, unrelated/duplicate assets, unfinished upload state, a stable release or an unrecognized older/manual draft fail with explicit instructions; no deletion or `--clobber` is used. A rebuilt DEB can differ due to build timestamps: rerun the failed publication job against the original validated run artifact, rather than overwriting a release with rebuilt bytes. If originals are unavailable or differ, use a new approved tag. An older `.5` draft without the ownership marker requires operator review; this implementation does not silently adopt it.

The locally available `v0.1.0-canary.20260903.5` workflow already included `gh release create/upload/edit`. Therefore local inspection alone does **not** prove why the reported hosted run had no visible Release. Check that run's actual publish-step outcome, draft state, repository visibility and Release URL; an environment deployment badge is not evidence of a Release. This fix adds missing postcondition checks and rerun handling, not a claim that the old workflow lacked a create command. Existing tag reruns use their original workflow revision; a new tag must contain the updated files.

For `v0.1.0-canary.20260903.6`, expected URLs (not created during local validation):

```text
https://github.com/vinod-cs/observe-agent/releases/tag/v0.1.0-canary.20260903.6
https://github.com/vinod-cs/observe-agent/releases/download/v0.1.0-canary.20260903.6/observe-agent_0.1.0-canary.20260903.6_amd64.deb
https://github.com/vinod-cs/observe-agent/releases/download/v0.1.0-canary.20260903.6/observe-agent_linux_amd64.deb
https://github.com/vinod-cs/observe-agent/releases/download/v0.1.0-canary.20260903.6/SHA256SUMS
```

Canaries remain **Pre-release**, never stable/Latest; `install.sh` must receive `--version <canary-tag>` when only canaries exist. Environment `canary-release` remains an approval/protection gate; it does not create a release by itself. No environment deployment to customer machines is performed by this workflow.

Offline publication tests: `python3 -B tests/release/publication_test.py` (32 cases, fake GitHub CLI only). They exercise create/prerelease, exact asset integrity, interrupted/resumed draft, identical rerun, conflicts, permission/API errors, final draft detection, summary links and local invocation refusal. CI runs them in the Linux native job before packaging/publication.

### Hosted .6 failure: draft and prerelease classification

The exact reported message came from this pre-upload predicate:

```python
release is not None and release.get("prerelease") is True
```

It ran **before** draft/ownership classification, conflating a missing lookup result
with a non-prerelease draft or a published stable release. An owned draft with
`draft=true, prerelease=false` could never reach the explicit publication edit.
The old fake CLI always returned an immediately visible draft with prerelease=true,
so it missed this case. A second weakness was exact `tag_name` filtering of REST
release-list data rather than resolving a pending draft tag.

Inspection of the local `.6` tag confirms create already passed `--draft` and
`--prerelease`, and edit passed `--draft=false --prerelease --latest=false`. No other
workflow command created a stable release first. The old REST `draft`/`prerelease`
field names were correct for `gh api`; it did not use `gh release view` at all.
There is no code evidence that edit lost the flag: the reported guard executes
before edit. A read-only unauthenticated lookup of `.6` returned HTTP 404; that
cannot reveal private/draft state. Without authenticated hosted response/log data,
we cannot claim whether that run saw None or prerelease=false. The code defect and
the two possible error branches are proven; the exact remote state is not.

Current state machine:

1. Resolve the exact pending/published tag through GraphQL `release(tagName: ...)`.
   Repository/query errors fail closed; only an explicit null means absent.
2. Read `gh release view <tag> --repo vinod-cs/observe-agent --json
   databaseId,apiUrl,isDraft,isPrerelease,tagName,body,assets,url`. Verify the numeric
   ID against the pending-tag result and API URL against the expected repository.
   Strictly map CLI camelCase boolean fields once; missing/string flags fail.
3. Absent → create draft with explicit --prerelease. A bounded post-create lookup
   retries absence twice (1s, 2s), never creates again on that attempt. Missing,
   malformed and stable states now have distinct diagnostics.
4. Owned draft → permit either prerelease boolean, verify existing bytes, upload
   missing expected assets, recheck ownership/ID/draft state, then publish with
   `--draft=false --prerelease --latest=false` in one edit. This is not conversion
   of a published stable release.
5. Published stable → fail without upload/edit, even when its notes contain our
   ownership marker. Unowned draft → fail. Identical published prerelease → no-op.
6. Final result must have the same numeric ID, exact triggering tagName,
   isDraft=false, isPrerelease=true, expected Release URL, exactly three assets,
   and identical remote checksums/bytes. Wrong final flags fail rather than trying
   to repair an unexpectedly published stable release.

Drafts can have temporary `untagged-*` REST/UI names: binding uses the requested
pending tag's numeric ID, never a fuzzy name. Final published tag must match exactly.
GitHub CLI itself uses pending-tag resolution for drafts; see its
[release fetch implementation](https://github.com/cli/cli/blob/trunk/pkg/cmd/release/shared/fetch.go)
and [documented JSON fields](https://cli.github.com/manual/gh_release_view).

The publisher logs only phase, requested tag, numeric release ID and boolean state;
the workflow also prints gh version. It does not print tokens or release bodies.
New tests reproduce the old guard failure, pending-tag lookup, false-prerelease
owned draft/resume, accidental published-stable creation, final edit losing its flag,
state changing to stable during upload, missing-result retries, wrong tag/repo/ID,
and malformed CLI flag types. All 32 tests pass on Windows and offline Linux;
actionlint, Linux installer regressions and git diff --check pass. No authenticated
GitHub mutation or hosted publication was performed. A new approved tag such as
`.7` must contain the fix; no tag was created locally.

Local publication-fix validation: all 20 tests passed on Windows Python 3.14 and Linux Python 3.11 (network-disabled container); cached actionlint 1.7.7 passed both workflows; Linux installer-contract regressions and `git diff --check` passed. No real gh publication/API request, tag creation, release, push or deployment was performed. Hosted execution with the repository's environment protections remains to be verified after an explicitly approved tag.

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
