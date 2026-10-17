# 映匣原生版第三方组件说明

本目录保存构建实际使用的依赖版本、原始版权和许可证文本、下载来源、二进制校验以及播放器构建配方。映匣代码使用 GPL-3.0-or-later；每个第三方组件保留自己的许可证。此文件不改变软件界面。

本次交付为 Windows 原生版 2.3.0。独立客户端与 Go 后台源码随交付源码包提供。下文保留完整依赖来源记录。

## Windows 2.1.0 FSR 1 与播放器修改

画质增强改编自 AMD 官方 [FidelityFX-FSR](https://github.com/GPUOpen-Effects/FidelityFX-FSR/tree/a21ffb8f6c13233ba336352bdff293894c706575)，固定提交 `a21ffb8f6c13233ba336352bdff293894c706575`，MIT，Copyright (c) 2021 Advanced Micro Devices, Inc. 原始许可随运行包保存在 `licenses/fsr1/LICENSE_AMD_FSR1.txt`；源码包同时保存 `assets/shaders/upstream/ffx_fsr1.h`、`ffx_a.h`、许可证和两个移植后的执行着色器。具体 SHA-256 见 `licenses/fsr1/source-manifest.json` 和 `evidence/fsr_shader/shader_provenance.json`。

增强档执行 EASU 与 RCAS；轻量档执行双线性放大与低强度 RCAS。移植为 FP32 普通片段着色器，并按本机播放器输出尺寸处理。完整算法移植及运行条件见 `assets/shaders/README_zh.md`。

`media_kit_video` 2.0.1 使用本项目中的完整源码 `vendor/media_kit_video`，保留其原始 MIT 许可。本版只在这份本地源码中修改 Windows ANGLE 上下文、实际纹理尺寸、mpv 高级渲染更新 / 性能数据和释放顺序；变更文件与上游哈希见 `evidence/fsr_shader/shader_provenance.json`。下方锁定包许可证表从当前 `pubspec.lock` 和实际解析目录重新生成，保留本地依赖来源。

## Go 后台与来源规则

Windows 后台、原生启动器和 Android JNI 后台使用 Go 1.27.2；Go runtime / 标准库的 BSD-3-Clause 原文与专利授权见 `licenses/go/runtime/LICENSE`、`PATENTS`，工具链版本文件为 `VERSION`。

Go 后台依赖锁定为 htmlquery 1.3.7、xpath 1.3.9、ojg 1.28.7、groupcache `41bb18bfe9da`、golang.org/x/net 0.61.0、golang.org/x/text 0.43.0；Android gomobile 绑定使用 golang.org/x/mobile `v0.0.0-20261011034528-197770444867`。对应模块的原始 LICENSE / PATENTS 已按上游目录保存于 `licenses/go`；版本见 `licenses/go/modules.txt`，随附许可文件的哈希见 `manifest.json`。

16 个动漫来源规则来自 Predidit/KazumiRules 固定快照 `2fa4d28659f94e1eb52e7a38d79b7b9a5dcd605b`，MIT，Copyright (c) 2024 Predidit；完整原文见 `licenses/go/KazumiRules_LICENSE.txt`。88lin/video_vip 3.2.9 的目录与解析配置参考其 GPL-3.0 源码，本程序只读取配置字段。具体来源、快照与完整项目许可同时保存在随附源码的 `current_source/THIRD_PARTY_NOTICES.md`、`current_source/LICENSE.txt`；未嵌入 LibreTV 程序或 Kazumi 的 Flutter 客户端。

新增 CMS 接口来自用户提供的 2026-10-15 来源表及其中公开订阅，作为接口地址和来源元数据整合；筛选、去重、响应校验和原始出处保存在 `evidence/v2_2/source_screening`。订阅中的远程 Spider / JAR 不作为程序执行。

2.3 拼音检索新增 [go-pinyin v0.21.0](https://github.com/mozillazg/go-pinyin/tree/v0.21.0)，MIT，Copyright (c) 2016 mozillazg。原始许可保存在 `licenses/go/github.com/mozillazg/go-pinyin/LICENSE`；代码与字音数据固定在后台 `vendor`，`go.mod` / `go.sum` 记录版本与校验和。拼音联想使用本机真实元数据索引，并可在允许联网后查询爱奇艺公开片名联想；只读取片名，不导入其播放链接、图片或年份字段。

## Flutter 与 Dart

Flutter 3.47.7，framework commit `abaf9c523780a608bd46686fd5e53740a07077f8`；engine commit `deb287481e3ce9468f3434937ced4240a70539ca`，engine content hash `d8a2d6c768c334d53e16277f837970ffa1e2de81`；Dart 3.13.5。

Flutter / Dart 原文见 `licenses/flutter/LICENSE-Flutter.txt`、`LICENSE-Dart.txt`。Flutter engine 汇总原文见 `licenses/flutter/NOTICES-Flutter-engine.txt`。Material Icons、Roboto、Roboto Condensed 原始文本同时保存。生成的应用资产 `flutter_assets/NOTICES.Z` 还保留 Flutter 构建器汇总的依赖通知。

## 原生播放器

| 平台 | 实际原生组件 | 来源与构建记录 |
|---|---|---|
| Windows | libmpv v0.36.0-403-g652a1dd907；FFmpeg n6.0 | [media-kit 2023-09-24 二进制](https://github.com/media-kit/libmpv-win32-video-build/releases/tag/2023-09-24)，文件 `mpv-dev-x86_64-20230924-git-652a1dd.7z`；DLL 元数据见 `licenses/native/libmpv-windows-binary.json`，实际 FFmpeg configure 字符串见 `ffmpeg-windows-configure.txt`。 |
| Android | media-kit native default v1.1.7；mpv 78d43740f52db817d98bcf24fb30a76ab6fa13ff；FFmpeg 6.0 | [不可变构建配方 fe8c3ac1](https://github.com/media-kit/libmpv-android-video-build/tree/fe8c3ac1a91c09aa6fb1deccbc833f1bafa54768)，保存了 recipe、依赖版本和 FFmpeg / mpv 的补丁。APK 使用 default flavor，不使用 encoders-gpl。 |

两份配方均使用 `-Dgpl=false` 的 mpv，并将 FFmpeg 配置为 `--disable-gpl --disable-nonfree --enable-version3`。mpv 原始版权文件说明这个配置使用 LGPL-2.1-or-later；FFmpeg 的 `--enable-version3` 选择 LGPL-3.0。相应 LGPL / GPL 原文和 FFmpeg 的完整上游许可证说明均在 `licenses/native`。复制 GPL 原文是保留上游 source 中的备用配置文本，并不意味着本次启用了那些 GPL 编解码器。

源代码快照随附：`mpv-windows-652a1dd907.zip`、`mpv-android-78d43740.zip`、`ffmpeg-6.0.tar.xz`；构建配方快照为 `libmpv-windows-build-f5a6f879.zip`、`libmpv-android-build-fe8c3ac1.zip`。下载 URL、大小、SHA-256 见 `upstream-source-manifest.json`。Android recipe 中包含版本锁定及所应用的修改补丁。

Windows release tag 的树仅保存 `version`，不保存当次构建的完整源码锁定记录。随附 Windows builder snapshot 是 release 时间之前的最近 master commit `f5a6f879c9b8bef6a73e52c8c0f35e51636c96e5`，这个关联是根据发布时间推得；mpv 和 FFmpeg 版本则直接读取发布 DLL，不依赖该推断。播放器第三方依赖的配置由这个配方列出；不能据此声称复建后二进制逐字节相同。

## Windows 图形与 Microsoft 运行库

ANGLE 从 [v1.0.1 预构建包](https://github.com/alexmercerind/flutter-windows-ANGLE-OpenGL-ES/releases/tag/v1.0.1) 引入。实际 `libEGL.dll` / `libGLESv2.dll` 的 PE 版本为 `2.1.18844 git hash: 2693b03eba82`，其 [ANGLE BSD 原文](https://github.com/google/angle/blob/2693b03eba82/LICENSE) 和 AUTHORS 已保存。ANGLE 是本机 OpenGL / Direct3D 图形转换库；它不创建 HTML 页面、WebView 或浏览器窗口。

同时保留 SwiftShader、Vulkan loader、LLVM libc++、zlib 的原始许可证参考文本。其二进制及 PE 元数据、SHA-256 见 `angle-binaries.json`。SwiftShader / Vulkan / zlib / libc++ 的独立源代码版本没有在此 ANGLE 发布包中完整锁定；保存的文本标明各自上游来源，不将许可证参考版本冒充实际二进制版本。

Microsoft Visual C++ runtime 和 `d3dcompiler_47.dll` 为 Microsoft 的专有可再发行组件，并非映匣或 media-kit 的开源代码。实际 VC runtime 版本及哈希见 `microsoft-vc-runtime-binaries.json`，D3D compiler 为 `10.0.20348.1`，哈希见 `angle-binaries.json`。原始 VC runtime 条款来自 [Microsoft Visual C++ Runtime 2015-2022](https://visualstudio.microsoft.com/license-terms/vs2022-cruntime/)，保留官方 `.docx` 及从中提取的 `.txt`。本机构建工具目录没有独立提供 d3dcompiler 的 SDK EULA 文件，因此记录 Microsoft 版权及上游来源，没有将其标为 MIT / BSD。

## 精确锁定的 Dart / Flutter 包

以下 63 个包覆盖本次 pubspec.lock，包括构建 / 测试依赖和非目标平台的条件依赖；这不表示所有平台代码都编入了本次 Windows 软件。锁文件 SHA-256：`39cb0db8da3a9747efa9b22e2d012f13a3a67cb298771e8291c384eb75a84664`。完整文件哈希见 `licenses/pub-license-manifest.json`。`media_kit_video` 2.0.1 从本工程 `vendor/media_kit_video` 解析，保留上游 MIT 许可并包含本版 Windows 修改。

| 包 | 版本 | 原始许可证文本 |
|---|---|---|
| archive | 4.5.2 | [LICENSE](licenses/pub/archive-4.5.2/LICENSE)、[LICENSE-other.md](licenses/pub/archive-4.5.2/LICENSE-other.md) |
| args | 2.7.0 | [LICENSE](licenses/pub/args-2.7.0/LICENSE) |
| async | 2.13.1 | [LICENSE](licenses/pub/async-2.13.1/LICENSE) |
| boolean_selector | 2.1.2 | [LICENSE](licenses/pub/boolean_selector-2.1.2/LICENSE) |
| characters | 1.4.1 | [LICENSE](licenses/pub/characters-1.4.1/LICENSE) |
| clock | 1.1.3 | [LICENSE](licenses/pub/clock-1.1.3/LICENSE) |
| collection | 1.19.1 | [LICENSE](licenses/pub/collection-1.19.1/LICENSE) |
| crypto | 3.0.7 | [LICENSE](licenses/pub/crypto-3.0.7/LICENSE) |
| dbus | 0.8.0 | [LICENSE](licenses/pub/dbus-0.8.0/LICENSE) |
| fake_async | 1.3.3 | [LICENSE](licenses/pub/fake_async-1.3.3/LICENSE) |
| ffi | 2.2.0 | [LICENSE](licenses/pub/ffi-2.2.0/LICENSE) |
| ffi_leak_tracker | 0.1.2 | [LICENSE](licenses/pub/ffi_leak_tracker-0.1.2/LICENSE) |
| fixnum | 1.1.1 | [LICENSE](licenses/pub/fixnum-1.1.1/LICENSE) |
| flutter | 0.0.0 | [LICENSE](licenses/pub/flutter-0.0.0/LICENSE) |
| flutter_lints | 5.0.0 | [LICENSE](licenses/pub/flutter_lints-5.0.0/LICENSE) |
| flutter_localizations | 0.0.0 | [LICENSE](licenses/pub/flutter_localizations-0.0.0/LICENSE) |
| flutter_test | 0.0.0 | [LICENSE](licenses/pub/flutter_test-0.0.0/LICENSE) |
| flutter_web_plugins | 0.0.0 | [LICENSE](licenses/pub/flutter_web_plugins-0.0.0/LICENSE) |
| http | 1.6.0 | [LICENSE](licenses/pub/http-1.6.0/LICENSE) |
| http_parser | 4.1.2 | [LICENSE](licenses/pub/http_parser-4.1.2/LICENSE) |
| image | 4.12.0 | [LICENSE](licenses/pub/image-4.12.0/LICENSE)、[LICENSE-other.md](licenses/pub/image-4.12.0/LICENSE-other.md) |
| intl | 0.20.3 | [LICENSE](licenses/pub/intl-0.20.3/LICENSE) |
| leak_tracker | 11.0.2 | [LICENSE](licenses/pub/leak_tracker-11.0.2/LICENSE) |
| leak_tracker_flutter_testing | 3.0.10 | [LICENSE](licenses/pub/leak_tracker_flutter_testing-3.0.10/LICENSE) |
| leak_tracker_testing | 3.0.2 | [LICENSE](licenses/pub/leak_tracker_testing-3.0.2/LICENSE) |
| lints | 5.1.1 | [LICENSE](licenses/pub/lints-5.1.1/LICENSE) |
| matcher | 0.12.20 | [LICENSE](licenses/pub/matcher-0.12.20/LICENSE) |
| material_color_utilities | 0.13.0 | [LICENSE](licenses/pub/material_color_utilities-0.13.0/LICENSE) |
| media_kit | 1.2.6 | [LICENSE](licenses/pub/media_kit-1.2.6/LICENSE) |
| media_kit_libs_android_video | 1.3.8 | [LICENSE](licenses/pub/media_kit_libs_android_video-1.3.8/LICENSE) |
| media_kit_libs_ios_video | 1.1.4 | [LICENSE](licenses/pub/media_kit_libs_ios_video-1.1.4/LICENSE) |
| media_kit_libs_linux | 1.2.1 | [LICENSE](licenses/pub/media_kit_libs_linux-1.2.1/LICENSE) |
| media_kit_libs_macos_video | 1.1.4 | [LICENSE](licenses/pub/media_kit_libs_macos_video-1.1.4/LICENSE) |
| media_kit_libs_video | 1.0.7 | [LICENSE](licenses/pub/media_kit_libs_video-1.0.7/LICENSE) |
| media_kit_libs_windows_video | 1.0.11 | [LICENSE](licenses/pub/media_kit_libs_windows_video-1.0.11/LICENSE) |
| media_kit_video | 2.0.1 | [LICENSE](licenses/pub/media_kit_video-2.0.1/LICENSE) |
| meta | 1.19.0 | [LICENSE](licenses/pub/meta-1.19.0/LICENSE) |
| package_info_plus | 10.2.2 | [LICENSE](licenses/pub/package_info_plus-10.2.2/LICENSE) |
| package_info_plus_platform_interface | 4.1.0 | [LICENSE](licenses/pub/package_info_plus_platform_interface-4.1.0/LICENSE) |
| path | 1.9.1 | [LICENSE](licenses/pub/path-1.9.1/LICENSE) |
| petitparser | 7.1.0 | [LICENSE](licenses/pub/petitparser-7.1.0/LICENSE) |
| plugin_platform_interface | 2.1.8 | [LICENSE](licenses/pub/plugin_platform_interface-2.1.8/LICENSE) |
| posix | 6.5.2 | [LICENSE](licenses/pub/posix-6.5.2/LICENSE) |
| safe_local_storage | 2.0.6 | [LICENSE](licenses/pub/safe_local_storage-2.0.6/LICENSE) |
| sky_engine | 0.0.0 | [license](licenses/pub/sky_engine-0.0.0/license) |
| source_span | 1.10.2 | [LICENSE](licenses/pub/source_span-1.10.2/LICENSE) |
| stack_trace | 1.12.2 | [LICENSE](licenses/pub/stack_trace-1.12.2/LICENSE) |
| stream_channel | 2.1.4 | [LICENSE](licenses/pub/stream_channel-2.1.4/LICENSE) |
| string_scanner | 1.4.1 | [LICENSE](licenses/pub/string_scanner-1.4.1/LICENSE) |
| synchronized | 3.5.0 | [LICENSE](licenses/pub/synchronized-3.5.0/LICENSE) |
| term_glyph | 1.2.2 | [LICENSE](licenses/pub/term_glyph-1.2.2/LICENSE) |
| test_api | 0.7.12 | [LICENSE](licenses/pub/test_api-0.7.12/LICENSE) |
| typed_data | 1.4.0 | [LICENSE](licenses/pub/typed_data-1.4.0/LICENSE) |
| universal_platform | 1.1.0 | [LICENSE](licenses/pub/universal_platform-1.1.0/LICENSE) |
| uri_parser | 3.0.2 | [LICENSE](licenses/pub/uri_parser-3.0.2/LICENSE) |
| uuid | 4.6.0 | [LICENSE](licenses/pub/uuid-4.6.0/LICENSE) |
| vector_math | 2.4.3 | [LICENSE](licenses/pub/vector_math-2.4.3/LICENSE) |
| vm_service | 15.3.0 | [LICENSE](licenses/pub/vm_service-15.3.0/LICENSE) |
| wakelock_plus | 1.8.1 | [LICENSE](licenses/pub/wakelock_plus-1.8.1/LICENSE) |
| wakelock_plus_platform_interface | 1.7.0 | [LICENSE](licenses/pub/wakelock_plus_platform_interface-1.7.0/LICENSE) |
| web | 1.1.1 | [LICENSE](licenses/pub/web-1.1.1/LICENSE) |
| win32 | 6.4.0 | [LICENSE](licenses/pub/win32-6.4.0/LICENSE) |
| xml | 7.1.0 | [LICENSE](licenses/pub/xml-7.1.0/LICENSE) |
