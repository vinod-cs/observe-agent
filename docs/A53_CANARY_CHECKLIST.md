<!-- AGENTV1 FILE START: A5.3 operator-only single-host commands; no deployed change executed -->
# A5.3 — exact single-EC2 canary checklist

Authority: [LIVE_EC2_VALIDATION.md](LIVE_EC2_VALIDATION.md). These commands are **prepared, not executed on EC2**. Stop at any unmet prerequisite. Never substitute an old tunnel/key or unconfirmed host. No package, persistent service installation, backend mutation, IAM change or old-Agent restart is required.

## 1. Confirm access and baseline before sending telemetry

Required operator inputs: approved EC2 SSH address/user, trusted host fingerprint, approved SSH identity, intended existing **non-root** service user/group, current backend public HTTPS OTLP base, organization-scoped ingestion key, and an authorized organization read session/DB context. None was supplied for automatic use.

Historical target is `testing` / `i-0345d461c99a6da2f`, account `127696279140`, region `us-east-2`, Acme Commerce. Verify it is still the approved target. Record its existing entity UUID and Agent installation UUID before starting. If the canonical EC2 is absent, **stop**: creating a standalone host would not pass this canary.

Candidate:

- Version: `a5.3-canary-20260902T180500Z`
- Linux AMD64, Go 1.26.7, CGO disabled; existing linker variable only, no Go source changed.
- SHA256: `DF8981CB01BF07653D3EF2276EE545120974EAD198305B7EFA2ED0883552F5B9`
- Local binary: `D:\ob-cs-repo\Updated-Agent-v1\dist\canary\a5.3-canary-20260902T180500Z\linux_amd64\observe-agent`
- Local config template: `D:\ob-cs-repo\Updated-Agent-v1\configs\canary-a53.json`

Exact new EC2 paths:

| Purpose | Path |
|---|---|
| Candidate binary | `/opt/observe-agent-canary-a53/observe-agent` |
| Candidate JSON | `/etc/observe-agent-canary-a53/agent.json` |
| Candidate environment | `/etc/observe-agent-canary-a53/env` |
| Persistent spool | `/var/lib/observe-agent-canary-a53/metrics` |
| Private evidence | `/var/lib/observe-agent-canary-a53/evidence` |
| Temporary unit | `observe-agent-a53-canary.service` |

Do not touch `/usr/local/bin/agent-i`, `/etc/agent-i/agent.yaml`, `/etc/agent-i/env`, existing registry/checkpoints or `agent-i.service` configuration/state. The canary is a separate process; it deliberately shares canonical host and installation identity.

## 2. Stage files from Windows

First replace placeholders with approved values. These commands do not read the SSH private key into output. An unknown host-key error requires fingerprint verification, not disabling host-key checks.

```powershell
Set-Location 'D:\ob-cs-repo\Updated-Agent-v1'
$CanarySshTarget = '<SSH_USER>@<APPROVED_EC2_ADDRESS>'
$CanaryIdentityFile = '<APPROVED_SSH_IDENTITY_PATH>'
$CanaryBinary = 'dist\canary\a5.3-canary-20260902T180500Z\linux_amd64\observe-agent'
Get-FileHash -LiteralPath $CanaryBinary -Algorithm SHA256
ssh -o StrictHostKeyChecking=yes -o IdentitiesOnly=yes -i $CanaryIdentityFile $CanarySshTarget 'test ! -e observe-agent-a53.candidate && test ! -e observe-agent-a53.json'
if ($LASTEXITCODE -ne 0) { throw 'Staging names already exist or SSH failed; stop.' }
scp -o StrictHostKeyChecking=yes -o IdentitiesOnly=yes -i $CanaryIdentityFile $CanaryBinary "${CanarySshTarget}:observe-agent-a53.candidate"
if ($LASTEXITCODE -ne 0) { throw 'Binary copy failed' }
scp -o StrictHostKeyChecking=yes -o IdentitiesOnly=yes -i $CanaryIdentityFile configs\canary-a53.json "${CanarySshTarget}:observe-agent-a53.json"
if ($LASTEXITCODE -ne 0) { throw 'Config copy failed' }
ssh -t -o StrictHostKeyChecking=yes -o IdentitiesOnly=yes -i $CanaryIdentityFile $CanarySshTarget
```

