#!/usr/bin/env bash
# AGENTV1 FILE START: actual in-place dpkg upgrade; isolated systemd environment only.
set -euo pipefail
[[ ${OBSERVE_DEB_TEST_ONLY:-} == yes && $(cat /proc/1/comm) == systemd ]]
[[ ! -e /usr/local/bin/agent-i ]]
old=${1:?older test DEB}; new=${2:?newer test DEB}
dpkg --compare-versions "$(dpkg-deb -f "$new" Version)" gt "$(dpkg-deb -f "$old" Version)"
dpkg -i "$old"
printf 'observe:\n  endpoint: https://127.0.0.1:8443/api/v1/otlp\n  api_key: upgrade-test-not-real\ncollection:\n  interval_seconds: 5\n  metrics:\n    enabled: true\n  logs:\n    enabled: false\n  traces:\n    enabled: false\nec2_metadata:\n  enabled: false\n' > /etc/observe-agent/agent.yaml
chmod 0640 /etc/observe-agent/agent.yaml
# AGENTV1 START: exercise real legacy binary when supplied, otherwise current package fixture.
sed -i 's/8443/8444/;s/upgrade-test-not-real/package-test-key-not-real/' /etc/observe-agent/agent.yaml
if [[ ${OBSERVE_UPGRADE_V1:-no} != yes ]]; then
  sed -i '/^observe:/a\  backend_id: fixture-backend\n  organization_id: fixture-org' /etc/observe-agent/agent.yaml
fi
# AGENTV1 END: legacy/current fixture selection
before=$(sha256sum /etc/observe-agent/agent.yaml)
uid=$(id -u observe-agent)
systemctl start observe-agent
sleep 6
systemctl is-active --quiet observe-agent
# Quiesce for a precise checksum baseline, then let prerm handle an active process.
systemctl stop observe-agent
find /var/lib/observe-agent/metrics -name '*.rec' -exec sha256sum '{}' \; > /tmp/observe-upgrade-records
[[ -s /tmp/observe-upgrade-records ]]
systemctl start observe-agent
dpkg --force-confdef --force-confold -i "$new"
[[ $(systemctl is-active observe-agent) == inactive ]]
[[ $(sha256sum /etc/observe-agent/agent.yaml) == "$before" && $(id -u observe-agent) == "$uid" ]]
sha256sum --check /tmp/observe-upgrade-records
[[ $(stat -c '%a %U:%G' /etc/observe-agent/agent.yaml) == '640 root:observe-agent' ]]
# AGENTV1 START: one-time v1 binding then actual TLS replay after transport change.
if [[ ${OBSERVE_UPGRADE_V1:-no} == yes ]]; then
  if observe-agent --check --config /etc/observe-agent/agent.yaml; then echo 'Unbound v1 config accepted'; exit 1; fi
  sed -i '/^observe:/a\  backend_id: fixture-backend\n  organization_id: fixture-org\n  previous_endpoint: https://127.0.0.1:8444/api/v1/otlp' /etc/observe-agent/agent.yaml
fi
sed -i 's@endpoint: https://127.0.0.1:8444/@endpoint: https://127.0.0.1:8443/@' /etc/observe-agent/agent.yaml
# Keep the migration-only original endpoint exact (the sed above also matches its suffix).
sed -i 's@previous_endpoint:.*@previous_endpoint: https://127.0.0.1:8444/api/v1/otlp@' /etc/observe-agent/agent.yaml
observe-agent --check --config /etc/observe-agent/agent.yaml
systemctl start observe-agent
sleep 2
systemctl is-active --quiet observe-agent
systemctl stop observe-agent
sha256sum --check /tmp/observe-upgrade-records
node -e 'if(require("/var/lib/observe-agent/metrics/manifest.json").Version!==2)process.exit(1)'
sed -i '/previous_endpoint:/d' /etc/observe-agent/agent.yaml
openssl req -x509 -newkey rsa:2048 -nodes -days 1 -subj /CN=localhost -addext 'subjectAltName=IP:127.0.0.1' -keyout /tmp/fixture.key -out /tmp/fixture.crt >/dev/null 2>&1
chmod 0600 /tmp/fixture.key
install -m 0644 /tmp/fixture.crt /usr/local/share/ca-certificates/observe-upgrade-test.crt
update-ca-certificates >/dev/null 2>&1
node /opt/deb-test/fixture.cjs >/tmp/fixture-output 2>&1 & fixture_pid=$!
trap 'kill "$fixture_pid" 2>/dev/null || true' EXIT
systemctl start observe-agent
sleep 12
systemctl is-active --quiet observe-agent
systemctl stop observe-agent
node -e 'const s=require("/tmp/fixture-summary.json");if(s.accepted<1||s.points<1||s.secretInPayload)process.exit(1)'
while read -r checksum record; do [[ ! -f $record ]]; done < /tmp/observe-upgrade-records
echo 'PASS migrated/upgraded pending queue replayed to changed HTTPS endpoint; no previous_endpoint needed on restart'
# AGENTV1 END: TLS replay
dpkg --purge observe-agent
echo 'PASS true older-to-newer upgrade: service stopped, config/UID/queued records retained, no autostart'
# AGENTV1 FILE END
