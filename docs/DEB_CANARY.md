<!-- AGENTV1 FILE START: Linux AMD64 canary DEB installation and verified boundaries -->
# Linux AMD64 canary DEB

> Queue-scope v2 supersedes the endpoint-scoped behavior of the historical artifact below. See [one-time migration](QUEUE_SCOPE_V2.md); old local DEBs do not contain the new implementation.

**Test package, not a production release. No EC2 deployment or backend modification was performed.** This milestone adds delivery/install UX only; identity, metrics and durable delivery remain the existing implementations. No published release/download URL exists yet.

## Artifact

- Version: `0.1.0~canary.20260903.1`
- File: `dist/deb/observe-agent_0.1.0~canary.20260903.1_amd64.deb`
- SHA256: `58a14552d4ad62f718b26627b88dac87de1b4d817405d85a15806e96acf85eec`
- Linux AMD64, statically built with Go 1.26.7 (`CGO_ENABLED=0`).
- Target: Debian/Ubuntu AMD64 with systemd, passwd and ca-certificates installed. Other distributions and service managers have not been package-tested.
- `packaging/deb/build.sh` builds the archive; it does not install, publish or upload it. The build timestamp is not yet reproducible-release metadata. This is an unsigned local test artifact; distribute its checksum through a separately trusted channel.

## Installed footprint

| Path / identity | Ownership / permissions | Purpose |
|---|---|---|
| `observe-agent` user/group | System UID/GID; non-root; `/usr/sbin/nologin` | Dedicated service identity; no login or extra capabilities |
| `/usr/bin/observe-agent` | root:root `0755` | Host metrics and opt-in Linux file Logs executable |
| `/etc/observe-agent` | root:observe-agent `0750` | Protected configuration directory |
| `/etc/observe-agent/agent.yaml` | root:observe-agent `0640` | dpkg conffile; readable by service, not writable by service |
| `/var/lib/observe-agent` | observe-agent:observe-agent `0700` | Persistent private state |
| `/var/lib/observe-agent/metrics` | observe-agent:observe-agent `0700`; files `0600` | Durable queue, manifest, lock and recovery files |
| `/var/lib/observe-agent/logs` | observe-agent:observe-agent `0700`; files `0600` | Independent Logs queue and checkpoints |
| `/lib/systemd/system/observe-agent.service` | root:root `0644` | Packaged unit (may resolve under `/usr/lib` on merged-/usr systems) |
| `/usr/share/doc/observe-agent/README.md` | root:root `0644` | Installed short operator guide |
| `/etc/observe-agent/env` (optional, operator-created) | root:root `0600` | systemd EnvironmentFile |
| `/etc/observe-agent/api-key` (optional, operator-created) | root:observe-agent `0640` | Secret-file reference |

The package creates the user/directories/unit automatically. It refuses unsafe symlinks and unexpected pre-existing service identities/groups. It never touches `/usr/local/bin/agent-i`, `/etc/agent-i`, existing agent-i checkpoints or `agent-i.service`. Application log files are read only when an operator explicitly enables and allowlists a Logs source; no trace receiver or inbound service is registered.

The archive contains executable, YAML, unit, README and dpkg lifecycle scripts/conffile declaration. State/user are created at configure time, not shipped with machine-specific IDs. The archive root has mode `0755`; the YAML is private even before postinst applies its service-readable group.

## Shipped default YAML

```yaml
observe:
  backend_id: ""
  organization_id: ""
  endpoint: ""
  api_key: ""
  # api_key_env: OBSERVE_API_KEY
  # api_key_file: /etc/observe-agent/api-key

collection:
  metrics:
    enabled: true
  logs:
    enabled: false
  traces:
    enabled: false

ec2_metadata:
  enabled: true
  required: false
  timeout_seconds: 2

delivery:
  state_directory: /var/lib/observe-agent/metrics
  overflow_policy: reject_new
```

Initial setup also requires stable `observe.backend_id` and the actual `observe.organization_id`; see [queue scope v2](QUEUE_SCOPE_V2.md). Use the backend-provided **HTTPS** public OTLP base ending `/api/v1/otlp`; the sender appends `/v1/metrics`. There is no TLS bypass. Blank identity/endpoint/auth fail closed. Install remains **disabled and stopped**. Offline check cannot prove remote identity/connectivity or migrate a spool. After one-time migration, changing only endpoint needs edit, check and restart, not a new spool.

For the EC2 identity canary, additionally set `ec2_metadata.required: true`: metadata failure must stop the candidate instead of using its supported machine-ID fallback. Do not set a fabricated host/Agent ID. Real IMDSv2 identity must remain the EC2 instance ID.

The frontend accepts strict block-style YAML, not the old agent-i YAML schema. Existing normalized JSON remains supported. Unknown/duplicate keys, anchors/aliases, merge keys, custom scalar tags, and multiple documents are rejected. Linux file Logs use the strict schema documented in [Linux file Logs](LINUX_FILE_LOGS.md); traces enablement is rejected. Size/depth/node bounds apply. YAML/private key files require safe Linux ownership/mode and cannot be symlinks. No automatic legacy config import or rewrite occurs.