## 3. Prepare restricted candidate files on EC2

Run in Bash. `CANARY_USER` must be an approved existing service identity, not root or the general SSH login. Do not create a user as part of this checklist; obtain approval if none exists. `jq`, `strace`, `systemd-run`, `ss` must already be present; do not install packages silently.

```bash
set -euo pipefail
set +x
umask 077
CANARY_USER='<APPROVED_EXISTING_SERVICE_USER>'
CANARY_GROUP=$(id -gn "$CANARY_USER")
test "$(id -u "$CANARY_USER")" -ne 0
test "$(uname -m)" = x86_64
command -v jq strace systemd-run ss
printf '%s  %s\n' 'df8981cb01bf07653d3ef2276ee545120974ead198305b7efa2ed0883552f5b9' \
  observe-agent-a53.candidate | sha256sum -c -
for p in /opt/observe-agent-canary-a53 /etc/observe-agent-canary-a53 /var/lib/observe-agent-canary-a53; do
  if sudo test -e "$p"; then printf 'Existing path: %s; stop, do not overwrite.\n' "$p"; exit 1; fi
done
test "$(systemctl show observe-agent-a53-canary.service -p LoadState --value)" = not-found
sudo install -d -o root -g root -m 0755 /opt/observe-agent-canary-a53
sudo install -d -o "$CANARY_USER" -g "$CANARY_GROUP" -m 0700 \
  /etc/observe-agent-canary-a53 /var/lib/observe-agent-canary-a53 \
  /var/lib/observe-agent-canary-a53/metrics /var/lib/observe-agent-canary-a53/evidence
sudo install -o root -g root -m 0755 observe-agent-a53.candidate /opt/observe-agent-canary-a53/observe-agent
read -r -p 'Current backend public HTTPS OTLP base: ' OTLP_BASE
test "$OTLP_BASE" != 'https://ingest.example.invalid/api/v1/otlp'
jq --arg endpoint "$OTLP_BASE" '.exporter.endpoint=$endpoint' observe-agent-a53.json > observe-agent-a53.ready.json
sudo install -o "$CANARY_USER" -g "$CANARY_GROUP" -m 0600 observe-agent-a53.ready.json /etc/observe-agent-canary-a53/agent.json
sudo -u "$CANARY_USER" /opt/observe-agent-canary-a53/observe-agent --version
sudo -u "$CANARY_USER" /opt/observe-agent-canary-a53/observe-agent --check --config /etc/observe-agent-canary-a53/agent.json
```

Create **only the new** environment file with a masked prompt. Paste the raw organization key, without the `ApiKey ` prefix; the command supplies it. Nothing is printed, placed in a command argument or copied from the old env file. Never enable `set -x`, print the environment or include it in evidence. Use a key belonging to the same organization as the baseline UUID; do not rotate the old Agent's key for this test.

```bash
sudo bash -c '
set -e; set +x; umask 077
test ! -e /etc/observe-agent-canary-a53/env
read -r -s -p "Organization ingest key (raw, hidden): " candidate_key </dev/tty
printf "\n" >/dev/tty
case "$candidate_key" in ""|*[!a-zA-Z0-9_.-]*) unset candidate_key; echo "Unexpected key format; stop without writing."; exit 1;; esac
printf "OBSERVE_CANARY_AUTHORIZATION=ApiKey %s\n" "$candidate_key" > /etc/observe-agent-canary-a53/env
unset candidate_key
chmod 0600 /etc/observe-agent-canary-a53/env
'
```

If the existing key format differs, do not transform or truncate it; use an approved secret provisioner that safely quotes systemd EnvironmentFile syntax. The env file is root-owned; PID 1 reads it and supplies the variable to the restricted process. The candidate does not source deployed shell/env files.

