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
      dependency_source = if ($package.name -eq 'media_kit_video') { 'local vendored path: vendor/media_kit_video; upstream 2.0.1 with Windows changes' } else { 'pubspec.lock' }
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
# Refresh only this generated section. Platform and FSR provenance above it is
# maintained by the project and must survive a license-collection rerun.
$noticesPath = Join-Path $ProjectDirectory 'THIRD_PARTY_NOTICES.md'
if (Test-Path -LiteralPath $noticesPath) {
  $existing = [IO.File]::ReadAllText($noticesPath)
  $marker = '## 精确锁定的 Dart / Flutter 包'
  $sectionOffset = $existing.IndexOf($marker, [StringComparison]::Ordinal)
  if ($sectionOffset -lt 0) { throw 'Missing generated package notice section.' }
  $rows = @()
  foreach ($group in ($manifest | Group-Object package | Sort-Object Name)) {
    $links = @($group.Group | Sort-Object file | ForEach-Object { '[' + [IO.Path]::GetFileName($_.file) + '](' + $_.file + ')' }) -join '、'
    $rows += '| ' + $group.Name + ' | ' + $group.Group[0].version + ' | ' + $links + ' |'
  }
  $section = $marker + "`n`n" +
    "以下 $($result.package_count) 个包覆盖本次 pubspec.lock，包括构建 / 测试依赖和非目标平台的条件依赖；这不表示所有平台代码都编入了本次 Windows 软件。锁文件 SHA-256：``$($result.pubspec_lock_sha256)``。完整文件哈希见 ``licenses/pub-license-manifest.json``。``media_kit_video`` 2.0.1 从本工程 ``vendor/media_kit_video`` 解析，保留上游 MIT 许可并包含本版 Windows 修改。`n`n" +
    "| 包 | 版本 | 原始许可证文本 |`n|---|---|---|`n" + ($rows -join "`n") + "`n"
  [IO.File]::WriteAllText($noticesPath, $existing.Substring(0, $sectionOffset) + $section, [Text.UTF8Encoding]::new($false))
}
$result | Select-Object flutter_version, package_count, pubspec_lock_sha256 | ConvertTo-Json
