#!/bin/sh
# AGENTV1 FILE START: local canary DEB assembly, not publication or installation.
set -eu
version=${1:?version required}
binary=${2:?Linux amd64 binary path required}
out=${3:?output directory required}
case "$version" in *[!0-9A-Za-z.+~:-]*|'') echo 'invalid version' >&2;exit 1;; esac
test -f "$binary"
mkdir -p "$out"
stage=$(mktemp -d /tmp/observe-agent-deb.XXXXXX)
trap 'rm -r -- "$stage"' EXIT
# mktemp is private; the archive root represents / and must not carry mode 0700.
chmod 0755 "$stage"
install -d "$stage/DEBIAN" "$stage/usr/bin" "$stage/etc/observe-agent" "$stage/lib/systemd/system" "$stage/usr/share/doc/observe-agent"
install -m 0755 "$binary" "$stage/usr/bin/observe-agent"
install -m 0600 packaging/deb/agent.yaml "$stage/etc/observe-agent/agent.yaml"
install -m 0644 packaging/deb/observe-agent.service "$stage/lib/systemd/system/observe-agent.service"
for script in postinst prerm postrm; do install -m 0755 "packaging/deb/$script" "$stage/DEBIAN/$script"; done
printf '/etc/observe-agent/agent.yaml\n' > "$stage/DEBIAN/conffiles"
printf 'Package: observe-agent\nVersion: %s\nArchitecture: amd64\nMaintainer: Observe Agent Canary <canary@example.invalid>\nDepends: passwd, ca-certificates, systemd\nSection: admin\nPriority: optional\nDescription: Observe Linux host metrics and opt-in file logs canary (not a production release)\n' "$version" > "$stage/DEBIAN/control"
install -m 0644 packaging/deb/README.md "$stage/usr/share/doc/observe-agent/README.md"
dpkg-deb --root-owner-group -Zgzip -z6 --build "$stage" "$out/observe-agent_${version}_amd64.deb"
sha256sum "$out/observe-agent_${version}_amd64.deb"
# AGENTV1 FILE END
