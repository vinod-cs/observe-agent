<!-- AGENTV1 FILE START: read-only contract inspection -->
# Existing backend and Agent compatibility

> Current addition: real OTel pdata batches now pass the existing backend normalizer read-only. See [metrics validation](METRICS_VALIDATION.md). Live authenticated ingestion and database persistence remain unverified; the format/identity contract is verified.

Inspected local references, without modification:

- `D:/ob-cs-repo/Agent-v1/oneagent`
- `D:/ob-cs-repo/ob-second-push/Observability`

## Confirmed backend contracts

| Contract | Source relative to Observability |
|---|---|
| `Authorization: ApiKey <key>`; organization-scoped principal | `backend/api/src/identity/guards.ts` |
| `POST /api/v1/otlp/v1/metrics`, `metrics.ingest` | `backend/api/src/telemetry/metrics/otlp-metric.controller.ts` |
| `POST /api/v1/otlp/v1/logs`, `logs.ingest` | `backend/api/src/telemetry/logs/otlp-log.controller.ts` |
| `POST /api/v1/otlp/v1/traces`, `traces.ingest` | `backend/api/src/telemetry/traces/otlp-trace.controller.ts` |
| Parsed OTLP JSON and structured identity | `backend/api/src/shared/normalization/otlp-{metric,log,trace}-normalization.ts`, `otlp-resource-identity.ts` |
| Tenant-bound installation identity and activity | `backend/api/src/integrations/agent/agent-integration.service.ts` |
| Platform constraint and installation persistence | `database/migrations/022_agent_integrations.up.sql` |

Exporter configuration keeps the `/api/v1/otlp` base, appending `/v1/<signal>`. The request helper uses JSON because the existing controllers normalize a parsed JSON body; protobuf support must not be assumed. The helper checks JSON syntax/size only. It is not an OTLP serializer and does not prove payload schema compatibility. Future integration must use standard OTel serialization with contract fixtures and real backend tests.

`agent-i` distro/version attributes and exact host/cloud identity must survive every signal. Host metrics/logs resolve infrastructure; application traces retain services and an infrastructure reference. CloudWatch and Agent provenance stay separate. No database schema, canonical-key, identity resolver or A1-A4 behavior is modified by this repository.

One key can authorize many hosts. Existing installation stable identity is AWS account + region + host.id where complete, otherwise host.id, within the authenticated organization. API-key ID is informational, not identity. Agent liveness currently comes from accepted host metrics; logs/traces are independent activity. This foundation adds no heartbeat protocol.

## Real limitations, not hidden assumptions

1. **Windows fleet support is not ready in the inspected backend.** Migration 022 permits Linux platform and the service insert uses literal `linux`; UI marks Windows/macOS coming soon. Never label Windows as Linux to work around this. A separately approved backend extension is a prerequisite for Windows fleet rollout, even though a Windows foundation binary cross-compiles.
2. No approved remote-policy delivery endpoint, authentication/signature format, enrollment or installation-binding exchange was identified. `fleet.Transport`/`Verifier` therefore remain interfaces, with no network implementation. Existing ingest authentication is not silently reused as remote-admin authorization.
3. Current Agent YAML, persistent log checkpoints and runtime EC2 detector must be covered by compatibility tests before replacing the old binary. The new JSON foundation parser is explicit, not backward-compatible with old YAML. Neither old config nor state is rewritten here.
4. Source compilation does not prove live OTLP acceptance. No keys, AWS resources, backend database or installed Agent were used for this task. Production identity/secret/service behavior remains a rollout gate.

## API-key flow

```mermaid
sequenceDiagram
  participant Admin as Authorized organization admin
  participant Backend as Existing Observe key API
  participant OS as Protected service environment
  participant Agent as Future Agent sender
  Admin->>Backend: Create fixed-scope Agent ingest key
  Backend-->>Admin: Secret once; retain hash/metadata
  Admin->>OS: Store full ApiKey authorization securely
  Agent->>OS: Resolve configured environment reference
  Agent->>Backend: HTTPS OTLP JSON with Authorization
  Backend->>Backend: Authenticate org, check signal scope, correlate exact host
  Backend-->>Agent: Signal acceptance or structured rejection
```

In plain words: the key grants organization access; it never names the machine. Rotating a key must not create a new host. The diagram is the intended sender integration with existing backend behavior; the current CLI does not make these calls.
<!-- AGENTV1 FILE END -->
