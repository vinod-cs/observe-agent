<!-- AGENTV1 FILE START: release prerequisites and non-destructive migration plan -->
# Upgrade and implementation plan

> The Linux metrics vertical slice is implemented and locally tested; see [current runtime](LINUX_METRICS.md). The packaging, compatibility-import and live rollout gates below still apply.

## Never deploy this foundation over the working Linux Agent

It has no collection daemon and uses a new strict JSON foundation format. Existing `agent.yaml`, environment secret references, file registries, checkpoints, Agent ID/host.id and service must remain untouched. No package, installer, remote updater or release has been created in this task.

## Prerequisites before packaging

1. Pin supported OTel components, add adapter-backed Linux collection and preserve golden metric/log/trace resource semantics and units.
2. Implement and test IMDSv2 identity with bounds/proxy isolation; keep explicit Agent label and exact host identity separate; retain machine-id fallback and startup failure behavior.
3. Add legacy YAML import/validation with strict unknown-field behavior and no secret echo; migrate only with explicit opt-in and retain original config. Reuse current safe batch/retry/queue defaults after inspecting their behavior, not merely their names.
4. Implement bounded queue, checkpoints, memory limiter, sender retries/partial acceptance and production self-diagnostics. Version state formats; no destructive conversion without rollback path.
5. Connect OS service lifecycles: systemd signals, Windows SCM controls and later launchd. Validate permissions under a restricted service user and real stop/start conditions.
6. Implement protected OS-specific state and machine identity, native OS tests and actual backend Windows/macOS platform support before those rollouts.
7. Approve a backend remote-policy protocol, authenticated scope binding, verifier and startup LKG recovery. Invalid/expired policies leave current policy running; policy failure never executes arbitrary commands.

## Staged plan preserving deployed Linux

| Stage | Deliverable | Exit gate |
|---|---|---|
| Foundation (this task) | Portable boundaries, policy/identity/security tests and compile-safe platforms | Native current-host tests and target cross-builds; no deployment |
| Linux metrics vertical slice | Standard OTel host metrics, exact identity, HTTPS JSON sender, bounded pipeline | Native AMD64/ARM64 tests, least-privilege review, real backend acceptance without duplicate entities |
| Logs and application traces | Approved file reader and existing loopback OTLP contract | Rotation/checkpoint, signal enable/disable, service/infrastructure provenance and workload-no-modification tests |
| Policy and diagnostics | Authenticated delivery, LKG recovery and bounded audit/diagnostics | Cross-tenant/replay rejection, rollback/fault-injection, offline operation |
| Windows adapters | Machine ID, file identity, OS readers, state ACL and SCM | Native Windows tests and matching backend platform support |
| Packages and upgrade | CI-built signed artifacts and install/repair/uninstall | Upgrade/rollback preserving config, secrets, identities and all checkpoints |
| macOS | Native adapters and launchd | ARM64 hardware tests, signing/notarization and PKG validation |

Docker/Kubernetes/ECS/Lambda/config-management reuse the portable core later. These require distinct deployment/lifetime/read-permission adapters, not copied identity/exporter/authentication logic. None is implemented here.

## Future artifacts, CI-owned only

- Linux AMD64 and ARM64: tar.gz, DEB and RPM; systemd.
- Windows AMD64: ZIP and signed MSI; Windows Service.
- macOS ARM64 later: tar.gz and signed/notarized PKG; launchd.

The included CI definition validates Linux/Windows native tests and four cross-build targets. Preview artifacts are labeled **not for installation**. It does not publish a release, upload packages or deploy. Future release CI must pin actions by reviewed commit, lock Go/OTel dependencies, generate SBOM/provenance, sign artifacts and publish verified checksums. ARM64/Darwin cross-build is not native runtime validation; add native runners before support claims.

## Safe in-place upgrade protocol (future)

1. Verify signed package provenance, OS/architecture and state/config compatibility before stopping the old service.
2. Preserve service account/SID, config, secret references, canonical host.id, OS machine ID and Agent installation identity. Do not regenerate identity or silently replace API keys.
3. Quiesce the old process, drain within bounds and commit durable checkpoints. Enforce one writer to the registry/queue across versions.
4. Atomically switch versioned binaries; run compatible state migration with protected backup and explicit schema version. No telemetry purge.
5. Start and health-check the new process. Roll back binary and compatible state on failure. Never roll back checkpoints blindly if that would duplicate/delete acknowledged records.
6. DEB/RPM scripts preserve local config and environment; MSI upgrade/repair preserves protected ProgramData state and service identity. Uninstall preserves state by default; explicit purge is separate and guarded.

No automatic updater or unsafe manual SCP workflow is part of this design. Package-specific rollback compatibility is a release gate, not a promise of the current state primitive.
<!-- AGENTV1 FILE END -->
