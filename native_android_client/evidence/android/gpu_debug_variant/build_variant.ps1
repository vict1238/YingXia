# SPDX-License-Identifier: GPL-3.0-or-later
$ErrorActionPreference = 'Stop'
$qaClientRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../../..'))
$qaProjectRoot = Split-Path -Parent $qaClientRoot
$qaRuntimeRoot = Join-Path $qaProjectRoot '.tools/native'
$qaClone = Join-Path $qaClientRoot 'build/gpu_debug_client'
$qaFlutter = Join-Path $qaRuntimeRoot 'flutter_source/bin/flutter.bat'
$env:JAVA_HOME = Join-Path $qaRuntimeRoot 'jdk/jdk-17.0.20.1+1'
$env:ANDROID_HOME = Join-Path $qaRuntimeRoot 'android_sdk'
$env:ANDROID_SDK_ROOT = $env:ANDROID_HOME
$env:ANDROID_NDK_HOME = Join-Path $env:ANDROID_HOME 'ndk/28.2.13676358'
$env:GRADLE_USER_HOME = Join-Path $qaRuntimeRoot 'gradle_home_real'
$env:YINGXIA_MAVEN_REPO = Join-Path $qaRuntimeRoot 'maven_local_repo'
$env:YINGXIA_ANDROID_KEY_PROPERTIES = Join-Path $qaRuntimeRoot 'keys/android-key.properties'
$env:YINGXIA_OFFLINE_BUILD = '1'
$env:YINGXIA_ANDROID_FIXTURE = '1'
$env:PATH = (Join-Path $env:JAVA_HOME 'bin') + ';' + $env:PATH
$qaBuildArguments = @('build', 'apk', '--release', '--no-pub', '--target-platform', 'android-x64', '--dart-define=YINGXIA_PLAYER_FIXTURE=true', '--dart-define=YINGXIA_QA_ANDROID_GPU=true')
$qaConfig = [ordered]@{
    created_at_utc = [datetime]::UtcNow.ToString('o')
    clone = $qaClone
    flutter = $qaFlutter
    java_home = $env:JAVA_HOME
    android_sdk = $env:ANDROID_HOME
    ndk = $env:ANDROID_NDK_HOME
    gradle_user_home = $env:GRADLE_USER_HOME
    maven_repo = $env:YINGXIA_MAVEN_REPO
    signing_properties_path = $env:YINGXIA_ANDROID_KEY_PROPERTIES
    fixture_environment = $env:YINGXIA_ANDROID_FIXTURE
    offline = $true
    package = 'com.yingxia.yingxia_native.fixture'
    pub_get_arguments = @('pub', 'get', '--offline')
    build_arguments = $qaBuildArguments
    source_change = "AndroidVideoController.create initial properties: gpu-debug=yes after vo=null, before enabling vo=gpu"
    shipping_source_modified = $false
    target_platform = 'android-x64'
}
$qaConfig | ConvertTo-Json -Depth 8 | Set-Content -Encoding utf8 (Join-Path $PSScriptRoot 'build_configuration.json')
Push-Location $qaClone
try {
    & $qaFlutter pub get --offline 2>&1 | Tee-Object -FilePath (Join-Path $PSScriptRoot 'pub_get.log')
    if ($LASTEXITCODE -ne 0) { throw 'Isolated QA pub get failed' }
    & $qaFlutter @qaBuildArguments 2>&1 | Tee-Object -FilePath (Join-Path $PSScriptRoot 'build.log')
    if ($LASTEXITCODE -ne 0) { throw 'Isolated QA GPU debug build failed' }
    $qaBuiltApk = Join-Path $qaClone 'build/app/outputs/flutter-apk/app-release.apk'
    $qaApk = Join-Path $qaClone 'build/candidates/YingXia_v2.2.0_Android_fixture_gpu_debug_x64.apk'
    New-Item -ItemType Directory -Force -Path (Split-Path -Parent $qaApk) | Out-Null
    Copy-Item -LiteralPath $qaBuiltApk -Destination $qaApk -Force
    & (Join-Path $env:ANDROID_HOME 'build-tools/36.0.0/apksigner.bat') verify --verbose --print-certs $qaApk 2>&1 | Tee-Object -FilePath (Join-Path $PSScriptRoot 'apk_signature.txt')
    if ($LASTEXITCODE -ne 0) { throw 'QA APK signature check failed' }
    & (Join-Path $env:ANDROID_HOME 'build-tools/36.0.0/aapt.exe') dump badging $qaApk 2>&1 | Tee-Object -FilePath (Join-Path $PSScriptRoot 'apk_badging.txt')
    if ($LASTEXITCODE -ne 0) { throw 'QA APK manifest inspection failed' }
    $qaFile = Get-Item -LiteralPath $qaApk
    $qaArtifact = [ordered]@{
        apk = $qaFile.FullName
        bytes = $qaFile.Length
        sha256 = (Get-FileHash -LiteralPath $qaFile.FullName -Algorithm SHA256).Hash.ToLower()
        target_platform = 'android-x64'
        package = 'com.yingxia.yingxia_native.fixture'
        gpu_debug_context = $true
        device_executed = $false
    }
    $qaArtifact | ConvertTo-Json -Depth 5 | Tee-Object -FilePath (Join-Path $PSScriptRoot 'artifact.json')
} finally {
    Pop-Location
}
