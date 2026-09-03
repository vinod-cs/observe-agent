#!/bin/bash
# AGENTV1 FILE START: destructive-to-test-container-only package lifecycle exercise.
set -euo pipefail
test "${OBSERVE_DEB_TEST_ONLY:-}" = yes
test "$(cat /proc/1/comm)" = systemd
test ! -e /usr/local/bin/agent-i
deb=/opt/observe-agent.deb
dpkg -i "$deb"
test "$(stat -c %a /)" = 755
test "$(systemctl is-enabled observe-agent)" = disabled
test "$(systemctl is-active observe-agent)" = inactive
if /usr/bin/observe-agent --check --config /etc/observe-agent/agent.yaml; then echo 'Unconfigured check unexpectedly passed';exit 1;fi
if systemctl start observe-agent; then echo 'Unconfigured service unexpectedly started';exit 1;fi
echo 'PASS: default invalid config fails closed; install does not start/enable'
systemctl reset-failed observe-agent
test "$(stat -c '%a %U:%G' /etc/observe-agent/agent.yaml)" = '640 root:observe-agent'
test "$(stat -c '%a %U:%G' /var/lib/observe-agent/metrics)" = '700 observe-agent:observe-agent'
test "$(id -u observe-agent)" -ne 0
test "$(getent passwd observe-agent | cut -d: -f7)" = /usr/sbin/nologin

openssl req -x509 -newkey rsa:2048 -nodes -days 1 -subj /CN=localhost -addext 'subjectAltName=IP:127.0.0.1' -keyout /tmp/fixture.key -out /tmp/fixture.crt >/dev/null 2>&1
chmod 0600 /tmp/fixture.key
install -m 0644 /tmp/fixture.crt /usr/local/share/ca-certificates/observe-test.crt
update-ca-certificates >/dev/null 2>&1
node /opt/deb-test/fixture.cjs >/tmp/fixture-output 2>&1 & fixture_pid=$!
trap 'kill "$fixture_pid" 2>/dev/null || true' EXIT
# Test-only credential is not real. This config and all evidence live in the disposable container.
printf 'observe:\n  endpoint: https://127.0.0.1:8443/api/v1/otlp\n  api_key: package-test-key-not-real\ncollection:\n  interval_seconds: 5\n  metrics:\n    enabled: true\n  logs:\n    enabled: false\n  traces:\n    enabled: false\nec2_metadata:\n  enabled: false\ndelivery:\n  state_directory: /var/lib/observe-agent/metrics\n' > /etc/observe-agent/agent.yaml
chmod 0640 /etc/observe-agent/agent.yaml
# AGENTV1 START: deployment-scoped v2 package fixture, never real credentials.
sed -i '/^observe:/a\  backend_id: fixture-backend\n  organization_id: fixture-org' /etc/observe-agent/agent.yaml
# AGENTV1 END: fixture identity
runuser -u observe-agent -- /usr/bin/observe-agent --check --config /etc/observe-agent/agent.yaml
systemctl start observe-agent
sleep 12
systemctl is-active --quiet observe-agent
pid=$(systemctl show observe-agent -p MainPID --value)
test "$(stat -c %U /proc/"$pid")" = observe-agent
node -e 'const s=require("/tmp/fixture-summary.json");if(s.accepted<1||s.points<1||s.hosts.length!==1||s.secretInPayload)process.exit(1);console.log("PASS: restricted service -> real host metrics -> TLS",{accepted:s.accepted,points:s.points,hosts:s.hosts.length})'
systemctl status observe-agent --no-pager
# Test both references under real systemd. Invalid inline value must be ignored.
printf 'OBSERVE_API_KEY=package-test-key-not-real\n' > /etc/observe-agent/env
chmod 0600 /etc/observe-agent/env
sed -i 's/api_key: package-test-key-not-real/api_key: ignored-inline-fixture\n  api_key_env: OBSERVE_API_KEY/' /etc/observe-agent/agent.yaml
accepted=$(node -p 'require("/tmp/fixture-summary.json").accepted')
systemctl restart observe-agent
sleep 6
test "$(node -p 'require("/tmp/fixture-summary.json").accepted')" -gt "$accepted"
printf 'package-test-key-not-real\n' > /etc/observe-agent/api-key
chown root:observe-agent /etc/observe-agent/api-key
chmod 0640 /etc/observe-agent/api-key
sed -i 's@api_key_env: OBSERVE_API_KEY@api_key_file: /etc/observe-agent/api-key@' /etc/observe-agent/agent.yaml
runuser -u observe-agent -- observe-agent --check --config /etc/observe-agent/agent.yaml
accepted=$(node -p 'require("/tmp/fixture-summary.json").accepted')
systemctl restart observe-agent
sleep 6
test "$(node -p 'require("/tmp/fixture-summary.json").accepted')" -gt "$accepted"
echo 'PASS: inline, EnvironmentFile and private secret reference under systemd'
touch /tmp/fixture-unavailable
sleep 10
systemctl stop observe-agent
test "$(systemctl is-active observe-agent)" = inactive
test "$(find /var/lib/observe-agent/metrics -name '*.rec' | wc -l)" -gt 0
before=$(sha256sum /etc/observe-agent/agent.yaml | cut -d' ' -f1)
find /var/lib/observe-agent/metrics -type f -name '*.rec' -exec sha256sum '{}' \; > /tmp/retained-checksums
old_uid=$(id -u observe-agent)
dpkg --remove observe-agent
test -f /etc/observe-agent/agent.yaml
sha256sum --check /tmp/retained-checksums >/dev/null
dpkg -i "$deb"
test "$(sha256sum /etc/observe-agent/agent.yaml | cut -d' ' -f1)" = "$before"
test "$(id -u observe-agent)" = "$old_uid"
sha256sum --check /tmp/retained-checksums >/dev/null
test "$(systemctl is-active observe-agent)" = inactive
rm /tmp/fixture-unavailable
systemctl start observe-agent
sleep 10
systemctl restart observe-agent
sleep 6
systemctl is-active --quiet observe-agent
node -e 'const s=require("/tmp/fixture-summary.json");if(s.accepted<3||s.secretInPayload)process.exit(1);console.log("PASS: restart/replay",{accepted:s.accepted,points:s.points})'
systemctl stop observe-agent
journalctl -u observe-agent --no-pager > /tmp/observe-test-journal
test -s /tmp/observe-test-journal
if grep -Eq 'package-test-key-not-real|ignored-inline-fixture' /tmp/observe-test-journal;then echo 'secret leaked to journal';exit 1;fi
grep -q 'delivered_records' /tmp/observe-test-journal
test -z "$(find /var/lib/observe-agent/metrics -type f ! -perm 0600 -print)"
echo 'PASS: stop/restart/status/journal; config/state/account survive remove/reinstall; no secret in journal'
# Explicit purge removes conffile but never the spool/account.
dpkg --purge observe-agent
test ! -e /etc/observe-agent/agent.yaml
test -d /var/lib/observe-agent/metrics
test "$(id -u observe-agent)" = "$old_uid"
echo 'PASS: explicit purge retains state/account and removes dpkg YAML conffile'
# AGENTV1 FILE END
