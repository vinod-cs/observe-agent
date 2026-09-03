#!/usr/bin/env bash
# AGENTV1 FILE START: local cross-build and supported-package assembly; never publishes.
set -euo pipefail
cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.."
tag=${1:?usage: build-release.sh vX.Y.Z[-canary.SUFFIX]}
[[ $tag =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-canary\.[0-9A-Za-z.]+)?$ ]] || { echo 'Invalid release tag' >&2; exit 1; }
version=${tag#v}
deb_version=${version/-canary./~canary.}
for tool in go dpkg-deb sha256sum; do command -v "$tool" >/dev/null || { echo "Missing build tool: $tool" >&2; exit 1; }; done
bin="dist/bin/$tag"
packages="dist/packages/$tag"
checksums="dist/checksums/$tag"
for path in "$bin" "$packages" "$checksums"; do
  [[ ! -e $path ]] || { echo "Output already exists: $path; use a new version or review/remove only that generated output" >&2; exit 1; }
done
mkdir -p "$bin" "$packages" "$checksums"
export CGO_ENABLED=0
for target in linux/amd64 linux/arm64 windows/amd64; do
  os=${target%/*}; arch=${target#*/}; ext=''; [[ $os != windows ]] || ext=.exe
  mkdir -p "$bin/${os}_${arch}"
  GOOS=$os GOARCH=$arch go build -trimpath -ldflags "-X github.com/agent-i/agent/internal/version.Version=$version" -o "$bin/${os}_${arch}/observe-agent$ext" ./cmd/observe-agent
done
# Keep the validated DEB builder, maintainer scripts, service and paths unchanged.
sh packaging/deb/build.sh "$deb_version" "$bin/linux_amd64/observe-agent" "$packages"
asset="observe-agent_${version}_amd64.deb"
if [[ $deb_version != "$version" ]]; then mv -- "$packages/observe-agent_${deb_version}_amd64.deb" "$packages/$asset"; fi
# Stable alias supports latest/download after a future release is promoted from prerelease.
cp -- "$packages/$asset" "$packages/observe-agent_linux_amd64.deb"
bash scripts/checksums.sh "$packages" "$checksums/SHA256SUMS"
cp -- "$checksums/SHA256SUMS" "$packages/SHA256SUMS"
echo "Built $tag: Linux AMD64 DEB only. ARM64/Windows binaries are compile-validation outputs, not supported installers."
# AGENTV1 FILE END
