. "$PSScriptRoot/qa_helpers.ps1"
$qaOut=Join-Path $PSScriptRoot 'fixture_candidate3'
New-Item -ItemType Directory -Force -Path $qaOut | Out-Null
$qaPkg='com.yingxia.yingxia_native.fixture'
$qaActivity="$qaPkg/com.yingxia.yingxia_native.MainActivity"
$qaLibrary="/data/user/0/$qaPkg/files/yingxia/library.json"
function Save-QALibrary([string]$name) {
 $qaRaw=Invoke-QAAdb shell cat $qaLibrary
 $qaRaw | Set-Content "$qaOut/$name.json"
 ($qaRaw -join "`n")|ConvertFrom-Json
}
function Wait-QAPlayed([string]$name,[string]$priorId='') {
 for($qaTry=0;$qaTry -lt 20;$qaTry++) {
  Start-Sleep -Milliseconds 400
  $qaDoc=Save-QALibrary $name
  if($qaDoc.history[0].id -ne $priorId -and $qaDoc.history[0].evidence -eq 'browser_time_advanced' -and $qaDoc.history[0].position -gt 0) {return $qaDoc}
 }
 Save-QAScreenshot "$qaOut/${name}_failed.png"
 Invoke-QAAdb logcat -d -v threadtime | Set-Content "$qaOut/${name}_failed_logcat.txt"
 throw "Media did not play: $name"
}
Invoke-QAAdb shell am force-stop $qaPkg | Out-Null
Invoke-QAAdb logcat -c | Out-Null
Invoke-QAAdb shell am start -W -n $qaActivity | Set-Content "$qaOut/launch.log"
Start-Sleep -Seconds 2
$xml=Get-QAHierarchy "$qaOut/search.xml"
Tap-QANode ($xml.SelectNodes('//node') | Where-Object {$_.GetAttribute('class') -eq 'android.widget.EditText'} | Select-Object -First 1)
Invoke-QAAdb shell input text native | Out-Null
Invoke-QAAdb shell input keyevent 4 | Out-Null
$xml=Get-QAHierarchy "$qaOut/query.xml"
Tap-QANode (Find-QANode $xml '搜索' -Exact)
Start-Sleep -Seconds 2
Invoke-QAAdb shell input swipe 158 490 158 145 400 | Out-Null
$xml=Get-QAHierarchy "$qaOut/cards.xml"
Tap-QANode ($xml.SelectNodes('//node') | Where-Object {$_.GetAttribute('content-desc').Contains('HLS') -and $_.GetAttribute('clickable') -eq 'true'} | Select-Object -First 1)
$xml=Get-QAHierarchy "$qaOut/selected.xml"
Tap-QANode (Find-QANode $xml '▷ 播放所选集' -Exact)
$qaFirst=Wait-QAPlayed 'hls_first_progress'
Save-QAScreenshot "$qaOut/hls_first_frame.png"
Invoke-QAAdb shell input swipe 158 490 158 340 300 | Out-Null
$xml=Get-QAHierarchy "$qaOut/stop_controls.xml"
Tap-QANode (Find-QANode $xml '停止' -Exact)
Start-Sleep -Milliseconds 400
$xml=Get-QAHierarchy "$qaOut/stopped.xml"
Save-QAScreenshot "$qaOut/stopped.png"
if($null -eq (Find-QANode $xml '已停止' -Exact)) {
 Start-Sleep -Seconds 2
 $xml=Get-QAHierarchy "$qaOut/stopped_retry.xml"
 if($null -eq (Find-QANode $xml '已停止' -Exact)) {
  Invoke-QAAdb logcat -d -v threadtime | Set-Content "$qaOut/stop_failed_logcat.txt"
  throw 'Stop did not finish'
 }
}
Tap-QANode (Find-QANode $xml '▷ 播放所选集' -Exact)
$qaReplay=Wait-QAPlayed 'hls_replay_progress' $qaFirst.history[0].id
Save-QAScreenshot "$qaOut/hls_replay_frame_1.png"
Start-Sleep -Seconds 1
Save-QAScreenshot "$qaOut/hls_replay_frame_2.png"
$xml=Get-QAHierarchy "$qaOut/replay_controls.xml"
Tap-QANode (Find-QANode $xml '下一集' -Exact)
$qaNext=Wait-QAPlayed 'hls_episode2_progress' $qaReplay.history[0].id
Save-QAScreenshot "$qaOut/hls_episode2_frame.png"
Invoke-QAAdb logcat -d -v threadtime | Set-Content "$qaOut/stop_replay_logcat.txt"
@{first=$qaFirst.history[0];replay=$qaReplay.history[0];next=$qaNext.history[0];nextEpisodeCorrect=($qaNext.history[0].episode -eq '第02集');stopCompleted=$true} | ConvertTo-Json -Depth 8 | Set-Content "$qaOut/stop_replay_summary.json"
Get-Content "$qaOut/stop_replay_summary.json"
