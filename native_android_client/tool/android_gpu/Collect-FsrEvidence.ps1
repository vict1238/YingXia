# SPDX-License-Identifier: GPL-3.0-or-later
[CmdletBinding()]
param(
    [string]$Serial = 'emulator-5560',
    [ValidateSet('com.yingxia.yingxia_native.fixture')]
    [string]$Package = 'com.yingxia.yingxia_native.fixture',
    [datetime]$SinceUtc = [datetime]::UtcNow.AddMinutes(-15),
    [string]$OutputDirectory = (Join-Path $PSScriptRoot '../../evidence/android/gpu'),
    [string]$Adb = (Join-Path $PSScriptRoot '../../../.tools/native/android_sdk/platform-tools/adb.exe')
)

$ErrorActionPreference = 'Stop'
$qaOutput = [IO.Path]::GetFullPath($OutputDirectory)
$qaAdb = (Get-Item -LiteralPath $Adb).FullName
$qaSince = $SinceUtc.ToUniversalTime()
New-Item -ItemType Directory -Force -Path $qaOutput | Out-Null
$qaUtf8 = [Text.UTF8Encoding]::new($false)

function Invoke-ProbeAdb {
    & $qaAdb -s $Serial @args
}

$qaState = @(Invoke-ProbeAdb get-state)
if ($LASTEXITCODE -ne 0 -or ($qaState -join '').Trim() -ne 'device') {
    throw "Android device $Serial is unavailable"
}

$qaDevice = [ordered]@{}
foreach ($qaProperty in @('ro.build.version.sdk', 'ro.product.cpu.abi', 'ro.hardware', 'ro.kernel.qemu')) {
    $qaDevice[$qaProperty] = (@(Invoke-ProbeAdb shell getprop $qaProperty) -join '').Trim()
}

$qaProcessText = (@(Invoke-ProbeAdb shell pidof $Package) -join ' ').Trim()
$qaProcess = ($qaProcessText -split '\s+' | Where-Object { $_ -match '^\d+$' } | Select-Object -First 1)
if ($qaProcess) {
    $qaLines = @(Invoke-ProbeAdb logcat -d -v brief "--pid=$qaProcess")
    $qaLogScope = 'current_fixture_process'
} else {
    # A GPU initialization failure can terminate the process. Read only these
    # tags, retain structured fixture records, and classify fixed error phrases.
    $qaLines = @(Invoke-ProbeAdb logcat -d -v brief -s flutter mpv media_kit VideoOutput EGL_emulation AndroidRuntime)
    $qaLogScope = 'filtered_tags_process_not_running'
}
if ($LASTEXITCODE -ne 0) { throw 'Reading Android logcat failed' }

$qaRecords = [Collections.Generic.List[object]]::new()
$qaMalformed = 0
foreach ($qaLine in $qaLines) {
    $qaMarker = 'YINGXIA_FSR_QA '
    $qaMarkerIndex = $qaLine.IndexOf($qaMarker, [StringComparison]::Ordinal)
    if ($qaMarkerIndex -lt 0) { continue }
    $qaJson = $qaLine.Substring($qaMarkerIndex + $qaMarker.Length)
    if ($qaJson.Length -gt 32768) { $qaMalformed++; continue }
    try {
        $qaRecord = $qaJson | ConvertFrom-Json
        $qaRecordTime = ([datetime]$qaRecord.time).ToUniversalTime()
        if ($qaRecordTime -ge $qaSince) { $qaRecords.Add($qaRecord) }
    } catch { $qaMalformed++ }
}

$qaGpuErrors = [ordered]@{}
foreach ($qaPhrase in @('EGL_BAD_NATIVE_WINDOW', 'Could not create EGL surface', 'EGL failed to initialize', 'Failed to set context', 'Failed initializing any suitable GPU context')) {
    $qaGpuErrors[$qaPhrase] = @($qaLines | Where-Object { $_.Contains($qaPhrase) }).Count
}

$qaSamples = @($qaRecords | Where-Object { $_.event -eq 'sample' })
$qaProof = @($qaSamples | Where-Object {
    $_.effective -in @('light', 'enhanced') -and $_.shader_verified -eq $true -and
    $_.renderer -eq 'android-gpu' -and $_.current_vo -eq 'gpu' -and
    $null -ne $_.target -and $null -ne $_.mpv_output -and
    $_.target.width -eq $_.mpv_output.width -and $_.target.height -eq $_.mpv_output.height -and
    $_.target.width -gt $_.source.width -and $_.target.height -gt $_.source.height -and
    $_.target.width -le 1920 -and $_.target.height -le 1080
})
$qaOffRestored = @($qaSamples | Where-Object {
    $_.effective -eq 'off' -and $_.current_vo -eq 'gpu' -and
    $null -eq $_.target -and $null -ne $_.source -and $null -ne $_.mpv_output -and
    $_.source.width -eq $_.mpv_output.width -and $_.source.height -eq $_.mpv_output.height
})
$qaModes = @($qaProof | ForEach-Object effective | Sort-Object -Unique)
$qaFallbacks = @($qaRecords | Where-Object { $_.fallback } | ForEach-Object fallback | Sort-Object -Unique)
$qaClassification = 'insufficient_evidence'
if ($qaProof.Count -gt 0) { $qaClassification = 'gpu_shader_verified' }
elseif ($qaFallbacks.Count -gt 0) {
    $qaClassification = 'gpu_path_unavailable_or_fell_back'
} elseif (($qaGpuErrors.Values | Measure-Object -Sum).Sum -gt 0) {
    $qaClassification = 'gpu_errors_in_log_buffer_without_shader_proof'
}

$qaEvidenceText = @($qaRecords | ForEach-Object { $_ | ConvertTo-Json -Compress -Depth 20 }) -join "`n"
[IO.File]::WriteAllText((Join-Path $qaOutput 'fsr_samples.jsonl'), $qaEvidenceText, $qaUtf8)
$qaReport = [ordered]@{
    collected_at_utc = [datetime]::UtcNow.ToString('o')
    since_utc = $qaSince.ToString('o')
    serial = $Serial
    package = $Package
    process_id = $qaProcess
    log_scope = $qaLogScope
    device = $qaDevice
    classification = $qaClassification
    structured_records = $qaRecords.Count
    malformed_or_truncated_records = $qaMalformed
    active_samples = $qaSamples.Count
    proven_shader_samples = $qaProof.Count
    verified_modes = $qaModes
    off_at_source_size_samples = $qaOffRestored.Count
    light_enhanced_off_verified = ('light' -in $qaModes -and 'enhanced' -in $qaModes -and $qaOffRestored.Count -gt 0)
    fallback_reasons = $qaFallbacks
    gpu_error_phrase_counts = $qaGpuErrors
    performance_scope = 'This device and emulator renderer only; no ARM device performance inference.'
    gpu_error_time_scope = 'Error phrase counts cover the retained log buffer; structured records are filtered by since_utc.'
}
$qaReportText = $qaReport | ConvertTo-Json -Depth 20
[IO.File]::WriteAllText((Join-Path $qaOutput 'summary.json'), $qaReportText, $qaUtf8)
$qaReportText
