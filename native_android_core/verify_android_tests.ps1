param([string]$Serial = 'emulator-5560')
$ErrorActionPreference = 'Stop'
if ($Serial -ne 'emulator-5560') { throw 'This QA script only targets the owned emulator.' }
$taskProject = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$taskAdb = Join-Path $taskProject '.tools/native/android_sdk/platform-tools/adb.exe'
$taskHardware = & $taskAdb -s $Serial shell getprop ro.hardware
if (($taskHardware -join '').Trim() -ne 'ranchu') { throw 'Owned emulator hardware was not found.' }
$taskEvidence = Join-Path $PSScriptRoot 'evidence/v2_3'
New-Item -ItemType Directory -Force -Path $taskEvidence | Out-Null
$env:GOOS = 'android'
$env:GOARCH = 'amd64'
$env:CGO_ENABLED = '1'
$env:GOTOOLCHAIN = 'local'
$env:CC = Join-Path $taskProject '.tools/native/android_sdk/ndk/28.2.13676358/toolchains/llvm/prebuilt/windows-x86_64/bin/x86_64-linux-android26-clang.cmd'
$taskGo = Join-Path $taskProject '.tools/next/go/bin/go.exe'
& $taskAdb -s $Serial shell mkdir -p /data/local/tmp/yingxia_23_goqa/tmp
foreach ($taskSuite in @(@{Name='engine';Package='./internal/engine'},@{Name='core';Package='.'})) {
  $taskOutput = Join-Path $taskEvidence ($taskSuite.Name+'.test')
  & $taskGo -C $PSScriptRoot test -mod=vendor -c -o $taskOutput $taskSuite.Package
  if ($LASTEXITCODE -ne 0) { throw 'Android test compilation failed.' }
  $taskRemote = '/data/local/tmp/yingxia_23_goqa/'+$taskSuite.Name+'.test'
  & $taskAdb -s $Serial push $taskOutput $taskRemote
  if ($LASTEXITCODE -ne 0) { throw 'Android test transfer failed.' }
  & $taskAdb -s $Serial shell chmod 700 $taskRemote
  $taskLog = Join-Path $taskEvidence ('android_'+$taskSuite.Name+'_tests.log')
  & $taskAdb -s $Serial shell "TMPDIR=/data/local/tmp/yingxia_23_goqa/tmp $taskRemote -test.v" *> $taskLog
  $taskExit = $LASTEXITCODE
  $taskExit | Set-Content -LiteralPath ($taskLog.Replace('.log','.exit.txt'))
  Get-Content -LiteralPath $taskLog | Select-Object -Last 13
  if ($taskExit -ne 0) { throw 'Android Go tests failed; inspect the log.' }
}
