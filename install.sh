#!/usr/bin/env bash
# AGENTV1 FILE START: self-contained HTTPS/SHA256 bootstrap for validated DEB hosts only.
# Root entry point intentionally self-contained for curl | bash; no second script is fetched.
set -euo pipefail
fail() { printf 'Observe Agent: %s\n' "$*" >&2; return 1; }
valid_tag() { [[ $1 =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-canary\.[0-9A-Za-z.]+)?$ ]]; }
select_platform() {
  local os=$1 distro=$2 arch=$3
  [[ $os == Linux ]] || { fail 'Linux required; Windows/macOS installers are not available'; return 1; }
  case "$distro" in debian|ubuntu) :;; *) fail "Unsupported distribution: $distro (RPM installation is not yet implemented)";return 1;; esac
  case "$arch" in x86_64|amd64) printf 'amd64\n';; aarch64|arm64) fail 'ARM64 compiles, but its package has not been validated';return 1;; *) fail "Unsupported architecture: $arch";return 1;; esac
}
verify_asset() {
  local directory=$1 asset=$2 entry digest actual count
  # Require exactly one entry; never feed untrusted paths from a manifest to sha256sum -c.
  count=$(awk -v name="$asset" '$2 == name {n++} END {print n+0}' "$directory/SHA256SUMS")
  [[ $count == 1 ]] || { fail 'Missing or duplicate checksum entry'; return 1; }
  entry=$(awk -v name="$asset" '$2 == name {print $1}' "$directory/SHA256SUMS")
  [[ $entry =~ ^[0-9a-fA-F]{64}$ ]] || { fail 'Invalid checksum entry'; return 1; }
  actual=$(sha256sum -- "$directory/$asset"); digest=${actual%% *}
  [[ ${entry,,} == "$digest" ]] || { fail 'Checksum mismatch; package NOT installed'; return 1; }
}
install_main() {
  local tag='' dry=false arg distro arch effective version deb_version base asset work installed
  while (($#)); do
    arg=$1; shift
    case "$arg" in
      --version) (($#)) || { fail '--version needs a v-prefixed tag'; return 1; }; tag=$1;shift;;
      --dry-run) dry=true;;
      --help) echo 'Usage: bash install.sh [--version vX.Y.Z-canary.SUFFIX] [--dry-run]. No key is requested. Only Debian/Ubuntu AMD64 is supported.';return 0;;
      *) fail "Unknown argument: $arg";return 1;;
    esac
  done
  [[ -z $tag ]] || valid_tag "$tag" || { fail 'Invalid release tag'; return 1; }
  [[ -r /etc/os-release ]] || { fail 'Cannot identify distribution'; return 1; }
  distro=$(sed -n 's/^ID=//p' /etc/os-release | tr -d '\"')
  arch=$(select_platform "$(uname -s)" "$distro" "$(uname -m)") || return 1
  for arg in curl sha256sum dpkg dpkg-deb dpkg-query; do command -v "$arg" >/dev/null || { fail "Required tool missing: $arg"; return 1; }; done
  base=https://github.com/vinod-cs/observe-agent/releases
  if [[ -z $tag ]]; then
    effective=$(curl --proto '=https' --proto-redir '=https' --tlsv1.2 --fail --silent --show-error --location --connect-timeout 10 --max-time 30 --output /dev/null --write-out '%{url_effective}' "$base/latest") || { fail 'No published latest release; canaries require --version <tag>'; return 1; }
    [[ $effective == "$base/tag/"* ]] || { fail 'No latest release; select an explicit published canary tag';return 1; }
    tag=${effective#"$base/tag/"}
    valid_tag "$tag" || { fail 'Unexpected latest release tag';return 1; }
  fi
  version=${tag#v}; deb_version=${version/-canary./~canary.}
  asset="observe-agent_${version}_${arch}.deb"
  if $dry; then printf 'Would verify SHA256SUMS and install %s/download/%s/%s; no package downloaded or installed.\n' "$base" "$tag" "$asset";return 0;fi
  [[ $EUID == 0 ]] || { fail 'Run with sudo after reviewing the installer'; return 1; }
  [[ -d /run/systemd/system ]] || { fail 'A running systemd environment is required';return 1; }
  # Never downgrade or silently replace unsupported package identities.
  installed=$(dpkg-query -W -f='${Version}' observe-agent 2>/dev/null || true)
  if [[ -n $installed ]] && dpkg --compare-versions "$deb_version" lt "$installed"; then fail 'Refusing package downgrade';return 1;fi
  umask 077
  work=$(mktemp -d /tmp/observe-agent-install.XXXXXX)
  # Remove only this installer-owned, exact mktemp directory on exit.
  trap "rm -r -- '$work'" EXIT
  for arg in SHA256SUMS "$asset"; do
    curl --proto '=https' --proto-redir '=https' --tlsv1.2 --fail --silent --show-error --location --connect-timeout 10 --max-time 180 --retry 2 --max-filesize 157286400 --output "$work/$arg" "$base/download/$tag/$arg" || { fail 'Download failed; nothing installed';return 1; }
  done
  [[ $(wc -c < "$work/SHA256SUMS") -le 65536 ]] || { fail 'Checksum manifest too large';return 1; }
  verify_asset "$work" "$asset" || return 1
  [[ $(dpkg-deb -f "$work/$asset" Package) == observe-agent && $(dpkg-deb -f "$work/$asset" Architecture) == amd64 && $(dpkg-deb -f "$work/$asset" Version) == "$deb_version" ]] || { fail 'Package metadata does not match requested release';return 1; }
  # Preserve local conffiles; no apt install, credential editing, enabling or starting.
  dpkg --force-confdef --force-confold -i "$work/$asset"
  rm -r -- "$work"
  trap - EXIT
  printf '%s\n' 'Installed without starting/enabling collection. Existing config/state retained.' 'Configure: sudoedit /etc/observe-agent/agent.yaml' 'Validate: sudo observe-agent --check --config /etc/observe-agent/agent.yaml' 'Start: sudo systemctl start observe-agent' 'Status: sudo systemctl status observe-agent' 'Journal: sudo journalctl -u observe-agent' 'Environment-reference mode: ExecStartPre validates in the service environment.'
}
if [[ ${BASH_SOURCE[0]:-} == "$0" || -z ${BASH_SOURCE[0]:-} ]]; then install_main "$@"; fi
# AGENTV1 FILE END
