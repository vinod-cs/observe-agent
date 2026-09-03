#!/usr/bin/env bash
# AGENTV1 FILE START: local fixture-only installer platform and integrity tests.
set -euo pipefail
cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.."
source ./install.sh
valid_tag v0.1.0-canary.20260903.1
for bad in '../x' 'v1.0.0;id' 'v01.0.0' '--help'; do if valid_tag "$bad";then echo 'unsafe tag accepted';exit 1;fi;done
[[ $(select_platform Linux debian x86_64) == amd64 ]]
[[ $(select_platform Linux ubuntu amd64) == amd64 ]]
for args in 'Linux ubuntu arm64' 'Linux fedora x86_64' 'Darwin ubuntu arm64' 'Linux ubuntu mips'; do
  # intentional test argument splitting, never from user/network data.
  if select_platform $args >/dev/null 2>&1;then echo 'unsupported platform accepted';exit 1;fi
done
dir=$(mktemp -d /tmp/observe-installer-test.XXXXXX)
trap 'rm -r -- "$dir"' EXIT
asset=observe-agent_0.1.0-canary.test_amd64.deb
printf 'fixture, not a package' > "$dir/$asset"
bash scripts/checksums.sh "$dir" "$dir/SHA256SUMS"
verify_asset "$dir" "$asset"
cp "$dir/SHA256SUMS" "$dir/good"
printf 'corruption' >> "$dir/$asset"
if verify_asset "$dir" "$asset" 2>/dev/null;then echo 'corruption accepted';exit 1;fi
cat "$dir/good" "$dir/good" > "$dir/SHA256SUMS"
if verify_asset "$dir" "$asset" 2>/dev/null;then echo 'duplicate accepted';exit 1;fi
printf '%064d  ../../etc/passwd\n' 0 > "$dir/SHA256SUMS"
if verify_asset "$dir" "$asset" 2>/dev/null;then echo 'wrong path accepted';exit 1;fi
[[ $(bash install.sh --version v0.1.0-canary.test --dry-run) == *'no package downloaded or installed'* ]]
[[ $(bash installers/install.sh --help) == *'Only Debian/Ubuntu AMD64'* ]]
for f in install.sh installers/install.sh scripts/*.sh packaging/deb/build.sh;do bash -n "$f";done
echo 'PASS installer platform/tag/integrity/entry-point tests; no network or installation'
# AGENTV1 FILE END
