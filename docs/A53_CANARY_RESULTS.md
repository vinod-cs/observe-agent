<!-- AGENTV1 FILE START: A5.3 evidence worksheet; never substitute local tests for real acceptance -->
# A5.3 canary results and read-only evidence queries

**Overall: BLOCKED / NOT LIVE-VALIDATED.** Candidate preparation passed. No SSH connection, remote file copy, live IMDS request, credential access, API ingestion, DB mutation or EC2 process/service action was performed. No fleet deployment, package or commit occurred.

## Candidate evidence

| Field | Verified value |
|---|---|
| Version, executed under local Linux | `a5.3-canary-20260902T180500Z` |
| Platform/toolchain | Linux AMD64, GOAMD64=v1, CGO_ENABLED=0, Go 1.26.7 |
| Candidate SHA256 | `DF8981CB01BF07653D3EF2276EE545120974EAD198305B7EFA2ED0883552F5B9` |
| Local candidate | `dist/canary/a5.3-canary-20260902T180500Z/linux_amd64/observe-agent` |
| Config template | `configs/canary-a53.json`; required IMDSv2 identity, metrics only, separate spool/env reference |
| Local Linux `--check` | PASS; no collectors/listeners/secrets/network started |
| Live UTC window | NOT STARTED; start/end must be captured on approved EC2 and correlated with backend UTC |
| Historical intended EC2 | `testing` / `i-0345d461c99a6da2f`, account `127696279140`, region `us-east-2`; needs operator confirmation |

Build stamp time is **not** the live validation window. The prior unversioned build was not overwritten. No production Go source changed; only linker metadata differs.

## Gate worksheet

| Gate | Before / after evidence | Status |
|---|---|---|
| Real IMDSv2 / exact host.id | No live document or persisted candidate resource yet | NOT RUN |
| Correct provider/platform/account/region | Expected aws/aws_ec2/127696279140/us-east-2; not live-observed | NOT RUN |
| API-key authentication | No approved live key used; no HTTP result | NOT RUN |
| Real metric persistence | No candidate-version ClickHouse samples observed | NOT RUN |
| Existing EC2 UUID | BEFORE unknown / AFTER unknown | NOT RUN |
| Agent installation UUID | BEFORE unknown / AFTER unknown | NOT RUN |
| Target Host/EC2 entity count | BEFORE unknown / AFTER unknown | NOT RUN |
| Entity source count / preserved aws_api | BEFORE unknown / AFTER unknown | NOT RUN |
| Separate Agent / OTLP provenance | No live source rows or samples inspected | NOT RUN |
| Independent AWS/CloudWatch samples | No before/after samples observed | NOT RUN |
| Services / relationships unchanged correctly | No baseline or final rows observed | NOT RUN |
| Restricted service identity | Identity not specified; process not run on EC2 | NOT RUN |
| Candidate listener count / no trace receiver | No live process/socket evidence | NOT RUN |
| No application-log reads / documented access only | No complete live syscall audit | NOT RUN |
| Private/bounded queue state | No live state/permissions/size evidence | NOT RUN |
| Restart / retained real telemetry replay | No live candidate restart | NOT RUN |
| Duplicate persistence after acknowledgement loss | No acknowledgement-loss experiment; exactly-once NOT CLAIMED | NOT RUN |
| Current Agent files/service untouched | No remote actions performed; canary paths are separate | PASS (preparation only) |

## Read-only PostgreSQL snapshots

Run through an approved read-only database session. Replace UUID placeholders only after confirming the organization from the UI. These are session-local read-only transactions; no entity, schema or RLS policy mutation. `set_config(..., true)` sets the repository's existing transaction-local tenant context; it does not grant authorization. The account must already have permitted organization read access. Never disable RLS or use another tenant's context.

Run before and after; save only these non-secret results in protected evidence storage. Unrelated background discovery/application activity can change whole-org counts, so investigate attribution rather than demanding whole-org totals never change.

```sql
BEGIN TRANSACTION READ ONLY;
SELECT set_config('app.organization_id','<confirmed-org-uuid>',true);

SELECT id, entity_type, canonical_key, display_name
FROM entities
WHERE organization_id='<confirmed-org-uuid>'
 AND canonical_key IN (
  'aws:127696279140:us-east-2:ec2:i-0345d461c99a6da2f',
  'host:i-0345d461c99a6da2f'
 ) ORDER BY id;

SELECT id, stable_identity, host_id, mapped_entity_id, agent_version,
 cloud_provider, cloud_account_id, cloud_region, first_seen_at, last_seen_at
FROM agent_installations
WHERE organization_id='<confirmed-org-uuid>' AND host_id='i-0345d461c99a6da2f'
ORDER BY id;

SELECT source_type, count(*) AS source_count
FROM entity_sources
WHERE organization_id='<confirmed-org-uuid>' AND entity_id='<existing-ec2-uuid>'
GROUP BY source_type ORDER BY source_type;

SELECT id, entity_id, source_type, source_id, external_id, account_id, region
FROM entity_sources
WHERE organization_id='<confirmed-org-uuid>' AND entity_id='<existing-ec2-uuid>'
ORDER BY source_type,id;

SELECT id,source_entity_id,relationship_type,target_entity_id,source_type,source_id
FROM entity_relationships
WHERE organization_id='<confirmed-org-uuid>'
 AND (source_entity_id='<existing-ec2-uuid>' OR target_entity_id='<existing-ec2-uuid>')
ORDER BY id;

SELECT entity_type,count(*) AS organization_entity_count
FROM entities WHERE organization_id='<confirmed-org-uuid>'
 AND entity_type IN ('host','aws_ec2_instance','service')
GROUP BY entity_type ORDER BY entity_type;
ROLLBACK;
```

