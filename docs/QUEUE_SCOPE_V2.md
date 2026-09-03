<!-- AGENTV1 FILE START: stable logical queue identity and non-destructive legacy migration -->
# Queue scope v2

The metrics spool is bound to a logical deployment, not a transport URL. Its scope
is SHA-256 of a domain-separated JSON tuple, in this exact order:
`backend_id, organization_id, canonical host.id, cloud.account.id, cloud.region`.
Endpoint, API key, key ID, Agent display name and hostname are excluded. EC2 host.id
still comes from verified IMDSv2; non-cloud identity uses the existing stable fallback.
No new identity rule or backend endpoint was introduced.

## Protected configuration

```yaml
observe:
  backend_id: observe-production
  organization_id: "<actual-Observe-organization-UUID>"
  endpoint: "https://<ingestion-host>/api/v1/otlp"
  api_key_file: /etc/observe-agent/api-key
collection:
  metrics:
    enabled: true
  logs:
    enabled: false
  traces:
    enabled: false
```

Use the actual organization identity and a stable administrator-assigned backend
identifier (not its DNS name). IDs allow 1–128 ASCII letters/digits plus `. _ : -`,
starting with a letter/digit. The example angle-bracket placeholders intentionally
do not validate. All hosts in one deployment use the same backend/org IDs.
Do not use a credential as either ID. JSON configurations use the equivalent
`exporter.backend_id`, `exporter.organization_id`, `exporter.previous_endpoint`.
Existing strict YAML/JSON parsing and credential precedence remain unchanged.

YAML stays root:observe-agent 0640; parent directory 0750. Secret-file/environment
reference modes and inline keys retain their existing protections. Queue files
remain service-owned 0600 inside the existing 0700 state directory.

**Trust boundary:** these IDs are an explicit, protected operator binding, not a
remote tenant attestation. An opaque API key does not reveal its organization.
Operators must verify that the configured ID and replacement key belong to the
same organization and that a replacement HTTPS endpoint is the same backend.
The Agent cannot detect a wrong-tenant key or unrelated endpoint falsely labelled
with the same IDs. No credential hash, key or URL is stored in queue metadata.

## One-time upgrade from a v1 spool

1. Stop `observe-agent` before changing its configuration. Upgrade the DEB normally,
   keeping the local conffile. Packaging never truncates/recreates the spool or
   automatically starts the service.
2. Set the confirmed `observe.backend_id` and `observe.organization_id`. V1 stored
   neither: the operator is explicitly binding that backlog to its original tenant.
3. If the endpoint has already changed, temporarily add:

   ```yaml
   observe:
     # alongside backend_id, organization_id, endpoint and credential settings
     previous_endpoint: "https://<exact-original-host>/api/v1/otlp"
   ```

   This must equal the original configured string, including any trailing slash.
   It is used only to recompute the old SHA-256 of
   `endpoint + NUL + host.id + NUL + AWS account + NUL + region`.
   It is never contacted. If omitted, the current endpoint is tried; no alternatives
   are guessed. Host/account/region must still match the original deployment.
4. Run the offline configuration check, then start/restart the service. Startup
   resolves real host identity and verifies the exact v1 hash before migrating.
   Check the journal for a successful start and delivered-record counters.
5. After successful migration, remove `previous_endpoint`. Future restarts/replay
   use only the v2 scope. Keeping the same logical IDs across key rotation is required.

`--check` validates configuration/credential availability **offline**; it does not
read/migrate queue state, contact AWS/Observe or prove the remote organization.
Environment mode needs the protected environment available to that command;
systemd ExecStartPre loads its existing EnvironmentFile. Secret-file mode works
with the shown sudo command directly. Never print the key to diagnose a failure.

## Normal endpoint changes after migration

Keep backend/org IDs and host identity unchanged. No spool rename/reset is needed:

```sh
sudo nano /etc/observe-agent/agent.yaml
sudo observe-agent --check --config /etc/observe-agent/agent.yaml
sudo systemctl restart observe-agent
```

Change only `observe.endpoint` to the new verified HTTPS OTLP base. Same-organization
API-key rotation also preserves the queue. Changing any scope member fails closed.

## Crash-safe metadata migration

The exclusive existing Linux flock is held throughout. V1 primary and backup control
copies must agree. A verified transition writes a private, fsynced `migration.json`
journal containing only old/new scope hashes, version and next sequence number.
Dedicated `.next` control stages are fsynced, renamed atomically and the directory
fsynced; backup is committed before primary. Only the completed control journal is
removed. `.rec`/`.bad` payloads and receipts are never rewritten or deleted by migration.
The normal acknowledgement-only dequeue remains unchanged.

After journal commit, recovery verifies the same v2 scope and the journal's exact
old/new manifest state, finishes idempotently and no longer needs previous_endpoint.
A crash before journal commit needs the original endpoint again. Complete identical
stages are reusable; partial/conflicting stages, unsafe files, missing control copies
or an ambiguous existing `.pending` cause a redacted failure with all evidence retained.
Do not delete/rename these files to force startup; obtain operator-assisted recovery.
An old binary cannot open the v2 manifest: downgrade is intentionally fail-closed.

V2 reserves 24 KiB for control/migration work plus the existing bounded directory
reserve, within the configured queue budget (default 64 MiB). Record accounting,
reject-new overflow, item limit and bounded quarantine are unchanged. Filesystem-wide
journal/snapshot/inode overhead still requires a filesystem quota for a hard volume cap.

Delivery remains **at-least-once**, not exactly-once: loss of an acknowledgement may
replay an unchanged batch. This change does not assert backend deduplication. No
telemetry source, listener, log reader, packaging path, unit policy or auth API changed.

## Diagnostics

| Code | Meaning / action |
|---|---|
| queue_scope_identity_required | Set verified backend and organization IDs; no defaults are invented |
| queue_scope_v1_unverified | Original URL or host/cloud identity does not match; verify evidence, retain backlog |
| queue_scope_v2_mismatch | One logical identity member changed; restore the original deployment, never force reuse |
| queue_scope_v1_ambiguous / migration_conflict | Inconsistent control copies; retain all files for recovery |
| queue_scope_staging_conflict / pending_ambiguous | Uncertain interrupted write; no automatic deletion |

Diagnostics expose field names/error codes, not endpoint values, credentials or payloads.
Local tests use synthetic fixtures only. Live EC2/Observe acceptance and filesystem
power-loss testing are separate gates; process-exit crash tests do not prove every
hardware/filesystem failure mode.
<!-- AGENTV1 FILE END -->
