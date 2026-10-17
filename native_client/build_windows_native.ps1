param(
  [string]$ToolsRoot = (Join-Path $PSScriptRoot '..\.tools\native'),
  [string]$BundleDirectory = (Join-Path $PSScriptRoot 'build\windows\native_bundle')
)
$ErrorActionPreference = 'Stop'
$clientRoot = [IO.Path]::GetFullPath($PSScriptRoot)
$ToolsRoot = [IO.Path]::GetFullPath($ToolsRoot)
$buildRoot = Join-Path $clientRoot 'build'
$BundleDirectory = [IO.Path]::GetFullPath($BundleDirectory)
if (!$BundleDirectory.StartsWith($buildRoot+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase)) { throw 'Build bundle must remain inside native_client/build.' }
$flutter = Join-Path $ToolsRoot 'flutter_source\bin\flutter.bat'
$setup = Join-Path $ToolsRoot 'msvc\setup_x64.bat'
$cmake = Join-Path $ToolsRoot 'cmake\cmake-3.31.8-windows-x86_64\bin\cmake.exe'
$ninja = Join-Path $ToolsRoot 'ninja\ninja.exe'
foreach($required in @($flutter,$setup,$cmake,$ninja)){ if(!(Test-Path -LiteralPath $required)){throw "Missing portable build tool: $required"} }
# Read only the compiler environment produced by the portable Microsoft tools.
$compilerEnvironment = & $env:ComSpec /d /c "call `"$setup`" >nul && set"
if ($LASTEXITCODE -ne 0) { throw 'Portable compiler environment failed.' }
foreach($line in $compilerEnvironment){
  if($line -match '^(PATH|INCLUDE|LIB|VCToolsVersion|WindowsSDKVersion|VCToolsInstallDir|WindowsSdkBinPath|VSCMD_ARG_HOST_ARCH|VSCMD_ARG_TGT_ARCH)=(.*)$'){
    [Environment]::SetEnvironmentVariable($matches[1],$matches[2],'Process')
  }
}
$env:Path = (Split-Path -Parent $cmake)+';'+(Split-Path -Parent $ninja)+';'+(Split-Path -Parent $flutter)+';'+$env:Path
Push-Location $clientRoot
try {
  # Flutter writes generated_config.cmake and plugin links before it reports
  # missing registered Visual Studio. The subsequent build uses CMake/Ninja.
  $pluginMetadata = Join-Path $clientRoot '.flutter-plugins-dependencies'
  if(!(Test-Path -LiteralPath $pluginMetadata)){throw 'Run flutter pub get before the Windows build.'}
  $plugins = (Get-Content -LiteralPath $pluginMetadata -Raw | ConvertFrom-Json).plugins.windows
  $links = Join-Path $clientRoot 'windows\flutter\ephemeral\.plugin_symlinks'
  New-Item -ItemType Directory -Path $links -Force | Out-Null
  foreach($plugin in $plugins){
    $link = Join-Path $links $plugin.name
    if(!(Test-Path -LiteralPath $link)){New-Item -ItemType Junction -Path $link -Value ([IO.Path]::GetFullPath($plugin.path)) | Out-Null}
  }
  & $flutter build windows --release --config-only --no-pub
  $generated = Join-Path $clientRoot 'windows\flutter\ephemeral\generated_config.cmake'
  if(!(Test-Path -LiteralPath $generated)){throw 'Flutter did not produce Windows build configuration.'}
  $cmakeBuild = Join-Path $clientRoot 'build\windows\native_cmake'
  & $cmake -S (Join-Path $clientRoot 'windows') -B $cmakeBuild -G Ninja '-DCMAKE_BUILD_TYPE=Release' '-DFLUTTER_TARGET_PLATFORM=windows-x64' "-DCMAKE_INSTALL_PREFIX=$($BundleDirectory.Replace('\','/'))"
  if($LASTEXITCODE -ne 0){throw 'Native CMake generation failed.'}
  # The upstream media plugin assumes MSBuild ordering. Pre-extract its pinned
  # archives with CMake, then regenerate so Ninja sees the existing libraries.
  foreach($item in @(@{Archive='mpv-dev-x86_64-20230924-git-652a1dd.7z';Directory='libmpv'},@{Archive='ANGLE.7z';Directory='ANGLE'})){
    $archivePath = Join-Path $cmakeBuild $item.Archive
    $extractRoot = [IO.Path]::GetFullPath((Join-Path $cmakeBuild $item.Directory))
    if(!$extractRoot.StartsWith($buildRoot+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase)){throw 'Archive extraction escapes the project build directory.'}
    if(!(Test-Path -LiteralPath (Join-Path $extractRoot 'libmpv-2.dll')) -and !(Test-Path -LiteralPath (Join-Path $extractRoot 'libEGL.dll'))){
      New-Item -ItemType Directory -Path $extractRoot -Force | Out-Null
      $entries = & $cmake -E tar tf $archivePath
      if($LASTEXITCODE -ne 0){throw 'Native dependency archive cannot be listed.'}
      foreach($entry in $entries){$name=$entry.Replace('\','/');if($name.StartsWith('/') -or $name.Contains(':') -or $name -match '(^|/)\.\.(/|$)'){throw 'Native dependency archive contains an unsafe entry.'}}
      Push-Location $extractRoot
      try { & $cmake -E tar xzf $archivePath; if($LASTEXITCODE -ne 0){throw 'Native dependency extraction failed.'} } finally { Pop-Location }
      if($item.Directory -eq 'libmpv'){
        $includeRoot = Join-Path $extractRoot 'include'
        $stagedInclude = Join-Path $extractRoot 'mpv_include_staged'
        New-Item -ItemType Directory -Path $stagedInclude -Force | Out-Null
        Get-ChildItem -LiteralPath (Join-Path $includeRoot 'mpv') -File | ForEach-Object { Copy-Item -LiteralPath $_.FullName -Destination $stagedInclude }
        foreach($target in @($includeRoot,$stagedInclude)){if(![IO.Path]::GetFullPath($target).StartsWith($extractRoot+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase)){throw 'Header staging escapes the native dependency directory.'}}
        Remove-Item -LiteralPath $includeRoot -Recurse -Force
        Move-Item -LiteralPath $stagedInclude -Destination $includeRoot
      }
    }
  }
  & $cmake -S (Join-Path $clientRoot 'windows') -B $cmakeBuild -G Ninja '-DCMAKE_BUILD_TYPE=Release' '-DFLUTTER_TARGET_PLATFORM=windows-x64' "-DCMAKE_INSTALL_PREFIX=$($BundleDirectory.Replace('\','/'))"
  if($LASTEXITCODE -ne 0){throw 'Native CMake regeneration failed.'}
  & $cmake --build $cmakeBuild --config Release --parallel 6
  if($LASTEXITCODE -ne 0){throw 'Native Windows compilation failed.'}
  & $cmake --install $cmakeBuild --config Release
  if($LASTEXITCODE -ne 0){throw 'Native Windows bundle installation failed.'}
  Copy-Item -LiteralPath (Join-Path $clientRoot '..\native_core\build\yingxia_backend.exe') -Destination (Join-Path $BundleDirectory 'yingxia_backend.exe')
  Get-ChildItem -LiteralPath (Join-Path $ToolsRoot 'msvc_redist') -File -Filter '*.dll' | ForEach-Object { Copy-Item -LiteralPath $_.FullName -Destination (Join-Path $BundleDirectory $_.Name) }
  Copy-Item -LiteralPath (Join-Path $ToolsRoot 'flutter_source\LICENSE') -Destination (Join-Path $BundleDirectory 'LICENSE_FLUTTER.txt')
  Copy-Item -LiteralPath (Join-Path $clientRoot '..\current_source\LICENSE.txt') -Destination (Join-Path $BundleDirectory 'LICENSE_YINGXIA.txt')
  # These dependencies declare web-only assets for every target. Native Flutter
  # uses libmpv and platform wakelock; its Windows runtime never loads these JS.
  Get-ChildItem -LiteralPath $BundleDirectory -Recurse -File -Filter '*.js' | ForEach-Object {
    if(!$_.FullName.StartsWith($BundleDirectory+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase)){throw 'Unused asset path escapes the bundle.'}
    Remove-Item -LiteralPath $_.FullName
  }
  Write-Output "Native Windows bundle: $BundleDirectory"
} finally { Pop-Location }
