. "$PSScriptRoot/qa_helpers.ps1"
$qaOut=Join-Path $PSScriptRoot 'fixture_candidate3_pause'
New-Item -ItemType Directory -Force -Path $qaOut | Out-Null
$qaPkg='com.yingxia.yingxia_native.fixture'
$qaActivity="$qaPkg/com.yingxia.yingxia_native.MainActivity"
Invoke-QAAdb shell am force-stop $qaPkg | Out-Null
Invoke-QAAdb logcat -c | Out-Null
Invoke-QAAdb shell am start -W -n $qaActivity | Set-Content "$qaOut/pause_fresh_launch.log"
Start-Sleep -Seconds 3
$xml=Get-QAHierarchy "$qaOut/pause_fresh_search.xml"
Tap-QANode ($xml.SelectNodes('//node') | Where-Object {$_.GetAttribute('class') -eq 'android.widget.EditText'} | Select-Object -First 1)
Invoke-QAAdb shell input text native | Out-Null
Invoke-QAAdb shell input keyevent 4 | Out-Null
$xml=Get-QAHierarchy "$qaOut/pause_fresh_query.xml"
Tap-QANode (Find-QANode $xml '搜索' -Exact)
Start-Sleep -Seconds 2
Invoke-QAAdb shell input swipe 158 490 158 145 400 | Out-Null
$xml=Get-QAHierarchy "$qaOut/pause_fresh_cards.xml"
Tap-QANode ($xml.SelectNodes('//node') | Where-Object {$_.GetAttribute('content-desc').Contains('HLS') -and $_.GetAttribute('clickable') -eq 'true'} | Select-Object -First 1)
Start-Sleep -Milliseconds 500
$xml=Get-QAHierarchy "$qaOut/pause_fresh_selected.xml"
Tap-QANode (Find-QANode $xml '▷ 播放所选集' -Exact)
$playing=$false
for($qaTry=0;$qaTry -lt 16;$qaTry++) {
 Start-Sleep -Milliseconds 400
 $raw=Invoke-QAAdb shell cat "/data/user/0/$qaPkg/files/yingxia/library.json"
 $doc=($raw -join "`n") | ConvertFrom-Json
 if($doc.history[0].evidence -eq 'browser_time_advanced' -and $doc.history[0].position -gt 0) { $playing=$true; break }
}
if(-not $playing) {throw 'Fresh HLS did not advance'}
Save-QAScreenshot "$qaOut/navigation_active_frame.png"
Invoke-QAAdb shell input tap 200 558 | Out-Null
Start-Sleep -Milliseconds 600
Invoke-QAAdb shell cat "/data/user/0/$qaPkg/files/yingxia/library.json" | Set-Content "$qaOut/navigation_active_pause_before.json"
Start-Sleep -Seconds 2
Invoke-QAAdb shell cat "/data/user/0/$qaPkg/files/yingxia/library.json" | Set-Content "$qaOut/navigation_active_pause_after.json"
Invoke-QAAdb shell input tap 120 558 | Out-Null
Start-Sleep -Milliseconds 400
$xml=Get-QAHierarchy "$qaOut/navigation_active_return.xml"
Save-QAScreenshot "$qaOut/navigation_active_return_1.png"
Start-Sleep -Seconds 2
Save-QAScreenshot "$qaOut/navigation_active_return_2.png"
Tap-QANode (Find-QANode $xml '▷ 播放所选集' -Exact)
Start-Sleep -Seconds 1
Save-QAScreenshot "$qaOut/manual_resume_frame.png"
Invoke-QAAdb shell input keyevent 3 | Out-Null
Start-Sleep -Milliseconds 400
Invoke-QAAdb shell cat "/data/user/0/$qaPkg/files/yingxia/library.json" | Set-Content "$qaOut/home_pause_before.json"
Start-Sleep -Seconds 2
Invoke-QAAdb shell cat "/data/user/0/$qaPkg/files/yingxia/library.json" | Set-Content "$qaOut/home_pause_after.json"
Invoke-QAAdb shell am start -W -n $qaActivity | Out-Null
Start-Sleep -Milliseconds 400
$xml=Get-QAHierarchy "$qaOut/home_return.xml"
Save-QAScreenshot "$qaOut/home_return_1.png"
Start-Sleep -Seconds 2
Save-QAScreenshot "$qaOut/home_return_2.png"
Invoke-QAAdb logcat -d -v threadtime | Set-Content "$qaOut/active_pause_logcat.txt"
$xml.SelectNodes('//node') | Where-Object {$_.GetAttribute('content-desc') -in @('已暂停','播放中','播放结束','加载中…')} | ForEach-Object {$_.GetAttribute('content-desc')}

