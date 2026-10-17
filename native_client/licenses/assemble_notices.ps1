$ErrorActionPreference = 'Stop'
$project = Split-Path -Parent $PSScriptRoot
$manifest = Get-Content -Raw -LiteralPath (Join-Path $PSScriptRoot 'pub-license-manifest.json') | ConvertFrom-Json
$version = Get-Content -Raw -LiteralPath (Join-Path $PSScriptRoot 'flutter/flutter-version.json') | ConvertFrom-Json
$lines = @(
  '# 映匣原生版第三方组件说明',
  '',
  '本目录保存构建实际使用的依赖版本、原始版权和许可证文本、下载来源、二进制校验以及播放器构建配方。映匣代码使用 GPL-3.0-or-later；每个第三方组件保留自己的许可证。此文件不改变软件界面。',
  '',
  '## Flutter 与 Dart',
  '',
  ('Flutter {0}，framework commit `{1}`；engine commit `{2}`，engine content hash `{3}`；Dart {4}。' -f $version.frameworkVersion,$version.frameworkRevision,$version.engineRevision,$version.engineContentHash,$version.dartSdkVersion),
  '',
  'Flutter / Dart 原文见 `licenses/flutter/LICENSE-Flutter.txt`、`LICENSE-Dart.txt`。Flutter engine 汇总原文见 `licenses/flutter/NOTICES-Flutter-engine.txt`。Material Icons、Roboto、Roboto Condensed 原始文本同时保存。生成的应用资产 `flutter_assets/NOTICES.Z` 还保留 Flutter 构建器汇总的依赖通知。',
  '',
  '## 原生播放器',
  '',
  '| 平台 | 实际原生组件 | 来源与构建记录 |',
  '|---|---|---|',
  '| Windows | libmpv v0.36.0-403-g652a1dd907；FFmpeg n6.0 | [media-kit 2023-09-24 二进制](https://github.com/media-kit/libmpv-win32-video-build/releases/tag/2023-09-24)，文件 `mpv-dev-x86_64-20230924-git-652a1dd.7z`；DLL 元数据见 `licenses/native/libmpv-windows-binary.json`，实际 FFmpeg configure 字符串见 `ffmpeg-windows-configure.txt`。 |',
  '| Android | media-kit native default v1.1.7；mpv 78d43740f52db817d98bcf24fb30a76ab6fa13ff；FFmpeg 6.0 | [不可变构建配方 fe8c3ac1](https://github.com/media-kit/libmpv-android-video-build/tree/fe8c3ac1a91c09aa6fb1deccbc833f1bafa54768)，保存了 recipe、依赖版本和 FFmpeg / mpv 的补丁。APK 使用 default flavor，不使用 encoders-gpl。 |',
  '',
  '两份配方均使用 `-Dgpl=false` 的 mpv，并将 FFmpeg 配置为 `--disable-gpl --disable-nonfree --enable-version3`。mpv 原始版权文件说明这个配置使用 LGPL-2.1-or-later；FFmpeg 的 `--enable-version3` 选择 LGPL-3.0。相应 LGPL / GPL 原文和 FFmpeg 的完整上游许可证说明均在 `licenses/native`。复制 GPL 原文是保留上游 source 中的备用配置文本，并不意味着本次启用了那些 GPL 编解码器。',
  '',
  '源代码快照随附：`mpv-windows-652a1dd907.zip`、`mpv-android-78d43740.zip`、`ffmpeg-6.0.tar.xz`；构建配方快照为 `libmpv-windows-build-f5a6f879.zip`、`libmpv-android-build-fe8c3ac1.zip`。下载 URL、大小、SHA-256 见 `upstream-source-manifest.json`。Android recipe 中包含版本锁定及所应用的修改补丁。',
  '',
  'Windows release tag 的树仅保存 `version`，不保存当次构建的完整源码锁定记录。随附 Windows builder snapshot 是 release 时间之前的最近 master commit `f5a6f879c9b8bef6a73e52c8c0f35e51636c96e5`，这个关联是根据发布时间推得；mpv 和 FFmpeg 版本则直接读取发布 DLL，不依赖该推断。播放器第三方依赖的配置由这个配方列出；不能据此声称复建后二进制逐字节相同。',
  '',
  '## Windows 图形与 Microsoft 运行库',
  '',
  'ANGLE 从 [v1.0.1 预构建包](https://github.com/alexmercerind/flutter-windows-ANGLE-OpenGL-ES/releases/tag/v1.0.1) 引入。实际 `libEGL.dll` / `libGLESv2.dll` 的 PE 版本为 `2.1.18844 git hash: 2693b03eba82`，其 [ANGLE BSD 原文](https://github.com/google/angle/blob/2693b03eba82/LICENSE) 和 AUTHORS 已保存。ANGLE 是本机 OpenGL / Direct3D 图形转换库；它不创建 HTML 页面、WebView 或浏览器窗口。',
  '',
  '同时保留 SwiftShader、Vulkan loader、LLVM libc++、zlib 的原始许可证参考文本。其二进制及 PE 元数据、SHA-256 见 `angle-binaries.json`。SwiftShader / Vulkan / zlib / libc++ 的独立源代码版本没有在此 ANGLE 发布包中完整锁定；保存的文本标明各自上游来源，不将许可证参考版本冒充实际二进制版本。',
  '',
  'Microsoft Visual C++ runtime 和 `d3dcompiler_47.dll` 为 Microsoft 的专有可再发行组件，并非映匣或 media-kit 的开源代码。实际 VC runtime 版本及哈希见 `microsoft-vc-runtime-binaries.json`，D3D compiler 为 `10.0.20348.1`，哈希见 `angle-binaries.json`。原始 VC runtime 条款来自 [Microsoft Visual C++ Runtime 2015-2022](https://visualstudio.microsoft.com/license-terms/vs2022-cruntime/)，保留官方 `.docx` 及从中提取的 `.txt`。本机构建工具目录没有独立提供 d3dcompiler 的 SDK EULA 文件，因此记录 Microsoft 版权及上游来源，没有将其标为 MIT / BSD。',
  '',
  '## 精确锁定的 Dart / Flutter 包',
  '',
  ('以下 {0} 个包覆盖本次 pubspec.lock，包括构建 / 测试依赖和非目标平台的条件依赖；这不表示所有平台代码都编入了 Windows 或 Android。锁文件 SHA-256：`{1}`。完整文件哈希见 `licenses/pub-license-manifest.json`。' -f $manifest.package_count,$manifest.pubspec_lock_sha256),
  '',
  '| 包 | 版本 | 原始许可证文本 |',
  '|---|---|---|'
)
foreach ($group in ($manifest.notices | Group-Object package | Sort-Object Name)) {
  $first = $group.Group[0]
  $files = ($group.Group | ForEach-Object { "[$([IO.Path]::GetFileName($_.file))]($($_.file))" }) -join '、'
  $lines += "| $($first.package) | $($first.version) | $files |"
}
$notices = $lines -join [Environment]::NewLine
[IO.File]::WriteAllText((Join-Path $project 'THIRD_PARTY_NOTICES.md'), $notices, [Text.UTF8Encoding]::new($false))
$runtime = Join-Path $PSScriptRoot 'runtime'
New-Item -ItemType Directory -Path $runtime -Force | Out-Null
$text = [Text.StringBuilder]::new()
[void]$text.AppendLine($notices)
$all = @(Get-ChildItem -LiteralPath (Join-Path $PSScriptRoot 'pub') -Recurse -File)
$all += @(Get-ChildItem -LiteralPath (Join-Path $PSScriptRoot 'flutter') -File | Where-Object Extension -NE '.json')
$all += @(Get-ChildItem -LiteralPath (Join-Path $PSScriptRoot 'native') -File | Where-Object Name -Match '^(LICENSE|COPYING|COPYRIGHT|AUTHORS|Microsoft-VC-Runtime-License).*\.txt$')
foreach ($file in ($all | Sort-Object FullName)) {
  [void]$text.AppendLine()
  [void]$text.AppendLine('================================================================')
  [void]$text.AppendLine([IO.Path]::GetRelativePath($project, $file.FullName).Replace('\','/'))
  [void]$text.AppendLine('================================================================')
  [void]$text.AppendLine([IO.File]::ReadAllText($file.FullName))
}
[IO.File]::WriteAllText((Join-Path $runtime 'THIRD_PARTY_NOTICES.txt'), $text.ToString(), [Text.UTF8Encoding]::new($false))
[pscustomobject]@{packages=$manifest.package_count;source_archives=5;runtime_notice_bytes=(Get-Item -LiteralPath (Join-Path $runtime 'THIRD_PARTY_NOTICES.txt')).Length} | ConvertTo-Json
