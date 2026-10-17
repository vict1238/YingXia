. "$PSScriptRoot/qa_helpers.ps1"
$qaOut=Join-Path $PSScriptRoot 'fixture_continuous_switch'
$qaPkg='com.yingxia.yingxia_native.fixture'
$qaLibrary="/data/user/0/$qaPkg/files/yingxia/library.json"
New-Item -ItemType Directory -Force -Path $qaOut | Out-Null
function Read-QAPlayed([string]$name,[string]$oldId) {
 for($qaTry=0;$qaTry -lt 20;$qaTry++) {
  Start-Sleep -Milliseconds 400
  $qaRaw=Invoke-QAAdb shell cat $qaLibrary
  $qaRaw | Set-Content "$qaOut/$name.json"
  $qaDoc=($qaRaw -join "`n")|ConvertFrom-Json
  if($qaDoc.history[0].id -ne $oldId -and $qaDoc.history[0].evidence -eq 'browser_time_advanced') {return $qaDoc}
 }
 Invoke-QAAdb logcat -d -v threadtime | Set-Content "$qaOut/${name}_failure_logcat.txt"
 Save-QAScreenshot "$qaOut/${name}_failed.png"
 throw "Replacement did not play: $name"
}
$qaOriginal=((Invoke-QAAdb shell cat $qaLibrary)-join "`n")|ConvertFrom-Json
$xml=Get-QAHierarchy "$qaOut/repeated_stop_before.xml"
Tap-QANode (Find-QANode $xml '停止' -Exact)
$xml=Get-QAHierarchy "$qaOut/repeated_stop_after.xml"
if($null -eq (Find-QANode $xml '已停止' -Exact)) {throw 'Second stop did not finish'}
Tap-QANode (Find-QANode $xml '▷ 播放所选集' -Exact)
$qaRepeated=Read-QAPlayed 'repeated_replay_progress' $qaOriginal.history[0].id
Save-QAScreenshot "$qaOut/repeated_replay_frame.png"
foreach($qaKind in @('WebM','HLS')) {
 Invoke-QAAdb shell input tap 40 558 | Out-Null
 Invoke-QAAdb shell input swipe 158 490 158 145 400 | Out-Null
 $xml=Get-QAHierarchy "$qaOut/switch_${qaKind}_cards.xml"
 Tap-QANode ($xml.SelectNodes('//node')|Where-Object {$_.GetAttribute('content-desc').Contains($qaKind) -and $_.GetAttribute('clickable') -eq 'true'}|Select-Object -First 1)
 $xml=Get-QAHierarchy "$qaOut/switch_${qaKind}_selected.xml"
 Tap-QANode (Find-QANode $xml '▷ 播放所选集' -Exact)
 $qaDoc=Read-QAPlayed "switch_${qaKind}_progress" $qaRepeated.history[0].id
 Save-QAScreenshot "$qaOut/switch_${qaKind}_frame_1.png"
 Start-Sleep -Seconds 1
 Save-QAScreenshot "$qaOut/switch_${qaKind}_frame_2.png"
 $qaRepeated=$qaDoc
}
Invoke-QAAdb logcat -d -v threadtime | Set-Content "$qaOut/repeated_replay_switch_logcat.txt"