Record baseline UTC and old service PID/metadata without reading secrets:

```bash
date -u +'%Y-%m-%dT%H:%M:%SZ'
systemctl show agent-i.service -p MainPID -p ActiveState -p SubState
sudo stat -c '%n inode=%i bytes=%s mtime=%Y mode=%a owner=%U:%G' \
  /usr/local/bin/agent-i /etc/agent-i/agent.yaml /etc/agent-i/env
```

Complete the original procedure's real IMDSv2 document check as this same restricted identity. Do not weaken `ec2_metadata.required=true` if it fails. Record exact instance/account/region/AZ and compare them with persisted resource attributes later.

## 4. Run a bounded, observed canary

This is a transient unit, not an installed/enabled service. Use the same Bash session variables. strace captures **only file/network syscall metadata**, not read/write/send buffers, HTTP headers, environments or exec arguments. No `trace=all`, `read`, `write`, `sendto`, `sendmsg`, or `execve` capture. A descriptor snapshot alone is insufficient to prove absence of transient application-log reads.

```bash
CANARY_START_UTC=$(date -u +'%Y-%m-%dT%H:%M:%SZ')
printf 'Canary start UTC: %s\n' "$CANARY_START_UTC"
sudo systemd-run --unit=observe-agent-a53-canary --collect \
  --uid="$CANARY_USER" --gid="$CANARY_GROUP" \
  -p Type=exec -p Restart=no -p RuntimeMaxSec=600 -p TimeoutStopSec=20 \
  -p KillMode=control-group -p UMask=0077 -p NoNewPrivileges=yes \
  -p ProtectSystem=strict -p ReadWritePaths=/var/lib/observe-agent-canary-a53 \
  -p LimitFSIZE=8388608 \
  -p EnvironmentFile=/etc/observe-agent-canary-a53/env \
  /usr/bin/strace -f -qq -s 256 \
  -e trace=open,openat,openat2,stat,lstat,newfstatat,statx,readlink,readlinkat,access,faccessat,statfs,fstatfs,socket,bind,listen,connect \
  -o /var/lib/observe-agent-canary-a53/evidence/footprint-first.txt \
  /opt/observe-agent-canary-a53/observe-agent --run --config /etc/observe-agent-canary-a53/agent.json
systemctl show observe-agent-a53-canary.service -p ActiveState -p SubState -p MainPID -p User -p Group -p Result
```

If ptrace/strace/syscall names are unsupported, stop the candidate and report the footprint gate blocked; do not remove audit coverage then claim it passed. LimitFSIZE bounds each trace/evidence file; a truncated audit is not complete evidence. The old Agent continues untouched.

After at least three collection intervals (45s), inspect **candidate-only** diagnostics and PIDs:

```bash
TRACE_PID=$(systemctl show observe-agent-a53-canary.service -p MainPID --value)
CANDIDATE_PID=$(ps --ppid "$TRACE_PID" -o pid=,comm= | awk '$2=="observe-agent" {print $1}')
test -n "$CANDIDATE_PID"
test "$(sudo readlink /proc/"$CANDIDATE_PID"/exe)" = /opt/observe-agent-canary-a53/observe-agent
ps -p "$CANDIDATE_PID" -o pid,user,group,comm
sudo ls -l /proc/"$CANDIDATE_PID"/fd
sudo ss -lntup | grep -F "pid=$CANDIDATE_PID," || true
sudo journalctl -u observe-agent-a53-canary.service --since "$CANARY_START_UTC" --no-pager -o short-iso-precise
sudo find /var/lib/observe-agent-canary-a53/metrics -maxdepth 1 -printf '%m %u:%g %s %f\n'
sudo du -sk /var/lib/observe-agent-canary-a53/metrics
```

Expect no candidate listener, non-root UID, 0700 spool/0600 files, delivered records and no auth failures. Inspect trace metadata across the complete window for unexpected opens/listen/bind; allow documented proc/sys/device/mount/identity/config/spool files, DNS resolver/CA files and HTTPS sockets. Go may read system trust/DNS/timezone/runtime metadata. Record actual exceptions; do not claim an exact allowlist from a source review alone. No `/proc/<pid>/environ` or credential-file contents in evidence. A failed `ss`/PID lookup is not proof of zero listeners.

