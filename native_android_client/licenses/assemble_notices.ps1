# Refresh the generated package section, preserving maintained Go/FSR provenance,
# then concatenate the original license texts for the shipping Windows runtime.
$ErrorActionPreference = 'Stop'
$project = Split-Path -Parent $PSScriptRoot
& (Join-Path $PSScriptRoot 'collect_licenses.ps1') -ProjectDirectory $project | Out-Null
$manifest = Get-Content -Raw -LiteralPath (Join-Path $PSScriptRoot 'pub-license-manifest.json') | ConvertFrom-Json
$notices = [IO.File]::ReadAllText((Join-Path $project 'THIRD_PARTY_NOTICES.md'))
$runtime = Join-Path $PSScriptRoot 'runtime'
New-Item -ItemType Directory -Path $runtime -Force | Out-Null
$text = [Text.StringBuilder]::new()
[void]$text.AppendLine($notices)
$all = @(Get-ChildItem -LiteralPath (Join-Path $PSScriptRoot 'pub') -Recurse -File)
$all += @(Get-ChildItem -LiteralPath (Join-Path $PSScriptRoot 'flutter') -File | Where-Object Extension -NE '.json')
$all += @(Get-ChildItem -LiteralPath (Join-Path $PSScriptRoot 'native') -File | Where-Object Name -Match '^(LICENSE|COPYING|COPYRIGHT|AUTHORS|Microsoft-VC-Runtime-License).*\.txt$')
$all += @(Get-ChildItem -LiteralPath (Join-Path $PSScriptRoot 'go') -Recurse -File | Where-Object Extension -NE '.json')
$all += @(Get-ChildItem -LiteralPath (Join-Path $PSScriptRoot 'fsr1') -File | Where-Object Extension -EQ '.txt')
foreach ($file in ($all | Sort-Object FullName -Unique)) {
  [void]$text.AppendLine()
  [void]$text.AppendLine('================================================================')
  [void]$text.AppendLine([IO.Path]::GetRelativePath($project, $file.FullName).Replace('\','/'))
  [void]$text.AppendLine('================================================================')
  [void]$text.AppendLine([IO.File]::ReadAllText($file.FullName))
}
[IO.File]::WriteAllText((Join-Path $runtime 'THIRD_PARTY_NOTICES.txt'), $text.ToString(), [Text.UTF8Encoding]::new($false))
[pscustomobject]@{packages=$manifest.package_count;source_archives=5;runtime_notice_bytes=(Get-Item -LiteralPath (Join-Path $runtime 'THIRD_PARTY_NOTICES.txt')).Length;pubspec_lock_sha256=$manifest.pubspec_lock_sha256} | ConvertTo-Json