param(
    [string]$SourceRoot = (Join-Path $PSScriptRoot '..'),
    [string]$ReleaseRoot = (Join-Path $PSScriptRoot '..\..\releases')
)
$ErrorActionPreference = 'Stop'
$SourceRoot = [IO.Path]::GetFullPath($SourceRoot)
$ReleaseRoot = [IO.Path]::GetFullPath($ReleaseRoot)
$versionName = 'YingXia_v1.2.0'
$exeName = $versionName + '_Windows_x64.exe'
$zipName = $versionName + '_Windows_x64.zip'
$exePath = Join-Path $ReleaseRoot $exeName
$zipPath = Join-Path $ReleaseRoot $zipName
if (Test-Path -LiteralPath $zipPath) { throw 'This version ZIP already exists. Preserve published artifacts.' }
$info = Get-Content -LiteralPath (Join-Path $SourceRoot 'docs\BUILD_INFO.json') -Raw | ConvertFrom-Json
if ($info.version -ne '1.2.0-exe' -or $info.artifact -ne $exeName) { throw 'Build metadata belongs to another release version' }
$exeHash = (Get-FileHash -LiteralPath $exePath -Algorithm SHA256).Hash.ToLowerInvariant()
if ($exeHash -ne $info.sha256) { throw 'EXE does not match build metadata' }
$stageParent = Join-Path $ReleaseRoot ('package-stage-' + [Guid]::NewGuid().ToString('N'))
$stage = Join-Path $stageParent $versionName
$stageSource = Join-Path $stage 'current_source'
New-Item -ItemType Directory -Path $stageSource -Force | Out-Null
foreach ($item in Get-ChildItem -LiteralPath $SourceRoot -Force) {
    if ($item.Name -in @('build','dist','.git','.tools')) { continue }
    Copy-Item -LiteralPath $item.FullName -Destination $stageSource -Recurse
}
Copy-Item -LiteralPath $exePath -Destination (Join-Path $stage $exeName)
$usage = @'
映匣 v1.2.0

双击 YingXia_v1.2.0_Windows_x64.exe。需要 Windows 10/11 x64 和已有 Edge/Chrome。
无需安装 Python、Go、Node，保留旧版收藏和历史。

本版：搜索时自动检查详情和播放入口，媒体样本可访问的结果排前，页面可访问与失败分别显示。
进度条显示来源搜索及候选检查进度，可取消；点开结果优先选择检测成功的播放分组。
检查使用有限媒体样本，媒体可访问与实际解码播放分别记录。

保留 17 个内置搜索来源，来源开关与 CMS 批量导入，HLS 内置播放器、倍速、自动下一集。
来源与诊断中可检查更新；开启联网及自动更新后，按 24 小时周期跟踪 88lin、KazumiRules、Kazumi。
更新失败保留旧配置，规则接口不兼容时提示升级程序。本版未加入超分。

本轮验证结果以 current_source/docs/validation_v1.2.0 中原始记录为准。
功能和测试说明见 current_source/README_zh.md、current_source/docs/VALIDATION.md。

完整源码、锁定依赖、许可、测试与截图在 current_source。
开发构建见 current_source/README_zh.md 和 docs/BUILD_INFO.json。
'@
[IO.File]::WriteAllText((Join-Path $stage '使用说明.txt'), $usage, [Text.UTF8Encoding]::new($false))
$files = @()
foreach ($file in Get-ChildItem -LiteralPath $stage -Recurse -File | Sort-Object FullName) {
    $relative = [IO.Path]::GetRelativePath($stage,$file.FullName).Replace('\','/')
    $files += [ordered]@{path=$relative;size=$file.Length;sha256=(Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash.ToLowerInvariant()}
}
$manifest = [ordered]@{version='1.2.0-exe';created_at=[DateTime]::UtcNow.ToString('o');file_count=$files.Count;excludes=@('MANIFEST.json','SHA256SUMS.txt');files=$files}
$manifestText = $manifest | ConvertTo-Json -Depth 8
[IO.File]::WriteAllText((Join-Path $stage 'MANIFEST.json'),$manifestText,[Text.UTF8Encoding]::new($false))
$checksums = ($files | ForEach-Object { $_.sha256 + '  ' + $_.path }) -join "`n"
[IO.File]::WriteAllText((Join-Path $stage 'SHA256SUMS.txt'),$checksums,[Text.UTF8Encoding]::new($false))
Copy-Item -LiteralPath (Join-Path $stage 'MANIFEST.json') -Destination (Join-Path $ReleaseRoot ($versionName+'_MANIFEST.json'))
[IO.Compression.ZipFile]::CreateFromDirectory($stage,$zipPath,[IO.Compression.CompressionLevel]::Optimal,$true)
$archive = [IO.Compression.ZipFile]::OpenRead($zipPath)
try {
    foreach ($file in $files) {
        $entry = $archive.GetEntry($versionName+'/'+$file.path)
        if (!$entry -or $entry.Length -ne $file.size) { throw "Archive missing or wrong size: $($file.path)" }
        $stream = $entry.Open()
        $sha = [Security.Cryptography.SHA256]::Create()
        try { $actual=[Convert]::ToHexString($sha.ComputeHash($stream)).ToLowerInvariant() } finally { $stream.Dispose();$sha.Dispose() }
        if ($actual -ne $file.sha256) { throw "Archive hash mismatch: $($file.path)" }
    }
} finally { $archive.Dispose() }
$zipHash=(Get-FileHash -LiteralPath $zipPath -Algorithm SHA256).Hash.ToLowerInvariant()
[IO.File]::WriteAllText($zipPath+'.sha256.txt',$zipHash+'  '+$zipName+[Environment]::NewLine,[Text.UTF8Encoding]::new($false))
$result=[ordered]@{zip=$zipName;size=(Get-Item -LiteralPath $zipPath).Length;sha256=$zipHash;verified_files=$files.Count;all_hashes_match=$true;exe_sha256=$exeHash;staging_directory=$stage}
$resultText=$result|ConvertTo-Json
[IO.File]::WriteAllText((Join-Path $ReleaseRoot ($versionName+'_PACKAGING.json')),$resultText,[Text.UTF8Encoding]::new($false))
$resultText