Pass requires the **same existing EC2 UUID**, one effective target infrastructure identity, the same installation UUID (not an API-key-dependent identity), preserved AWS source row(s), and no inappropriate service/relationship creation. Source timestamps/metadata can change legitimately; compare relationship identity and endpoints, not last_seen alone.

## Read-only ClickHouse snapshots

The current reference stores metrics in `<actual-database>.metric_samples_v2`, normally `observability.metric_samples_v2`. Confirm the configured database; do not create tables or run `OPTIMIZE`, deletes or inserts. Use an authorized read-only connection, not an ingest key. Substitute confirmed UUIDs and actual UTC time bounds.

Important existing behavior: OTLP rows have `source_type='otel'`, `provider='otel'`, namespace `OTLP`; AWS identity is preserved in **dimensions**, while top-level cloud_account_id/region are empty on OTLP rows. Querying only the top-level AWS columns would hide valid Agent data. This was confirmed from `ClickHouseMetricStore.insertIngested`; no backend behavior was changed.

```sql
SELECT entity_id,source_type,provider,metric_namespace,metric_name,unit,
 observed_at,ingested_at,value,
 dimensions['host.id'] AS host_id,
 dimensions['cloud.provider'] AS cloud_provider,
 dimensions['cloud.platform'] AS cloud_platform,
 dimensions['cloud.account.id'] AS aws_account,
 dimensions['cloud.region'] AS aws_region,
 dimensions['telemetry.distro.name'] AS distro,
 dimensions['telemetry.distro.version'] AS version
FROM observability.metric_samples_v2 FINAL
WHERE organization_id=toUUID('<confirmed-org-uuid>')
 AND dimensions['telemetry.distro.version']='a5.3-canary-20260902T180500Z'
 AND observed_at>=parseDateTime64BestEffort('<START_UTC>')
 AND observed_at<parseDateTime64BestEffort('<END_UTC>')
ORDER BY observed_at DESC LIMIT 200;

-- Do not restrict to the expected entity first: this must detect incorrect mapping.
SELECT entity_id,source_type,metric_namespace,count(*) AS points,
 min(observed_at) AS first_observation,max(observed_at) AS latest_observation
FROM observability.metric_samples_v2 FINAL
WHERE organization_id=toUUID('<confirmed-org-uuid>')
 AND dimensions['telemetry.distro.version']='a5.3-canary-20260902T180500Z'
 AND observed_at>=parseDateTime64BestEffort('<START_UTC>')
 AND observed_at<parseDateTime64BestEffort('<END_UTC>')
GROUP BY entity_id,source_type,metric_namespace;

-- Independent sources on the existing canonical EC2, including AWS publication.
SELECT source_type,provider,metric_namespace,metric_name,count(*) AS points,
 max(observed_at) AS newest
FROM observability.metric_samples_v2 FINAL
WHERE organization_id=toUUID('<confirmed-org-uuid>')
 AND entity_id=toUUID('<existing-ec2-uuid>')
 AND observed_at>=parseDateTime64BestEffort('<START_UTC>')
 AND observed_at<parseDateTime64BestEffort('<END_UTC>')
GROUP BY source_type,provider,metric_namespace,metric_name;

-- Physical rows versus logical sorting-key identity; no FINAL in this query.
SELECT count(*) AS physical_rows,
 uniqExact(tuple(organization_id,entity_id,metric_id,statistic,observed_at,source_id)) AS logical_keys
FROM observability.metric_samples_v2
WHERE organization_id=toUUID('<confirmed-org-uuid>')
 AND dimensions['telemetry.distro.version']='a5.3-canary-20260902T180500Z'
 AND observed_at>=parseDateTime64BestEffort('<START_UTC>')
 AND observed_at<parseDateTime64BestEffort('<END_UTC>');
```

Repeat the last query with `FINAL` and record both before/after replay. ReplacingMergeTree background merges may already remove physical duplicates; zero observed duplicates is not a general exactly-once guarantee. A changed ingestion key can change OTLP source identity; use the same key throughout this canary. Record commit/lost-ack evidence if testing acknowledgement loss; a queued outage replay alone does not prove it.

## Blockers before A5.4

1. Confirmed SSH target/user/trusted host key and approved authentication method.
2. Current public HTTPS ingest URL, protected same-org key provision, and organization read/DB access for baselines.
3. Approved existing non-root service identity and preinstalled syscall-audit tooling/cgroup filtering support.
4. Actual canary window and every live gate above, including before/after UUIDs, source/provenance, real metrics and full footprint.
5. Controlled acknowledgement-loss evidence if that gate is required for A5.4; no exactly-once claim from code or a clean restart.

Only preparation artifacts changed: `configs/canary-a53.json`, `docs/LIVE_EC2_VALIDATION.md`, `docs/A53_CANARY_CHECKLIST.md`, this file, and the new ignored canary build output. No Go collector, exporter, queue, identity or backend source was changed.
<!-- AGENTV1 FILE END -->
