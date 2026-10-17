. "$PSScriptRoot/qa_helpers.ps1"
$qaOut=Join-Path $PSScriptRoot 'production_final'
$qaPkg='com.yingxia.yingxia_native'
$qaActivity="$qaPkg/com.yingxia.yingxia_native.MainActivity"
$qaLibrary="/data/user/0/$qaPkg/files/yingxia/library.json"
Invoke-QAAdb shell am force-stop $qaPkg | Out-Null
Invoke-QAAdb shell stat -c '%u:%g:%a' $qaLibrary | Set-Content "$qaOut/seed_permissions_before.txt"
Invoke-QAAdb shell ls -lZ $qaLibrary | Set-Content "$qaOut/seed_context_before.txt"
Invoke-QAAdb push "$PSScriptRoot/production/library_seed.json" /sdcard/yingxia-qa-library.json | Out-Null
Invoke-QAAdb shell cp /sdcard/yingxia-qa-library.json $qaLibrary | Out-Null
Invoke-QAAdb shell stat -c '%u:%g:%a' $qaLibrary | Set-Content "$qaOut/seed_permissions_after.txt"
Invoke-QAAdb shell ls -lZ $qaLibrary | Set-Content "$qaOut/seed_context_after.txt"
Invoke-QAAdb logcat -c | Out-Null
Invoke-QAAdb shell am start -W -n $qaActivity | Set-Content "$qaOut/clear_seed_launch.log"
Start-Sleep -Seconds 2
Invoke-QAAdb shell input tap 200 558 | Out-Null
Start-Sleep -Milliseconds 500
Invoke-QAAdb shell input swipe 158 485 158 135 400 | Out-Null
$xml=Get-QAHierarchy "$qaOut/clear_seeded_rows.xml"
Save-QAScreenshot "$qaOut/clear_seeded_rows.png"
Tap-QANode (Find-QANode $xml '清空记录' -Exact)
$xml=Get-QAHierarchy "$qaOut/clear_cancel_dialog.xml"
Tap-QANode (Find-QANode $xml '取消' -Exact)
Start-Sleep -Milliseconds 400
Invoke-QAAdb shell cat $qaLibrary | Set-Content "$qaOut/clear_after_cancel.json"
$xml=Get-QAHierarchy "$qaOut/clear_after_cancel.xml"
Tap-QANode (Find-QANode $xml '清空记录' -Exact)
$xml=Get-QAHierarchy "$qaOut/clear_confirm_dialog.xml"
Tap-QANode (Find-QANode $xml '清空' -Exact)
Start-Sleep -Milliseconds 700
Invoke-QAAdb shell cat $qaLibrary | Set-Content "$qaOut/clear_after_confirm.json"
$xml=Get-QAHierarchy "$qaOut/clear_after_confirm.xml"
Save-QAScreenshot "$qaOut/clear_after_confirm.png"
Invoke-QAAdb shell am force-stop $qaPkg | Out-Null
Invoke-QAAdb shell am start -W -n $qaActivity | Set-Content "$qaOut/clear_restart_launch.log"
Start-Sleep -Seconds 2
Invoke-QAAdb shell input tap 200 558 | Out-Null
Start-Sleep -Milliseconds 400
Invoke-QAAdb shell cat $qaLibrary | Set-Content "$qaOut/clear_after_restart.json"
$xml=Get-QAHierarchy "$qaOut/clear_after_restart.xml"
Save-QAScreenshot "$qaOut/clear_after_restart.png"
Invoke-QAAdb logcat -d -v threadtime | Set-Content "$qaOut/clear_logcat.txt"
$qaCancel=Get-Content -Raw "$qaOut/clear_after_cancel.json" | ConvertFrom-Json
$qaConfirm=Get-Content -Raw "$qaOut/clear_after_confirm.json" | ConvertFrom-Json
$qaRestart=Get-Content -Raw "$qaOut/clear_after_restart.json" | ConvertFrom-Json
@{cancel=@{history=$qaCancel.history.Count;favorites=$qaCancel.favorites.Count};confirm=@{history=$qaConfirm.history.Count;favorites=$qaConfirm.favorites.Count};restart=@{history=$qaRestart.history.Count;favorites=$qaRestart.favorites.Count};emptyStateVisible=($null -ne (Find-QANode $xml '暂无记录' -Exact));favoriteVisible=($null -ne (Find-QANode $xml '保留收藏测试'))} | ConvertTo-Json -Depth 5 | Set-Content "$qaOut/clear_summary.json"
Get-Content "$qaOut/clear_summary.json"