### Credentials

Precedence is **configured environment reference > configured secret-file reference > inline key**. A missing configured reference fails closed; it never falls back silently to an inline key. Values may be raw keys or a full `ApiKey <key>` value; output is exactly one Authorization ApiKey prefix.

Inline keys are retained only in the protected config and process memory, not returned by config formatting/JSON diagnostics, logged, or included in telemetry. Raw parse errors are not emitted. Do not share config/env/key files in support bundles or screenshots. There is no secret-recovery API. Core dumps are disabled in the unit.

Safer file mode: use `observe.api_key_file: /etc/observe-agent/api-key`, remove/blank `api_key` and omit `api_key_env`. Create a **new** file once with `sudo install -o root -g observe-agent -m 0640 /dev/null /etc/observe-agent/api-key`, then edit using `sudoedit`; never put the key on the command line. Do not run that creation command against an existing key file because it truncates it.

Environment mode: use `observe.api_key_env: OBSERVE_API_KEY`; create a new root-owned `0600` `/etc/observe-agent/env` with `OBSERVE_API_KEY=<key>` using a protected editor. systemd loads it for both ExecStartPre and ExecStart. A separate `sudo observe-agent --check ...` does **not** automatically load that EnvironmentFile: run with the same securely supplied environment, or use file-reference mode for a directly executable offline check. Do not source/echo an env file in debug shells or use `set -x`.

## Service

The complete authoritative unit is [packaging/deb/observe-agent.service](../packaging/deb/observe-agent.service). It uses:

```ini
User=observe-agent
Group=observe-agent
UMask=0077
EnvironmentFile=-/etc/observe-agent/env
ExecStartPre=/usr/bin/observe-agent --check --config /etc/observe-agent/agent.yaml
ExecStart=/usr/bin/observe-agent --run --config /etc/observe-agent/agent.yaml
Restart=no
TimeoutStopSec=20
StateDirectory=observe-agent
StateDirectoryMode=0700
NoNewPrivileges=yes
CapabilityBoundingSet=
AmbientCapabilities=
ProtectSystem=strict
ProtectHome=read-only
ReadWritePaths=/var/lib/observe-agent
RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6 AF_NETLINK
StandardOutput=journal
StandardError=journal
LimitCORE=0
```

No auto-restart loop or automatic boot enablement is introduced for this canary. Transient delivery retries happen inside the existing sender; permanent authentication/configuration rejection pauses delivery rather than retrying indefinitely. Correct configuration and explicitly restart. Config changes require restart; no SIGHUP/remote-config protocol was added. Root/operator validation can succeed while the service cannot read an incorrectly permissioned custom file, so also validate as the service user when using inline/file mode.

## Durable state and preservation

Defaults: 15-second scrape, 500-point batches, 64 queue records/batches, 64 MiB accounted disk budget, `reject_new` overflow. `limits.queue_bytes` configures the byte bound (when supplying `limits`, supply its other required fields too). This is bounded queue accounting, not a filesystem partition quota. Queue data is private but not encrypted at rest; it contains telemetry, never API keys. Existing Retry-After/backoff/jitter, FIFO, corruption recovery and acknowledgement behavior are unchanged.

Delivery remains **at least once**. An acknowledgement lost after remote acceptance can cause duplicate remote samples. Exactly-once persistence and live replay deduplication have not been proved. Stop/restart uses the same directory, and uninstall never silently destroys retained samples.

- `sudo dpkg --remove observe-agent`: stops service; removes packaged executable/unit; retains YAML, state, account and optional operator env/key files.
- Reinstall: retains edited YAML and spool/account; does not auto-start. Keep the local conffile if dpkg prompts on a future upgrade.
- `sudo dpkg --purge observe-agent`: also removes the dpkg-owned YAML conffile; **retains spool, account and operator-created env/key files**. Purge is not secure erasure. Credential deletion/state deletion need a separately reviewed operator action; no recursive cleanup is supplied.
- Upgrade stops the service first and does not restart it automatically. `dpkg --force-confnew` is not recommended because it discards local settings.

## Verification performed

All tests used local fixtures or isolated Linux; no live organization key/AWS endpoint was used.

| Gate | Result |
|---|---|
| Native Windows `go test -count=1 ./...`, `go vet ./...` | PASS |
| Linux-targeted vet and all test binaries executed natively in WSL Linux | PASS |
| Linux AMD64 / ARM64 and Windows AMD64 compilation | PASS (ARM64/Windows collectors not added) |
| Strict YAML, malformed input, unknown/duplicate fields, redacted format/errors | PASS |
| Inline/env/file precedence and unsafe private-file modes | PASS |
| Existing IMDSv2, exact identity, Metrics/Logs and OTLP contract regressions | PASS using fixtures; not live EC2 |
| Existing 401/422/429/503/network, crash/replay, corruption, saturation, shutdown tests | PASS |
| Fresh DEB install, disabled/stopped service, default invalid start | PASS |
| Real systemd restricted user, start/stop/restart/status/journal | PASS |
| Real Linux metrics -> isolated trusted HTTPS -> accepted | PASS; initial 3 batches / 364 points, one host identity |
| Inline + EnvironmentFile + private secret-file under systemd | PASS |
| 503 backlog -> stop/remove/reinstall -> successful replay | PASS; conffile SHA, retained queue checksums and UID unchanged |
| Journal secret absence, spool/file ownership/modes | PASS |
| Explicit purge policy | PASS: YAML removed; state/account retained |
| Live EC2, real key authentication, canonical backend UUID before/after | NOT RUN; mandatory canary gate |

