# AGENTV1 FILE START: native Go cross-build; DEB assembly in an isolated Linux container.
[CmdletBinding()]
param([Parameter(Mandatory)][string]$Tag, [string]$BuilderImage='node:22-bookworm')
$ErrorActionPreference='Stop'
if($Tag -cnotmatch '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-canary\.[0-9A-Za-z.]+)?$'){throw 'Invalid release tag'}
$repo=Split-Path $PSScriptRoot -Parent
$version=$Tag.Substring(1)
$debVersion=$version.Replace('-canary.','~canary.')
$bin=Join-Path $repo "dist/bin/$Tag"
$packages=Join-Path $repo "dist/packages/$Tag"
$checksums=Join-Path $repo "dist/checksums/$Tag"
foreach($tool in @('go','docker')){Get-Command $tool -ErrorAction Stop | Out-Null}
foreach($path in @($bin,$packages,$checksums)){if(Test-Path -LiteralPath $path){throw "Output already exists: $path; use a new version or review that generated output"}}
foreach($path in @($bin,$packages,$checksums)){[IO.Directory]::CreateDirectory($path) | Out-Null}
$saved=@{}; foreach($key in @('GOOS','GOARCH','CGO_ENABLED')){$saved[$key]=[Environment]::GetEnvironmentVariable($key,'Process')}
Push-Location $repo
try {
    $env:CGO_ENABLED='0'
    foreach($target in @('linux/amd64','linux/arm64','windows/amd64')){
        $env:GOOS,$env:GOARCH=$target.Split('/')
        $ext=if($env:GOOS -eq 'windows'){'.exe'}else{''}
        $dir=Join-Path $bin ($target.Replace('/','_'))
        [IO.Directory]::CreateDirectory($dir) | Out-Null
        & go build -trimpath -ldflags "-X github.com/agent-i/agent/internal/version.Version=$version" -o (Join-Path $dir "observe-agent$ext") ./cmd/observe-agent
        if($LASTEXITCODE -ne 0){throw "Go build failed: $target"}
    }
    & docker run --rm --network none --mount "type=bind,source=$repo,target=/src,readonly" --mount "type=bind,source=$packages,target=/out" -w /src --entrypoint sh $BuilderImage packaging/deb/build.sh $debVersion "/src/dist/bin/$Tag/linux_amd64/observe-agent" /out
    if($LASTEXITCODE -ne 0){throw 'DEB assembly failed'}
    $asset="observe-agent_${version}_amd64.deb"
    if($debVersion -ne $version){Move-Item -LiteralPath (Join-Path $packages "observe-agent_${debVersion}_amd64.deb") -Destination (Join-Path $packages $asset)}
    Copy-Item -LiteralPath (Join-Path $packages $asset) -Destination (Join-Path $packages 'observe-agent_linux_amd64.deb')
    & "$PSScriptRoot/checksums.ps1" -Assets $packages -Output (Join-Path $checksums 'SHA256SUMS')
    Copy-Item -LiteralPath (Join-Path $checksums 'SHA256SUMS') -Destination (Join-Path $packages 'SHA256SUMS')
    Write-Output "Built $Tag. Customer package: Linux AMD64 DEB. ARM64/Windows binaries: compile-validation only."
} finally {
    Pop-Location
    foreach($key in $saved.Keys){[Environment]::SetEnvironmentVariable($key,$saved[$key],'Process')}
}
# AGENTV1 FILE END
