# AGENTV1 FILE START: native Windows manifests use UTF-8 without BOM and LF.
[CmdletBinding()]
param([Parameter(Mandatory)][string]$Assets, [Parameter(Mandatory)][string]$Output)
$ErrorActionPreference='Stop'
$files=@(Get-ChildItem -LiteralPath $Assets -File | Where-Object { $_.Name -match '\.(deb|rpm|tar\.gz|zip|msi)$' } | Sort-Object Name)
if (!$files.Count) { throw 'No release assets' }
$lines=foreach($f in $files){
    if($f.Name -notmatch '^observe-agent[-_][A-Za-z0-9._~+\-]+$'){throw 'Unsafe asset name'}
    '{0}  {1}' -f (Get-FileHash -LiteralPath $f.FullName -Algorithm SHA256).Hash.ToLowerInvariant(),$f.Name
}
$path=[IO.Path]::GetFullPath($Output)
[IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($path)) | Out-Null
[IO.File]::WriteAllText($path,($lines -join "`n")+"`n",[Text.UTF8Encoding]::new($false))
# AGENTV1 FILE END
