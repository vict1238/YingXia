param(
    [switch]$Fixture,
    [string]$ToolsRoot = (Join-Path $PSScriptRoot '../.tools')
)

$ErrorActionPreference = 'Stop'
$taskTools = [IO.Path]::GetFullPath($ToolsRoot)
$taskCore = [IO.Path]::GetFullPath($PSScriptRoot)
$env:GOTOOLCHAIN = 'local'
$env:GOFLAGS = '-mod=mod'
$env:GOPATH = Join-Path $taskTools 'native/go_workspace'
$env:JAVA_HOME = Join-Path $taskTools 'native/jdk/jdk-17.0.20.1+1'
$env:ANDROID_HOME = Join-Path $taskTools 'native/android_sdk'
$env:ANDROID_SDK_ROOT = $env:ANDROID_HOME
$env:ANDROID_NDK_HOME = Join-Path $env:ANDROID_HOME 'ndk/28.2.13676358'
$taskGo = Join-Path $taskTools 'next/go/bin/go.exe'
$taskBin = Join-Path $taskCore 'evidence/tools'
foreach ($taskRequired in @($taskGo, (Join-Path $env:JAVA_HOME 'bin/javac.exe'), (Join-Path $env:ANDROID_NDK_HOME 'source.properties'))) {
    if (!(Test-Path -LiteralPath $taskRequired -PathType Leaf)) { throw "Required build tool missing: $taskRequired" }
}
# Android bind needs this cache directory. Build pinned gobind below instead of
# running upstream init, which installs gobind@latest in this mobile revision.
New-Item -ItemType Directory -Force -Path $taskBin, (Join-Path $taskCore 'build'), (Join-Path $env:GOPATH 'pkg/gomobile') | Out-Null
$env:GOBIN = $taskBin
$env:PATH = (Split-Path $taskGo -Parent) + ';' + (Join-Path $env:JAVA_HOME 'bin') + ';' + $taskBin + ';' + $env:PATH

Push-Location $taskCore
try {
    # go.mod pins both tools to the same upstream mobile revision as the AAR.
    foreach ($taskTool in @('gomobile', 'gobind')) {
        & $taskGo build -trimpath -o (Join-Path $taskBin "$taskTool.exe") "golang.org/x/mobile/cmd/$taskTool"
        if ($LASTEXITCODE -ne 0) { throw "$taskTool source build failed" }
    }
    $taskName = if ($Fixture) { 'yingxia_core_fixture' } else { 'yingxia_core' }
    $taskArgs = @('bind', '-target=android/arm64,android/amd64,android/arm', '-androidapi=26', '-trimpath', '-ldflags=-s -w')
    if ($Fixture) { $taskArgs += '-tags=native_fixture' }
    $taskArgs += @('-o', "build/$taskName.aar", '.')
    & (Join-Path $taskBin 'gomobile.exe') @taskArgs
    if ($LASTEXITCODE -ne 0) { throw "$taskName AAR build failed" }
    Get-FileHash -LiteralPath "build/$taskName.aar" -Algorithm SHA256
} finally {
    Pop-Location
}
