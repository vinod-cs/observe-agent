<!-- AGENTV1 FILE START: installed test-package guidance -->
# Observe Agent metrics and Linux file/journald Logs canary

This is a TEST package, not a production replacement for agent-i. It does not touch
`/usr/local/bin/agent-i`, `/etc/agent-i`, or `agent-i.service`. No download service
or real release URL is published by this build.

Install with `sudo dpkg -i observe-agent_<version>_amd64.deb` on Debian/Ubuntu with
systemd, passwd and ca-certificates installed. Installation never starts/enables
the service. Edit `/etc/observe-agent/agent.yaml`, then:

Set stable `observe.backend_id`, the actual `observe.organization_id`, the HTTPS
endpoint and credential/reference. These IDs are non-secret operator assertions;
verify the key and endpoint belong to that organization/backend. For a v1 queue
whose endpoint changed, set one-time `observe.previous_endpoint` to the exact old
URL. Startup verifies the old hash before migration; mismatches retain all backlog.
After successful v2 migration, remove previous_endpoint. Future endpoint changes
need only edit YAML, `observe-agent --check --config /etc/observe-agent/agent.yaml`,
and `systemctl restart observe-agent` (with sudo). Never rename/delete the spool.
Changed backend/org/host/account/region fails closed. Downgrading to a v1 binary
after migration is not supported. Offline --check does not migrate or inspect state.

```sh
sudo observe-agent --check --config /etc/observe-agent/agent.yaml
sudo systemctl start observe-agent
sudo systemctl status observe-agent
sudo journalctl -u observe-agent
sudo systemctl restart observe-agent
sudo systemctl stop observe-agent
```

YAML config is root:observe-agent 0640 in a 0750 directory. Inline keys are permitted
but never log or share the config. Prefer `observe.api_key_env: OBSERVE_API_KEY`
with a root-owned 0600 `/etc/observe-agent/env` containing `OBSERVE_API_KEY=...`, or
`observe.api_key_file` pointing at a root:observe-agent 0640 file. Environment
reference takes precedence over file, then inline. Missing references fail closed.
Values may be a raw key or `ApiKey <key>`. Do not put real keys on command lines.
`sudo --check` does not automatically load systemd's env file: use the same protected
environment or rely on ExecStartPre under systemd; secret-file mode works directly.

Linux file and journald Logs are opt-in and disabled by default; traces remain
unsupported. Each file source requires an explicit absolute authorization root plus
direct-child include/exclude patterns. The service account needs read/traverse ACLs
only for those roots/files. Symlink/path escapes are rejected. Journald requires an
operator-granted `systemd-journal` group or equivalent ACL; package scripts do not
change those permissions. YAML keys are strict;
duplicate/unknown keys, aliases, merges and multiple documents are rejected.
This is a new YAML schema, not an importer for old agent-i YAML. Existing JSON remains
supported. YAML `--check` validates references offline; it cannot prove a key is
accepted by the remote server. The endpoint must be a verified HTTPS /api/v1/otlp base.

The restricted non-login system user owns `/var/lib/observe-agent`, the unchanged
Metrics spool, and independent `/var/lib/observe-agent/logs/{queue,checkpoints}`
state (0700 directories, 0600 records). Logs defaults to a separate 64 MiB / 1024
record reject-new FIFO. File offsets and the journald cursor advance only after queue
admission is durable. Delivery is at-least-once, not exactly-once.

`dpkg --remove observe-agent` stops the service, retains YAML, state and account.
Reinstall preserves edited conffiles (choose keep-local on dpkg's upgrade prompt).
No automatic restart occurs on upgrade; explicitly validate and start it.
`dpkg --purge observe-agent` additionally deletes the dpkg YAML conffile, but retains
spool, account and operator-created env/key files. Purge is not a telemetry eraser.
Do not remove retained state until its delivery/retention implications are reviewed.
<!-- AGENTV1 FILE END -->
