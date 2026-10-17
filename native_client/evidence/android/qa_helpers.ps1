# QA helpers operate only on the owned Android emulator-5554 test device.
$qaAdb = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..\..\.tools\native\android_sdk\platform-tools\adb.exe'))
$qaDevice = 'emulator-5554'
function Invoke-QAAdb { & $qaAdb -s $qaDevice @args }
function Save-QAScreenshot { param([string]$Path)
  Invoke-QAAdb shell screencap -p /sdcard/yingxia-qa.png | Out-Null
  Invoke-QAAdb pull /sdcard/yingxia-qa.png $Path | Out-Null
  if (-not (Test-Path -LiteralPath $Path)) { throw 'Screenshot was not saved' }
}
function Get-QAHierarchy { param([string]$Path)
  Invoke-QAAdb shell uiautomator dump /sdcard/yingxia-qa.xml | Out-Null
  Invoke-QAAdb pull /sdcard/yingxia-qa.xml $Path | Out-Null
  [xml](Get-Content -Raw -LiteralPath $Path)
}
function Find-QANode { param([xml]$Hierarchy,[string]$Label,[switch]$Exact)
  $nodes = $Hierarchy.SelectNodes('//node')
  @($nodes | Where-Object {
    $text = $_.GetAttribute('text')
    $description = $_.GetAttribute('content-desc')
    if ($Exact) { $text -eq $Label -or $description -eq $Label }
    else { $text.Contains($Label) -or $description.Contains($Label) }
  }) | Select-Object -First 1
}
function Tap-QANode { param([Xml.XmlElement]$Node)
  if ($null -eq $Node) { throw 'Requested UI control was not found' }
  if ($Node.GetAttribute('bounds') -notmatch '^\[(\d+),(\d+)\]\[(\d+),(\d+)\]$') { throw 'UI control has invalid bounds' }
  $x=[int](([int]$Matches[1]+[int]$Matches[3])/2)
  $y=[int](([int]$Matches[2]+[int]$Matches[4])/2)
  Invoke-QAAdb shell input tap $x $y | Out-Null
}
