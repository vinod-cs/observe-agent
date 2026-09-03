<!-- AGENTV1 FILE START: operator-run single-host validation; not executed against AWS -->
# Live EC2 compatibility validation — prepared, not executed

> AGENTV1 A5.3: The exact staged candidate, protected paths, operator commands and before/after evidence queries are in [A5.3 operator checklist](A53_CANARY_CHECKLIST.md). It implements this procedure without replacing the deployed Agent. [A5.3 results](A53_CANARY_RESULTS.md) explicitly separates local preparation from unexecuted live gates.

No real credential, SSH session, EC2 deployment or platform database write was used in this phase. Canonical entity UUID reuse and live API authentication remain **NOT VERIFIED**. The local backend contract check is normalization only.

## Preconditions and safe canary boundary

1. Select one approved EC2 host. Historical test context is `testing`, `i-0345d461c99a6da2f`, account `127696279140`, region `us-east-2`; confirm these still describe the intended host and organization. Do not assume an organization UUID from an older database.
2. In the existing organization UI, record the canonical EC2 UUID and Agent installation UUID before testing. Record entity Sources and a recent AWS/CloudWatch metric separately. Do not change AWS collection or correlation.
3. Use the existing organization-scoped API key with `metrics.ingest`, via a protected service environment. Never paste its value into a command, URL, YAML, screenshot or report. The expected environment value is the **full** `ApiKey <key>` string. This executable does not itself source `/etc/agent-i/env`.
4. Read the backend-provided public OTLP URL from Manage > Integrations > Agent. Use a valid HTTPS base ending `/api/v1/otlp`. Do not use an example.invalid, localhost or stale tunnel address on EC2.
5. Stage the candidate binary and a **separate** JSON config/state directory for a bounded canary; do not overwrite `/usr/local/bin/agent-i` or deployed YAML/env. Set `ec2_metadata.enabled=true`, `required=true`, empty `agent_id`, metrics true, every other capability false, and the explicit persistent state directory. Set `exporter.backend_id` to the confirmed stable backend label and `exporter.organization_id` to the actual organization UUID. Reusing a prior Observe v1 canary spool requires [verified scope migration](QUEUE_SCOPE_V2.md). Use a local persistent filesystem, 0700 directory and 0600 config.
6. The old Agent emits more than metrics. Do not stop/replace it without a maintenance decision covering its logs/traces. If both run briefly, their real metric samples may overlap; isolate the test time window/version in evidence. No fleet-wide rollout.
7. Both Agents intentionally map to the same installation identity, so `agent_installations.agent_version` and activity can be updated by either process. A Live badge or version on that row alone is not candidate proof. Use candidate-version resource dimensions on persisted samples and candidate-only process diagnostics. Do not change host.id, agent_id, source identity or service name to manufacture an independent installation.

## Identity and metrics acceptance

Use the installed candidate's `--check --config <private-json-path>` first. This must perform no collection/network/state work. Run `--run --config <private-json-path>` under the approved service identity and environment, for at least three scrape intervals (first CPU observation needs a second point).

For independent IMDS confirmation on that host (the token is not printed):

```sh
set +x
imds_token=$(curl --noproxy '*' --fail --silent --show-error --max-time 3 -X PUT \
  -H 'X-aws-ec2-metadata-token-ttl-seconds: 60' \
  http://169.254.169.254/latest/api/token) || exit 1
# Token goes to curl through stdin config, not a process command-line argument.
printf 'header = "X-aws-ec2-metadata-token: %s"\n' "$imds_token" | \
  curl --config - --noproxy '*' --fail --silent --show-error --max-time 3 \
  http://169.254.169.254/latest/dynamic/instance-identity/document
unset imds_token
```

Record only instanceId/accountId/region from the returned document. Compare them to persisted OTLP resource attributes: `host.id`, `cloud.account.id`, `cloud.region`, `cloud.provider=aws`, `cloud.platform=aws_ec2`, `telemetry.distro.name=agent-i`. Hostname/IP/DNS must not determine identity.