## 5. Candidate-only queue retention and restart

Take a persisted-data snapshot first. If the host's systemd supports cgroup IP filters, the following affects **only the canary unit**, not the host firewall or old Agent:

```bash
sudo systemctl set-property --runtime observe-agent-a53-canary.service \
  IPAddressDeny=any IPAddressAllow=169.254.169.254/32
systemctl show observe-agent-a53-canary.service -p IPAddressDeny -p IPAddressAllow
```

Wait two scrape intervals; confirm new `.rec` files remain and no candidate deliveries occur. Property output alone is not enforcement proof. If filtering is unsupported or export continues, stop this fault test and mark it BLOCKED. Do not use global iptables, modify DNS or change the endpoint/spool identity. Keep the outage short: do not intentionally fill disk on a production host.

```bash
sudo find /var/lib/observe-agent-canary-a53/metrics -maxdepth 1 -name '*.rec' -printf '%f %s\n'
sudo systemctl stop observe-agent-a53-canary.service
sudo find /var/lib/observe-agent-canary-a53/metrics -maxdepth 1 -name '*.rec' -printf '%f %s\n'
```

Record retained receipt names/counts. Confirm the first candidate exited and released its spool lock. Re-run **section 4's systemd-run command** with `--unit=observe-agent-a53-canary-replay` and output file `footprint-restart.txt`, recording a separate UTC restart timestamp. First confirm this replay unit does not already exist. Keep the same binary/config/env/spool paths. The distinct transient unit avoids inheriting runtime IP-filter drop-ins from the faulted unit; it does not change Agent/host identity. For all section 4 inspection commands now substitute `observe-agent-a53-canary-replay.service`. Verify that unit has no IPAddressDeny restriction before interpreting recovery. Do not delete/rename queued records. Verify backlog drains, candidate-version rows from the outage interval become queryable, the EC2 UUID/installation UUID stay unchanged and old service PID/state remain unaffected.

This demonstrates outage retention/replay, not necessarily remote-acknowledgement loss. For ack-loss, require an explicitly approved candidate-only response-loss fault boundary and an observed remote commit before local Ack. Do not fabricate it by inserting SQL rows or treating a normal restart as ack-loss. Without that evidence the ack-loss gate remains NOT VERIFIED; queries below can still report naturally observed duplicate physical/logical rows. No exactly-once claim.

Stop only the canary when evidence is complete:

```bash
sudo systemctl stop observe-agent-a53-canary-replay.service
date -u +'%Y-%m-%dT%H:%M:%SZ'
systemctl show agent-i.service -p MainPID -p ActiveState -p SubState
```

If restart testing was skipped, stop `observe-agent-a53-canary.service` instead. Retain private canary state/evidence for review. Do not leave a running canary, enable a service or delete the retained spool/key automatically. Follow normal protected-secret retention after review. Runtime-only IP-filter drop-ins belong only to the first canary unit; they disappear on reboot and must not be copied to any real service.

## 6. Backend evidence and acceptance

Use the organization-scoped stored-data APIs in the authoritative procedure. An ingestion key is not a read-access token. Collect BEFORE, during, AFTER-restart and final snapshots using an authorized operator session. Keep API tokens/DB credentials out of commands/reports.

The reference code persists version and resource identity in metric `dimensions`. Filter `dimensions['telemetry.distro.version']='a5.3-canary-20260902T180500Z'` to distinguish the candidate from the still-running Agent. The installation's version can legitimately alternate because both processes share identity. Its liveness alone is not candidate proof.

Read-only SQL templates and a result worksheet are in [A5.3 results](A53_CANARY_RESULTS.md). Do not mark A5.3 passed until actual evidence fills every required gate. A local build/normalizer result cannot fill a live UUID, auth, footprint or persistence result.
<!-- AGENTV1 FILE END -->
