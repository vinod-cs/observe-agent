#!/usr/bin/env bash
# AGENTV1 FILE START: complete bootstrap with local download fixtures in disposable systemd VM.
set -euo pipefail
[[ ${OBSERVE_DEB_TEST_ONLY:-} == yes && $(cat /proc/1/comm) == systemd ]]
[[ ! -e /usr/local/bin/agent-i ]]
tag=${1:?test release tag}
root=${2:?checkout or copied test source}
fixtures=${3:?copied release assets}
# /tmp may be noexec in a hardened systemd test container; this directory holds a stub executable.
work=$(mktemp -d /opt/observe-bootstrap-test.XXXXXX)
trap 'rm -r -- "$work"' EXIT
mkdir "$work/bin" "$work/assets"
cp "$fixtures/"* "$work/assets/"
# No network: downloader stub accepts only the fixed repository and exact tag paths.
cat > "$work/bin/curl" <<'FIXTURE'
#!/bin/bash
set -euo pipefail
output='';url=''
while (($#));do case "$1" in --output) output=$2;shift 2;; https://*) url=$1;shift;; *) shift;;esac;done
prefix="https://github.com/vinod-cs/observe-agent/releases/download/$FIXTURE_TAG/"
[[ $url == "$prefix"* ]]
asset=${url#"$prefix"}
[[ $asset != */* && -f $FIXTURE_ASSETS/$asset && -n $output ]]
cp "$FIXTURE_ASSETS/$asset" "$output"
FIXTURE
chmod 0755 "$work/bin/curl"
export FIXTURE_TAG=$tag FIXTURE_ASSETS="$work/assets"
realpath=$PATH
export PATH="$work/bin:$PATH"
[[ $(command -v curl) == "$work/bin/curl" ]]
bash "$root/install.sh" --version "$tag"
expected=${tag#v}; expected=${expected/-canary./~canary.}
[[ $(dpkg-query -W -f='${Version}' observe-agent) == "$expected" ]]
[[ $(systemctl is-active observe-agent) == inactive ]]
[[ $(systemctl is-enabled observe-agent) == disabled ]]
printf '\n# retained bootstrap config\n' >> /etc/observe-agent/agent.yaml
before=$(sha256sum /etc/observe-agent/agent.yaml)
bash "$root/install.sh" --version "$tag"
[[ $(sha256sum /etc/observe-agent/agent.yaml) == "$before" ]]
printf 'tampered' >> "$work/assets/observe-agent_${tag#v}_amd64.deb"
if bash "$root/install.sh" --version "$tag";then echo 'Tampered bootstrap succeeded';exit 1;fi
[[ $(sha256sum /etc/observe-agent/agent.yaml) == "$before" ]]
cp "$fixtures/observe-agent_${tag#v}_amd64.deb" "$work/assets/observe-agent_${tag#v}_amd64.deb"
# A matching hash is not sufficient: reject a valid DEB with the wrong requested version.
if [[ -f /opt/old.deb ]]; then
  cp /opt/old.deb "$work/assets/observe-agent_${tag#v}_amd64.deb"
  (cd "$work/assets"; sha256sum "observe-agent_${tag#v}_amd64.deb") > "$work/assets/SHA256SUMS"
  if bash "$root/install.sh" --version "$tag";then echo 'Wrong-version package accepted';exit 1;fi
  [[ $(dpkg-query -W -f='${Version}' observe-agent) == "$expected" ]]
fi
export PATH=$realpath
dpkg --purge observe-agent
echo 'PASS full bootstrap: local download, checksum/metadata, conffile retention, tamper rejection, no autostart'
# AGENTV1 FILE END