Observe existing API access logs for `POST /api/v1/otlp/v1/metrics` success (currently 201), without enabling request-body/header logging. Agent diagnostics must show remote accepted batches and locally delivered records, not merely successful enqueue. A 401/403 is failure, not successful connectivity. No fixture/test telemetry should be sent to the live backend.

## Canonical entity and provenance proof

Using the logged-in organization session and its existing permissions, inspect only stored-data GET routes:

- `/api/v1/integrations/agent` — installation, mapped entity and metrics activity.
- `/api/v1/infrastructure/hosts` — same mapped EC2 UUID.
- `/api/v1/entities/<recorded-ec2-uuid>` — Sources/Relationships and canonical identity.
- `/api/v1/entities/<recorded-ec2-uuid>/metrics?resolution=raw&scope=all&limit=2000` with explicit start/end around the canary window — real host metrics.

These paths were read from the current reference controllers, not invented. Use the browser's existing authenticated request flow; do not print or persist bearer tokens. Ingest-only API keys need not authorize these read endpoints.

Optionally, have an authorized DB operator run tenant-bound **read-only** checks before/after:

```sql
BEGIN TRANSACTION READ ONLY;
SELECT id, entity_type, canonical_key
FROM entities
WHERE organization_id = '<confirmed-org-uuid>'
  AND canonical_key IN (
    'aws:127696279140:us-east-2:ec2:i-0345d461c99a6da2f',
    'host:i-0345d461c99a6da2f'
  );
SELECT entity_id, source_type, source_id
FROM entity_sources
WHERE organization_id = '<confirmed-org-uuid>'
  AND entity_id = '<recorded-ec2-uuid>';
ROLLBACK;
```

Use the application's established tenant DB context/RLS role; an empty result under an unconfigured RLS session is not proof of missing data. Do not disable RLS for this test.

Pass requires exactly the existing EC2 UUID, no new standalone Host/EC2 for this identity, preserved `aws_api` source plus Agent source, host metrics on that UUID with OTLP provenance, and independent AWS/CloudWatch metric identity/history. Confirm CPU, memory, disk, filesystem, network and load in Hosts > Inspect telemetry; storage counters must remain cumulative on the wire. Confirm service relationships remain unchanged.

## Policy, queue and restart observation

- Inspect the candidate process's file descriptors (`ls -l /proc/<candidate-pid>/fd`) and sockets (`ss -lntup`); no listener should be owned by the candidate. No application-log descriptors should exist. Expected writes are private spool files and stderr only.
- Descriptor snapshots alone cannot prove which files were opened and closed earlier. Capture candidate-only file/network syscall metadata for the full canary window if supported (see checklist), without tracing read/write/send payloads or process environments. Otherwise mark the complete-access-footprint gate NOT VERIFIED.
- Confirm record/manifest permissions and bounded state size with `find <canary-state-dir> -maxdepth 1 -printf '%m %f\n'` and `du -sk <canary-state-dir>`. Do not cat queued metrics into shared logs; they contain host metadata.
- Test network interruption only using an approved, candidate-specific fault boundary. Never block the old Agent or all host traffic. Backlog should grow up to the configured bound; overflow rejects new records visibly.
- Stop/restart the candidate with the same state path and identity. Retained records should drain when HTTPS recovers, with no new canonical infrastructure object. A lost remote acknowledgement may yield duplicate deliveries; verify ClickHouse deduplication separately before claiming exactly-once persistence.
- Cancel during retry and confirm prompt shutdown, retained `.rec` files and exclusive-lock release. Do not delete spool rows/files to manufacture a passing result.

## Evidence to return

Record UTC test window, candidate SHA256/version, EC2 identity fields, organization ID, before/after canonical UUID and installation UUID, HTTP status counts, delivery counters, representative real metric timestamps/source, duplicate entity count, source rows, and candidate-owned listener count. Exclude keys, tokens and complete environments. Live acceptance remains pending until this evidence exists.
<!-- AGENTV1 FILE END -->
