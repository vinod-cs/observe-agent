<!-- AGENTV1 FILE START: customer review, explicit implemented/proposed distinction -->
# Permissions and operational security

> Foundation review retained for history. For the implemented Linux metrics runtime, its exact OS reads and current network/write behavior, use [Linux metrics operational review](LINUX_METRICS.md). The no-sender/no-collector statements below apply only to the earlier foundation.

## What this build actually does

`observe-agent --check --config <path>` opens and reads that config (maximum 64 KiB), validates it and prints a safe result. `--version` prints build metadata. Neither reads host telemetry, looks up EC2 metadata, resolves machine identity, reads authorization, writes a registry, binds a listener or sends network traffic. No shell commands, child processes, service installation or privilege escalation are invoked. The Go runtime still creates normal runtime threads and uses OS facilities; no fixed thread count is promised.

The Linux adapters below run **only when invoked** by tests/future lifecycle wiring. They are not wired into a collection daemon. The permissions table describes proposed collector access, not deployed collection in this foundation. Production review must repeat syscall/file/network tracing after actual OTel components are pinned.

## Collector-by-collector matrix

| Capability | Proposed Linux read paths/API | Proposed Windows / macOS access | Least privilege and effects |
|---|---|---|---|
| CPU / memory | `/proc/stat`, `/proc/meminfo`, uptime; selected sysfs only if required | Native performance/memory APIs; later Mach/sysctl | Usually restricted service account; read only; exact APIs depend on selected OTel version |
| Disk counters | `/proc/diskstats`, approved block-device metadata in `/sys` | Performance counters; later IOKit/sysctl | Read counters, not raw disk contents; report unavailable rather than zero |
| Filesystem capacity | Mount table (`/proc/self/mountinfo` or supported equivalent), statfs on approved mounts | Volume enumeration/free-space APIs; later statfs | Traverse/read allowed mount metadata; no filesystem writes or recursive file indexing; remote mounts need explicit policy |
| Network counters | `/proc/net/dev`, interface metadata | Interface performance APIs; later route/sysctl | Read aggregate counters, not packets; no packet capture or network reconfiguration |
| File logs | Customer allowlisted files/globs only; stat/open/read and file identity | Allowlisted files with read ACL; later same principle | Restricted account plus explicit file/group ACL; protected logs require targeted grants, not universal root |
| journald | Approved local journal API/files | Not applicable | `systemd-journal` or explicit journal ACL only when enabled; no journal writes |
| Windows Event Log | Not applicable | Explicit channel subscriptions/reads | Channel ACL or Event Log Readers where appropriate; Security channel may need additional grants; no clearing/modification |
| OTLP traces | Enabled loopback listener; no process injection | Same listener boundary | Port above 1024 normally needs no elevation; app instrumentation sends traces; no automatic tracing of arbitrary processes |
| Process inspection | Selected `/proc/<pid>` metadata | Process query APIs | Same-user access first; protected processes may need extra grants; never imply complete coverage under restricted access |
| Container metrics | Explicitly allowed runtime endpoint | Approved runtime endpoint | Socket access can grant powerful runtime control even if Agent intends only reads; do not mount it by default; this capability is deferred |
| Prometheus | Explicit approved scrape URLs | Same | Outbound requests only when configured; SSRF/destination restrictions and credentials require separate review |
| EC2 enrichment | IMDSv2 token + identity endpoints on link-local address | Same on EC2 | No IAM modification; bound requests, disable proxies for metadata; token PUT creates a metadata token, not workload mutation; detector not implemented |
| eBPF | None | None | Out of scope; would need separate kernel/capability review |

No collector assumes root/Administrator. The executable must report unsupported/permission-denied coverage explicitly. It must not fabricate zeros, silently retry protected resources in a tight loop, or chmod customer files to gain access. Basic metrics and each optional reader receive separate permission declarations.

## Listener and connection inventory

| Connection | Current build | Future enabled behavior |
|---|---|---|
| OTLP ingestion outbound | Request construction only, no sender | HTTPS to configured `/api/v1/otlp/v1/metrics`, `/logs`, `/traces`; port from validated URL, normally 443 |
| Local trace receiver | None bound | Preserve existing Agent's loopback `127.0.0.1:4319/v1/traces` contract when introduced; no remote bind by default |
| Remote policy | None | Authenticated outbound HTTPS to a backend-approved endpoint, which does not exist in the inspected contract |
| EC2 metadata | None | Bounded link-local IMDSv2 requests only with explicit detector support |
| DNS / proxy | No outbound requests from CLI | Sender may resolve configured endpoint/proxy; Go default transport honors proxy environment; TLS remains validated end to end |
| Diagnostic HTTP server | None | Must remain disabled unless separately designed and approved |

Config only permits HTTPS and `/api/v1/otlp` base; URL user-info, query and fragment are rejected. TLS helper uses system trust, minimum TLS 1.2 and no insecure-skip-verify option. Redirects are refused to prevent authorization forwarding. Proxy credential handling and custom enterprise CA installation belong to the host/approved deployment, not to remote policy. No ngrok or download URL is hardcoded.

## API-key handling