The DEB lifecycle ran in a dedicated Debian Bookworm container with real systemd PID 1, no host mounts and `--network none`. A local HTTPS fixture validated Authorization and decoded real OTLP metric payloads. The test trust certificate was added **only inside that disposable container**. `packaging/deb/validate.sh` is destructive **only to its guarded test environment**, not an EC2 installation script. `packaging/deb/test.Dockerfile` defines the test image. Docker's cgroup-v1 environment emitted compat-systemd-cgroup attachment warnings while the service was active; full systemd isolation on EC2 still requires live verification. This container used additional test-only privileges to run systemd; the Agent unit itself runs without capabilities.

## Exact operator-run EC2 canary procedure (not executed)

1. Confirm one approved Debian/Ubuntu AMD64 EC2 and organization. Confirm no existing `observe-agent` installation would be overwritten. Record UTC start, canonical EC2 UUID, Agent installation UUID, source rows and relationships. Keep old `agent-i` running; do not edit its files. Use [live evidence procedure](LIVE_EC2_VALIDATION.md) for the tenant-bound before/after queries, substituting the package paths below for its earlier JSON/binary paths. Its earlier no-inline-key guidance applies to that JSON procedure; this protected YAML package explicitly supports inline keys.
2. Obtain this canary DEB from an approved transfer/location. If hosted for this test only:

```sh
wget '<approved-release-url>/observe-agent_0.1.0~canary.20260903.1_amd64.deb'
printf '%s  %s\n' \
  '58a14552d4ad62f718b26627b88dac87de1b4d817405d85a15806e96acf85eec' \
  'observe-agent_0.1.0~canary.20260903.1_amd64.deb' | sha256sum --check -
sudo dpkg -i observe-agent_0.1.0~canary.20260903.1_amd64.deb
systemctl is-enabled observe-agent  # expected disabled; nonzero exit is normal
systemctl is-active observe-agent   # expected inactive; nonzero exit is normal
sudoedit /etc/observe-agent/agent.yaml
```

3. In the protected editor set the valid backend public HTTPS endpoint and organization key (or private reference), leave metrics on/logs+traces off, set EC2 metadata required true. Do not edit spool paths or identity to create a different installation. Then, for inline/file mode:

```sh
sudo observe-agent --check --config /etc/observe-agent/agent.yaml
sudo -u observe-agent observe-agent --check --config /etc/observe-agent/agent.yaml
observe-agent --version
date -u +%FT%TZ
sudo systemctl start observe-agent
sudo systemctl status observe-agent --no-pager
sudo journalctl -u observe-agent --since '5 minutes ago' --no-pager
sudo stat -c '%a %U:%G %n' /etc/observe-agent/agent.yaml /var/lib/observe-agent /var/lib/observe-agent/metrics
pid=$(systemctl show observe-agent -p MainPID --value)
ps -o pid,user,group,args -p "$pid"
sudo ss -lntup  # candidate PID must not own a listener; old Agent may own its own ports
sudo ls -l /proc/"$pid"/fd  # inspect paths only; do not dump file contents/environment
```

4. Allow at least three scrape intervals. Confirm true IMDS instance/account/region and stored resource `host.id`, provider/platform against the existing EC2. Confirm `/api/v1/otlp/v1/metrics` acceptance (currently 201 in Observe), candidate-version samples, unchanged canonical EC2/installation UUID, no duplicate host, both `agent` and `aws_api` sources, independent CloudWatch history and unchanged service relationships. A Live badge alone is insufficient because old/new processes share installation identity.
5. `sudo systemctl restart observe-agent`; verify metrics continue on the same identity and state directory. Real backlog replay/ack-loss testing needs a separately controlled endpoint outage; do not block the old Agent or alter backend identity/security. Record at-least-once limitations rather than claiming zero duplicates. Compare before/after counts with the test UTC window.
6. Finish with `sudo systemctl stop observe-agent`, record UTC end, and confirm old `agent-i` remains unaffected. If uninstall is desired use `sudo dpkg --remove observe-agent`; reinstall later with the same DEB and repeat validation. Do not delete retained spool. Do not enable boot startup or deploy to other hosts until the canary is approved.

Remaining production gates: real EC2/Observe identity and provenance evidence, distro-specific service/sandbox validation, signed/reproducible release distribution, upgrade policy, legacy importer and broader packaging review. No production readiness is claimed.
<!-- AGENTV1 FILE END -->
