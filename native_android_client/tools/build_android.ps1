param([switch]$Fixture, [switch]$QaGpu, [switch]$Online)
$ErrorActionPreference = 'Stop'
$clientRoot = (Get-Item (Join-Path $PSScriptRoot '..')).FullName
$projectRoot = (Get-Item (Join-Path $clientRoot '..')).FullName
$runtimeRoot = Join-Path $projectRoot '.tools/native'
if (-not $env:JAVA_HOME) { $env:JAVA_HOME = Join-Path $runtimeRoot 'jdk/jdk-17.0.20.1+1' }
if (-not $env:ANDROID_HOME) { $env:ANDROID_HOME = Join-Path $runtimeRoot 'android_sdk' }
$env:ANDROID_SDK_ROOT = $env:ANDROID_HOME
$env:ANDROID_NDK_HOME = Join-Path $env:ANDROID_HOME 'ndk/28.2.13676358'
if (-not $env:GRADLE_USER_HOME) { $env:GRADLE_USER_HOME = Join-Path $runtimeRoot 'gradle_home_real' }
if (-not $env:YINGXIA_MAVEN_REPO) { $env:YINGXIA_MAVEN_REPO = Join-Path $runtimeRoot 'maven_local_repo' }
if (-not $env:YINGXIA_ANDROID_KEY_PROPERTIES) { $env:YINGXIA_ANDROID_KEY_PROPERTIES = Join-Path $runtimeRoot 'keys/android-key.properties' }
if (-not $env:YINGXIA_FLUTTER) { $env:YINGXIA_FLUTTER = Join-Path $runtimeRoot 'flutter_source/bin/flutter.bat' }
$env:PATH = (Join-Path $env:JAVA_HOME 'bin') + ';' + $env:PATH
$env:YINGXIA_OFFLINE_BUILD = if ($Online) { $null } else { '1' }
$env:YINGXIA_ANDROID_FIXTURE = if ($Fixture) { '1' } else { $null }
if ($QaGpu -and -not $Fixture) { throw 'QA GPU mode requires the isolated fixture application.' }
if (-not (Test-Path -LiteralPath $env:YINGXIA_ANDROID_KEY_PROPERTIES)) { throw 'Set YINGXIA_ANDROID_KEY_PROPERTIES to your private signing properties file.' }
$productionAar = Join-Path $projectRoot 'native_android_core/build/yingxia_core.aar'
$selectedAar = if ($Fixture) { Join-Path $projectRoot 'native_android_core/build/yingxia_core_fixture.aar' } else { $productionAar }
$clientAar = Join-Path $clientRoot 'android/app/libs/yingxia_core.aar'
if (-not (Test-Path -LiteralPath $selectedAar)) { throw 'Build the Android Go AAR first; see native_android_core/tools.' }
New-Item -ItemType Directory -Path (Split-Path -Parent $clientAar) -Force | Out-Null
Copy-Item -LiteralPath $selectedAar -Destination $clientAar -Force
$candidateDirectory = Join-Path $clientRoot 'build/candidates'
New-Item -ItemType Directory -Path $candidateDirectory -Force | Out-Null
$candidateName = if ($QaGpu) { 'YingXia_v2.3.0_Android_fixture_gpu.apk' } elseif ($Fixture) { 'YingXia_v2.3.0_Android_fixture.apk' } else { 'YingXia_v2.3.0_Android.apk' }
$candidate = Join-Path $candidateDirectory $candidateName
Push-Location $clientRoot
try {
    $buildArguments = @('build', 'apk', '--release', '--no-pub', '--target-platform', 'android-arm,android-arm64,android-x64')
    if ($QaGpu) { $buildArguments += @('--dart-define=YINGXIA_PLAYER_FIXTURE=true', '--dart-define=YINGXIA_QA_ANDROID_GPU=true') }
    & $env:YINGXIA_FLUTTER @buildArguments
    if ($LASTEXITCODE -ne 0) { throw 'Android release build failed.' }
    Copy-Item -LiteralPath (Join-Path $clientRoot 'build/app/outputs/flutter-apk/app-release.apk') -Destination $candidate -Force
    & (Join-Path $env:ANDROID_HOME 'build-tools/36.0.0/apksigner.bat') verify --verbose --print-certs $candidate
    if ($LASTEXITCODE -ne 0) { throw 'APK signature verification failed.' }
    Get-FileHash -LiteralPath $candidate -Algorithm SHA256
} finally {
    Pop-Location
    if ($Fixture) {
        Copy-Item -LiteralPath $productionAar -Destination $clientAar -Force
        if ((Get-FileHash -LiteralPath $productionAar).Hash -ne (Get-FileHash -LiteralPath $clientAar).Hash) { throw 'Production AAR restoration failed.' }
    }
}
