# Re-create package notices from the exact packages resolved by pubspec.lock.
# Does not delete or modify the dependency cache or the application source.
param([string]$ProjectDirectory = (Split-Path -Parent $PSScriptRoot))
$ErrorActionPreference = 'Stop'
$ProjectDirectory = [IO.Path]::GetFullPath($ProjectDirectory)
$destinationRoot = Join-Path $ProjectDirectory 'licenses'
$configuration = Get-Content -Raw -LiteralPath (Join-Path $ProjectDirectory '.dart_tool/package_config.json') | ConvertFrom-Json
$configurationDirectory = Join-Path $ProjectDirectory '.dart_tool'
$frameworkDirectory = ([Uri]$configuration.flutterRoot).LocalPath
$versions = @{}
$lockLines = Get-Content -LiteralPath (Join-Path $ProjectDirectory 'pubspec.lock')
$packageName = ''
foreach ($line in $lockLines) {
  if ($line -match '^  ([A-Za-z0-9_]+):$') { $packageName = $Matches[1] }
  if ($packageName -and $line -match '^    version: "([^"]+)"$') { $versions[$packageName] = $Matches[1] }
}
$manifest = @()
foreach ($package in $configuration.packages) {
  if ($package.name -eq 'yingxia_native') { continue }
  $packageDirectory = if ($package.rootUri -match '^file:') {
    ([Uri]$package.rootUri).LocalPath
  } else {
    [IO.Path]::GetFullPath((Join-Path $configurationDirectory $package.rootUri))
  }
  $version = $versions[$package.name]
  $packageLicenses = @(Get-ChildItem -LiteralPath $packageDirectory -File | Where-Object { $_.Name -match '^(license|licence|copying|notice)([.-].*)?$' })
  $inherited = $false
  if ($packageLicenses.Count -eq 0 -and $packageDirectory.StartsWith($frameworkDirectory, [StringComparison]::OrdinalIgnoreCase)) {
    $packageLicenses = @(Get-Item -LiteralPath (Join-Path $frameworkDirectory 'LICENSE'))
    $inherited = $true
  }
  if ($packageLicenses.Count -eq 0) { throw "Missing license for $($package.name) $version" }
  $targetDirectory = Join-Path $destinationRoot "pub/$($package.name)-$version"
  New-Item -ItemType Directory -Path $targetDirectory -Force | Out-Null
  foreach ($license in $packageLicenses) {
    $target = Join-Path $targetDirectory $license.Name
    Copy-Item -LiteralPath $license.FullName -Destination $target -Force
    $relative = [IO.Path]::GetRelativePath($ProjectDirectory, $target).Replace('\', '/')
    $manifest += [pscustomobject]@{
      package = $package.name
      version = $version
      file = $relative
      sha256 = (Get-FileHash -LiteralPath $target -Algorithm SHA256).Hash.ToLowerInvariant()
      inherited_from_flutter_framework = $inherited
    }
  }
}
$frameworkTarget = Join-Path $destinationRoot 'flutter'
New-Item -ItemType Directory -Path $frameworkTarget -Force | Out-Null
Copy-Item -LiteralPath (Join-Path $frameworkDirectory 'LICENSE') -Destination (Join-Path $frameworkTarget 'LICENSE-Flutter.txt') -Force
Copy-Item -LiteralPath (Join-Path $frameworkDirectory 'bin/cache/dart-sdk/LICENSE') -Destination (Join-Path $frameworkTarget 'LICENSE-Dart.txt') -Force
Copy-Item -LiteralPath (Join-Path $frameworkDirectory 'bin/cache/pkg/sky_engine/license') -Destination (Join-Path $frameworkTarget 'NOTICES-Flutter-engine.txt') -Force
Copy-Item -LiteralPath (Join-Path $frameworkDirectory 'bin/cache/flutter.version.json') -Destination (Join-Path $frameworkTarget 'flutter-version.json') -Force
$engineRuntime = Join-Path $frameworkDirectory 'bin/cache/artifacts/engine/windows-x64-release/flutter_windows.dll'
if (Test-Path -LiteralPath $engineRuntime) {
  $engineEntry = [pscustomobject]@{
    binary = 'flutter_windows.dll'
    version = $configuration.flutterVersion
    sha256 = (Get-FileHash -LiteralPath $engineRuntime -Algorithm SHA256).Hash.ToLowerInvariant()
  }
  [IO.File]::WriteAllText((Join-Path $frameworkTarget 'engine-binary.json'), ($engineEntry | ConvertTo-Json), [Text.UTF8Encoding]::new($false))
}
$result = [pscustomobject]@{
  generator = 'collect_licenses.ps1'
  flutter_version = $configuration.flutterVersion
  pubspec_lock_sha256 = (Get-FileHash -LiteralPath (Join-Path $ProjectDirectory 'pubspec.lock') -Algorithm SHA256).Hash.ToLowerInvariant()
  package_count = @($manifest.package | Sort-Object -Unique).Count
  notices = $manifest
}
[IO.File]::WriteAllText((Join-Path $destinationRoot 'pub-license-manifest.json'), ($result | ConvertTo-Json -Depth 6), [Text.UTF8Encoding]::new($false))
$result | Select-Object flutter_version, package_count, pubspec_lock_sha256 | ConvertTo-Json
