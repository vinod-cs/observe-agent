# AGENTV1 FILE START: PowerShell syntax, checksum parity and unsupported installer safety.
$ErrorActionPreference='Stop'
$repo=Split-Path (Split-Path $PSScriptRoot -Parent) -Parent
foreach($file in @(Get-ChildItem "$repo/scripts/*.ps1","$repo/installers/*.ps1")){
    $errors=$null;$tokens=$null
    [Management.Automation.Language.Parser]::ParseFile($file.FullName,[ref]$tokens,[ref]$errors) | Out-Null
    if($errors.Count){throw "PowerShell parse errors: $($file.Name)"}
}
try { & "$repo/installers/install.ps1" -Version 'v0.1.0-canary.test';throw 'Installer unexpectedly succeeded' }
catch { if($_.Exception.Message -notlike 'Observe Agent Windows installation is not implemented*'){throw} }
$dir=Join-Path $repo ('dist/checksums/test-'+[guid]::NewGuid().ToString('N'))
[IO.Directory]::CreateDirectory($dir) | Out-Null
$asset=Join-Path $dir 'observe-agent_test_amd64.deb'
[IO.File]::WriteAllText($asset,'fixture, not a package',[Text.UTF8Encoding]::new($false))
& "$repo/scripts/checksums.ps1" -Assets $dir -Output (Join-Path $dir 'SHA256SUMS')
$expected=(Get-FileHash $asset -Algorithm SHA256).Hash.ToLowerInvariant()+'  observe-agent_test_amd64.deb'+"`n"
if([IO.File]::ReadAllText((Join-Path $dir 'SHA256SUMS')) -cne $expected){throw 'Checksum content mismatch'}
$bytes=[IO.File]::ReadAllBytes((Join-Path $dir 'SHA256SUMS'))
if($bytes[0] -eq 239 -or $bytes -contains 13){throw 'Manifest BOM/CRLF regression'}
Write-Output 'PASS PowerShell parser, checksum format and fail-closed Windows installer'
# AGENTV1 FILE END
