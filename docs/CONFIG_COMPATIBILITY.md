<!-- AGENTV1 FILE START: non-destructive legacy configuration importer design; no deployed edits -->
# Existing agent.yaml / env compatibility design

> The current Observe YAML/JSON runtime now requires backend_id and organization_id for metrics check/start. This is separate from the historical agent-i importer design below. Existing Observe v1 spools use the non-destructive [queue scope v2 migration](QUEUE_SCOPE_V2.md); do not rewrite deployed agent-i files.

The deployed `/etc/agent-i/agent.yaml` and `/etc/agent-i/env` remain unchanged. This phase implements the new JSON runtime, **not** a production YAML importer. Packaging must not require users to rewrite existing files.

## Proposed importer contract

1. Detect the legacy YAML format at the supplied existing config path; parse with a strict, pinned YAML parser that rejects duplicate/unknown keys and bounded input. No string interpolation or execution.
2. Convert into an in-memory current config; preserve the original file. A dry-run migration report contains field names and compatibility outcomes only, never secrets.
3. Read credential **references** from `headers_env`. The OS service continues loading the same protected `/etc/agent-i/env`; the importer never reads/copies its secret contents or puts them into JSON.
4. Preserve empty agent_id, explicit label precedence, exact EC2 host.id, existing API base and resource semantics. Validate output against new strict schema and the actual backend contract.
5. Fail clearly on unsupported enabled capabilities, unsupported headers or behavior-changing values. Never silently disable existing logs/traces or clamp collection/retry values and call that compatible.
6. Only an explicitly approved future migration command may write a new sidecar config/backup; runtime loading should work directly from legacy YAML so users need not edit files.

| Legacy setting | Proposed mapping | Compatibility rule |
|---|---|---|
| `agent_id` | `agent_id` | Empty remains automatic; no hostname/random/key fallback |
| `interval: 15s` | `collection.interval_seconds: 15` | Reject unsupported precision/range; do not silently round |
| `ec2_metadata.enabled/timeout` | `ec2_metadata.enabled/timeout_seconds` | Preserve timeout semantics; release EC2 profile should require identity |
| `metrics.enabled` | `policy.enabled.metrics` | Preserve explicit state |
| `metrics.collect` | Future local collector selection schema | Must implement equivalent include list; current slice collects the defined six metric families |
| `logs.enabled`, `traces.enabled` | Capability flags | Enabled legacy signals currently block replacement, not silently drop |
| `exporter.type: otlp_http` | Same type | Preserve OTLP JSON model and actual endpoint suffixes |
| `exporter.endpoint` | HTTPS Observe OTLP base | Plain HTTP legacy endpoints need explicit operator transition; do not downgrade TLS |
| `exporter.headers_env.Authorization` | Same environment variable reference | Value remains full `ApiKey <key>` in protected environment |
| Static exporter headers | Future reviewed allowlist | Current schema accepts only environment-sourced Authorization; do not silently discard required proxy headers |
| Batch/retry/queue/registry options | Versioned equivalents | Existing durable/checkpoint behavior must be implemented or rejected as unsupported; memory-only queue is not equivalent |

## Required tests before release

Use sanitized fixtures from the existing repository to prove exact output for defaults, empty/explicit identity, enabled/disabled signals, environment references, durations, batch/retry limits and unsupported configuration. Test duplicate YAML keys, aliases/oversize input, secret redaction and no modifications to source config.

Upgrade tests must preserve the old service identity, environment-file path/permissions, canonical host.id, file-log registry/checkpoints and backend installation identity. Validate rollback from the new binary without deleting or rewriting existing state. No DEB/RPM/MSI, service migration, updater or deployment is implemented in this phase.
<!-- AGENTV1 FILE END -->
