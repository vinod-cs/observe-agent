#!/usr/bin/env bash
# AGENTV1 FILE START: checksum manifest for flat, known release assets only.
set -euo pipefail
assets=$(cd -- "${1:?asset directory}" && pwd)
output=${2:?manifest output path}
mkdir -p -- "$(dirname -- "$output")"
output=$(cd -- "$(dirname -- "$output")" && pwd)/$(basename -- "$output")
mapfile -t names < <(find "$assets" -maxdepth 1 -type f \( -name '*.deb' -o -name '*.rpm' -o -name '*.tar.gz' -o -name '*.zip' -o -name '*.msi' \) -printf '%f\n' | LC_ALL=C sort)
((${#names[@]} > 0)) || { echo 'No release assets' >&2; exit 1; }
for name in "${names[@]}"; do [[ $name =~ ^observe-agent[-_][A-Za-z0-9._~+-]+$ ]] || { echo 'Unsafe asset name' >&2;exit 1; }; done
(cd -- "$assets"; sha256sum -- "${names[@]}") > "$output"
# AGENTV1 FILE END