Config contains `headers_env.Authorization = OBSERVE_AUTHORIZATION`, **not the key**. The protected service environment supplies `ApiKey <key>`. The existing organization-scoped ingest key authorizes metrics/logs/traces and may be shared across hosts. Host identity is independent of the key. Rotation changes the environment reference value, not host.id or installation identity.

The secret boundary rejects missing, wrong-prefix, control-character or overlarge authorization values with redacted messages. Request headers contain the secret only when constructing an enabled signal request. Do not log requests, dump environments, include headers in errors, persist secret-bearing HTTP requests, or return secrets in diagnostics. The byte copy is cleared best-effort, but Go strings, environment variables and net/http headers are not guaranteed zeroizable. A future protected-secret-provider implementation can reduce retention; memory-erasure guarantees are not claimed.

Environment files must be service-readable only (Linux typically root/service-owned 0600; Windows service SID ACL). Crash dumps may contain memory and require restricted ACL/access and retention policy. This foundation creates no environment file and requests no real credential.

## Files/directories touched

| Path | Implemented access / proposed ownership |
|---|---|
| Supplied JSON config | Current CLI read only; bounded 64 KiB; does not rewrite it |
| `/etc/machine-id` | Linux identity adapter reads up to 257 bytes and validates; never creates/regenerates it |
| `/etc/agent-i/agent.json` | Foundation Linux default config path; legacy YAML untouched |
| `/etc/agent-i/env` | Layout reference only; no read/write by current CLI; future service environment manager owns it |
| `/var/lib/agent-i` | Proposed service-owned 0700 state directory; foundation does not create it |
| Fixed local LKG file in private state directory | Linux adapter bounded read and atomic temp-write/fsync/rename/directory-fsync; temp file 0600; parent must be service-owned and private |
| `/var/log/agent-i` | Proposed diagnostics location only; no file logging wired |
| `%ProgramData%/Observe/Agent` | Proposed Windows layout; current placeholder not a service install; use trusted Windows Known Folder API/ACL validation before production |
| macOS `/Library/...` layout | Future only; no daemon or PKG installed |

Linux state replacement neither deletes customer data nor creates/chmods directories. It removes only its own temporary file after success/failure. Paths must be fixed trusted local settings; remote policy cannot select them. Ancestor symlink/race hardening and durable checkpoint compatibility require further review before a production daemon uses the primitive. After an ambiguous storage failure the lifecycle reports degraded rather than claiming an LKG commit.

## Resource and failure bounds

| Bound | Foundation enforcement |
|---|---|
| Config / remote policy / local policy | 64 KiB maximum |
| OTLP request | Bounded read; default/max 4 MiB; syntactically valid JSON; actual OTLP schema/serializer deferred |
| Authorization | Maximum 4096 bytes; prefix and whitespace/control validation |
| HTTPS helper | 15-second whole-request timeout when sender eventually invokes it; redirects denied |
| Shutdown | Default 15 seconds, config 1–120; cooperative Collector.Stop contract; CLI does not run collectors |
| Queue | Config default 64 MiB, bounds 1 MiB–1 GiB; **no queue implementation or runtime enforcement yet** |
| Memory | Config default 128 MiB, bounds 32–4096; **not yet a hard RSS limit/memory-limiter implementation** |
| Goroutines/processes | CLI has Go runtime threads; lifecycle only constructs enabled registrations; future collector worker counts require measured bounds |

Exporter request building does not provide retry/delivery/idempotency guarantees yet. Future queue must bound disk and memory, expose rejected/dropped counts, acknowledge only after backend success, handle 401/403 as credential problems, 429 with backoff and capped transient retries. No busy-loop retry or unbounded batch behavior should be introduced.

## Enabling and disabling

| Toggle | Enable (future collector implementation) | Disable |
|---|---|---|
| Metrics | Start only approved host readers | Cancel sampling and join workers |
| Logs | Open only configured permitted files/channels; resume safe checkpoints | Close readers/subscriptions; retain checkpoints/history |
| Traces | Bind approved loopback receiver | Stop accepting, close listener and bounded drain |
| Processes / containers | Explicit local opt-in and permission preflight | Close probes/runtime clients; no continuing background reads |

Unimplemented capabilities fail preflight before any constructor. A remote toggle cannot bypass `remote_allowed`, OS permissions, identity validation or unavailable implementation. Delivery/drain/checkpoint behavior remains to be implemented and tested with standard OTel components.

## Agent does not modify customer workloads

No application restarts, injection, package installation, IAM changes, firewall changes, process killing, kernel tuning, customer-file chmod/chown, log rewriting, log deletion or automatic code instrumentation. Managed setup may eventually create **Agent-owned** service/config/state files with explicit administrator approval; that is separate from collection. Remote policy is data-only, never a command runner.

## Evidence for incident review

Retain exact binary build/version/hash, locally approved capability policy, redacted lifecycle events, OS account/ACL grants, open-file/listener inventory and timing of enable/disable operations. Before production, capture Linux syscall/file/network traces and Windows Process Monitor/ETW evidence for each capability profile, including permission denial, log rotation, low disk, network failures and shutdown. Current unit tests prove foundation invariants; they are not runtime evidence for collectors that have not been implemented.
<!-- AGENTV1 FILE END -->
