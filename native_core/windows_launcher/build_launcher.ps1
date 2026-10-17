param(
  [Parameter(Mandatory=$true)][string]$BundleRoot,
  [string]$GoExe = (Join-Path $PSScriptRoot '..\..\.tools\next\go\bin\go.exe'),
  [string]$OutputExe = (Join-Path $PSScriptRoot '..\..\releases\YingXia_v2.0.0_Native_Windows_x64.exe')
)
$ErrorActionPreference = 'Stop'
$BundleRoot = [IO.Path]::GetFullPath($BundleRoot)
$OutputExe = [IO.Path]::GetFullPath($OutputExe)
$payloadRoot = Join-Path $PSScriptRoot 'payload'
$archivePath = Join-Path $payloadRoot 'bundle.zip'
if (Test-Path -LiteralPath $OutputExe) { throw 'Preserve existing published artifacts; choose a new output filename.' }
$files = @(Get-ChildItem -LiteralPath $BundleRoot -Recurse -File | Where-Object { $_.Extension -ne '.pdb' } | Sort-Object FullName)
$manifestFiles = @()
$stream = [IO.File]::Open($archivePath,[IO.FileMode]::Create)
$archive = [IO.Compression.ZipArchive]::new($stream,[IO.Compression.ZipArchiveMode]::Create)
try {
  foreach ($file in $files) {
    $relative = [IO.Path]::GetRelativePath($BundleRoot,$file.FullName).Replace('\','/')
    if ($relative.StartsWith('../') -or $relative.Contains(':')) { throw 'Bundle path escapes input directory.' }
    $manifestFiles += [ordered]@{path=$relative;size=$file.Length;sha256=(Get-FileHash -Algorithm SHA256 -LiteralPath $file.FullName).Hash.ToLowerInvariant()}
    $entry = $archive.CreateEntry($relative,[IO.Compression.CompressionLevel]::Optimal)
    $entry.LastWriteTime = [DateTimeOffset]::new(2026,10,14,0,0,0,[TimeSpan]::Zero)
    $entryStream = $entry.Open(); $input = [IO.File]::OpenRead($file.FullName)
    try { $input.CopyTo($entryStream) } finally { $input.Dispose();$entryStream.Dispose() }
  }
} finally { $archive.Dispose();$stream.Dispose() }
$bundleSHA = (Get-FileHash -Algorithm SHA256 -LiteralPath $archivePath).Hash.ToLowerInvariant()
$manifest = [ordered]@{version='2.0.0-native-windows';bundle_sha256=$bundleSHA;files=$manifestFiles}
[IO.File]::WriteAllText((Join-Path $payloadRoot 'manifest.json'),($manifest | ConvertTo-Json -Depth 6),[Text.UTF8Encoding]::new($false))
Push-Location $PSScriptRoot
try {
  $env:CGO_ENABLED='0';$env:GOTOOLCHAIN='local';$env:GOPROXY='off';$env:GOSUMDB='off';$env:GOOS='windows';$env:GOARCH='amd64'
  & $GoExe build -trimpath '-ldflags=-H windowsgui -s -w' -o $OutputExe .
  if ($LASTEXITCODE -ne 0) { throw 'Native launcher build failed.' }
} finally { Pop-Location }
[ordered]@{artifact=[IO.Path]::GetFileName($OutputExe);size=(Get-Item -LiteralPath $OutputExe).Length;sha256=(Get-FileHash -Algorithm SHA256 -LiteralPath $OutputExe).Hash.ToLowerInvariant();bundle_sha256=$bundleSHA;bundle_files=$files.Count} | ConvertTo-Json
