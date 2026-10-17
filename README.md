# 映匣 YingXia

映匣是一款多来源影视检索与播放客户端，提供 Windows 原生版和 Android 版。界面使用 Flutter 原生控件，Go 后台负责来源配置、搜索与访问检查，media_kit / libmpv 负责视频播放。

## 下载

当前版本：**2.3.0**。

| 平台 | 下载 | 运行要求 |
|---|---|---|
| Windows 原生版 | [YingXia_v2.3.0_Native_Windows_x64.exe](https://github.com/vict1238/YingXia/releases/download/v2.3.0/YingXia_v2.3.0_Native_Windows_x64.exe) | Windows x64；双击运行 |
| Android | [YingXia_v2.3.0_Android.apk](https://github.com/vict1238/YingXia/releases/download/v2.3.0/YingXia_v2.3.0_Android.apk) | Android 8.0 / API 26 及以上；ARM64、ARMv7、x86_64 |

[版本说明与校验文件](https://github.com/vict1238/YingXia/releases/tag/v2.3.0) · [全部版本](https://github.com/vict1238/YingXia/releases) · [更新日志](CHANGELOG.md)

Windows 原生版首次启动会释放并校验随包运行文件，无需安装 Python、Go、Node 或 WebView2。升级前先退出旧版映匣；Windows 原生版与 WebView 版共用 `%LOCALAPPDATA%\YingXia\standalone` 中的收藏、历史与来源设置。Android 官方 APK 沿用原包名和签名，可覆盖安装；其数据保存在应用私有目录，与电脑数据分别保存。

## 2.3.0 的主要功能

- **检索与来源管理**：中文近似片名、完整拼音和首字母联想；搜索显示来源与媒体检查进度，可取消，并优先展示可访问结果。来源可启停、测速和更新，也可增删改自定义 CMS 接口。
- **播放与续播**：播放区和全屏共用音量、倍速、画面比例、快进快退、缓存与超分控件；收藏和历史记住最近集数与有效断点。清空历史同时清除断点，保留收藏。
- **缓存与换源**：“标准／提前缓存”限制内存占用，按需预取分片；卡顿时提示检查同一作品、年份、季数与集数的其他来源，确认后从当前位置继续。
- **HLS 标记广告跳过**：依据清单中的明确广告标记处理，可在播放设置中关闭。未标记广告与画面内嵌广告不在此识别范围内。
- **可选 FSR1 超分**：关闭、轻量、增强三档，启动默认关闭；可单独设置下次启动的默认档位，输出最高 1920×1080，持续渲染压力下自动降档。实际可用档位取决于设备输出与图形驱动。
- **Android 操作**：长按画面临时 2 倍速、松开恢复；提供 Android TV 启动入口、方向键焦点和大屏控制条。

原生版播放媒体直连或可静态提取的媒体地址；仅提供动态网页的来源会提示更换片源。来源响应、影片访问检查与实际完整播放是不同状态，列表中的测速表示目录接口响应速度。

## 版本与源码

| 版本 | 状态 | 界面与播放方式 | 源码 |
|---|---|---|---|
| Windows 原生 2.3.0 | 当前主线 | Flutter、libmpv | `native_windows_client/` + `native_windows_core/` |
| Android 2.3.0 | 当前主线 | Flutter、JNI Go 后台、libmpv | `native_android_client/` + `native_android_core/` |
| Windows WebView 1.3.2 | 历史版本 | Win32 窗口、WebView2、网页播放器；需 Microsoft WebView2 Runtime | `current_source/` |
| 原生 2.0.0 | 历史版本 | Flutter 与共享 Go 后台；含 Windows 2.0 冻结快照 | `native_client/` + `native_core/` |
| Python 1.0 原型 | 历史源码 | Python 3.10+ 与已安装的 Edge / Chrome | `legacy_python/` |

```text
native_windows_client/   当前 Windows 界面、播放器修改与单 EXE 启动器
native_windows_core/     当前 Windows Go 后台
native_android_client/   当前 Android 界面、宿主与 APK 构建脚本
native_android_core/     当前 Android Go/JNI 后台与 AAR 构建脚本
current_source/          历史 Windows WebView 1.3.2
native_client/           历史原生 2.0；windows_v2.0.0_source/ 为冻结快照
native_core/             历史原生共享后台
legacy_python/           Python 原型
source_audit/            上游参考源码与许可
releases/                发布说明、打包清单与验证记录
docs/BUILD.md            工具要求、构建命令与自有 Android 签名
```

从源码构建请阅读 [构建说明](docs/BUILD.md)。当前发布使用 Flutter 3.47.7、Dart 3.13.5、Go 1.27.2；Windows 还需 MSVC / Windows SDK，Android 还需 JDK 17、SDK 36 和 NDK 28.2。依赖版本记录在各项目的锁文件及第三方说明中。

发布源码归档：[Windows](https://github.com/vict1238/YingXia/releases/download/v2.3.0/YingXia_v2.3.0_Native_Windows_Source.zip) · [Android](https://github.com/vict1238/YingXia/releases/download/v2.3.0/YingXia_v2.3.0_Android_Source.zip)。实际验证范围见 [Windows 2.3 记录](releases/YingXia_v2.3.0_Native_Windows_VALIDATION.json) 和 [Android 2.3 记录](releases/YingXia_v2.3.0_Android_VALIDATION.json)；其中模拟器验证不代表 Android ARM 真机、电视或特定 GPU 的持续性能。

## 来源配置更新

2.3 源码内置 47 个来源：30 个经筛选的 CMS、16 个 Kazumi 规则与 1 个既有目录。运行时更新和自定义添加会改变数量，以来源管理页为准。

- 跟踪 [88lin/video_vip](https://github.com/88lin/video_vip) 的目录与解析配置、[KazumiRules](https://github.com/Predidit/KazumiRules) 的规则，以及 [Kazumi](https://github.com/Predidit/Kazumi) 的协议兼容信息。
- 更新只读取支持的配置字段与规则，不执行远程用户脚本、Spider 或 JAR；CMS 种子随应用版本维护。
- 更新保留自定义来源、启停设置与测速记录；失败时继续使用原配置。规则协议需要新适配时提示升级应用，应用程序通过新版本发布。
- 可在“来源与诊断”手动更新或关闭自动更新；关闭联网后停止发起新的来源更新请求。

## 开发人员

- **翻翻蓄小激光** · [cqh.vict1238@gmail.com](mailto:cqh.vict1238@gmail.com)
- **流风驻尘闲** · [yuchihaobo@gmail.com](mailto:yuchihaobo@gmail.com)

## 许可与第三方来源

当前原生版和 WebView 版的映匣应用源码按 **GPL-3.0-or-later** 发布，见现有 [GPL 许可文本](current_source/LICENSE.txt) 与各目录说明；Python 原型按其 [GPL-3.0 说明](legacy_python/THIRD_PARTY_NOTICES.md) 保留。第三方代码、规则和运行组件分别适用各自许可证。

原生版包含 Flutter / Dart、media_kit、libmpv、FFmpeg、AMD FSR1、Go 依赖及来源规则的许可与来源记录；Windows 随包 Microsoft 运行组件适用其可再发行条款。详细版本、上游源码、修改与构建配方见 [Windows 第三方说明](native_windows_client/THIRD_PARTY_NOTICES.md)、[Android 第三方说明](native_android_client/THIRD_PARTY_NOTICES.md) 和 [WebView 第三方说明](current_source/THIRD_PARTY_NOTICES.md)。

