# 映匣原生版 2.0.0

Windows 和 Android 使用同一套 Flutter 原生界面及 Go 后台。窗口内容由 Flutter 绘制，视频由 media_kit / libmpv 播放；这两个版本不使用 WebView2、Chromium、浏览器窗口或网页播放器。

Windows 保留搜索影片、播放工作台、收藏与记录、来源与诊断四页的深色布局。手机使用底部导航，搜索、选集和播放控件随屏幕宽度排列。播放页的空白提示为“选集之后请点击下方播放所选集”。离开播放工作台或应用进入后台时暂停播放。

清空记录使用界面自己的确认框。提交期间按钮禁用，失败时保留原列表；成功后重新读取保存结果，较早发出的读取请求不能把已删除的列表放回界面。收藏、来源设置和播放页绑定会保留。

Windows 默认沿用 `%LOCALAPPDATA%\YingXia\standalone` 数据。WebView 版与原生版共用数据锁，使用前先关闭另一个版本。原生 Windows 的单 EXE 首次启动会把经过哈希检查的播放器与界面运行文件释放到 `%LOCALAPPDATA%\YingXia\native-runtime-v2`；用户无需安装 Go、Flutter、Node、Python 或开发工具。

Android 最低支持 Android 8.0。Go 后台在手机应用私有目录中运行，不依赖电脑。APK 使用独立发布密钥签名；后续升级应继续使用同一密钥，私钥不在源码包内。

本轮两个 Windows 最终 EXE 已完成实际窗口运行检查。Android 正式 APK 在 API 35 x86_64 模拟器通过安装、四页界面、非空记录清空和重启持久化、Activity 快速替换检查；对应的发布模式测试 APK 通过真实画面的连续 HLS / WebM 解码、换集、反复停止重播、播放结束重播、切页和后台暂停。测试包与正式包的 12 个界面及播放器原生库一致，三个正式 Go JNI 均与发布 AAR 对应。具体证据见 `evidence/android/android_runtime_tests.json`；没有 Android 真机或 16KB 设备实跑结果。

原生版保留来源搜索、可用性检查、进度、收藏与历史、片源管理，以及 88lin / Kazumi 来源配置的更新检查。配置更新不会更换软件本身。能够直接提供媒体地址或静态提取媒体地址的来源可交给原生播放器；仅能通过动态网页播放的来源需要切换到其他片源。来源可访问的检查不能保证每一集、整个影片的网络状态始终相同。

## 开发与构建

依赖版本固定在 `pubspec.lock`；Go 依赖及许可证位于 `../native_core/vendor`。本轮使用 Flutter 3.47.7、Go 1.27.2、JDK 17、Android SDK 36、NDK 28.2.13676358、MSVC 14.44.35207、Windows SDK 10.0.26100.0。最终用户无需安装这些开发工具。

先按 `../native_core/README_zh.md` 构建 Windows 后台或 Android AAR，再执行 `flutter pub get`。Windows 可使用正常的 Flutter Windows 构建环境，也可使用本目录的 `build_windows_native.ps1` 配合便携 MSVC / CMake / Ninja。原生运行目录生成后，调用 `../native_core/windows_launcher/build_launcher.ps1 -BundleRoot <运行目录>` 生成单文件 EXE。脚本会拒绝覆盖已发布的同名文件。

已验证的 Windows 2.0.0 对应 Flutter 源码另存于 `windows_v2.0.0_source`，保留其源文件哈希。重新构建该已发布 Windows 版本时，在项目副本中使用这个快照的 `lib` 和依赖文件；后续 Android 配置调整不覆盖该 Windows 快照。

Android 的正式签名和构建步骤见 `android/README_build_zh.md`。源码包不包含 SDK、编译缓存、签名私钥、密码、本机路径配置或用户收藏记录。合成影片只用于测试构建，不进入发布后台和正式 APK。

`flutter analyze lib test` 与 `flutter test` 检查 Dart 界面。十一项回归覆盖清空确认、取消、保存失败、旧读取返回、可用片源与集数选择、动态网页来源提示、手机四页布局、选集期间的异步竞争，以及播放器频繁回调和前后跳转时的进度采样。实际 Windows / APK 运行证据另存于 `evidence`，不能用界面测试替代安装和视频解码测试。

本应用源码使用 GPL-3.0-or-later。依赖许可和播放器二进制对应的构建来源见 `THIRD_PARTY_NOTICES.md` 与 `licenses`。
