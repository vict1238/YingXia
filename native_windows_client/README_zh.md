# 映匣 Windows 原生版 2.3.0

运行 `YingXia_v2.3.0_Native_Windows_x64.exe`，启动前关闭旧版映匣。界面继续采用 Flutter 原生控件，视频由 libmpv 播放；收藏、历史与来源设置沿用原数据目录。

2.3 的播放设置提供“标准／提前缓存”，按时间和内存上限提前读取；接力缓存按需预取后续分片，关闭磁盘缓存。播放器与全屏使用一致的控制条，提供倍速、缓冲进度和播放设置。卡顿时可检查同一作品、年份、季数和集数的其他来源，用户确认后保留当前位置切换。HLS 广告跳过按清单的明确标记处理，可在播放设置关闭。

搜索支持中文近似输入、全拼和首字母联想。本机片名索引来自实际搜索结果、收藏和历史，最多保存 2000 条、2 MiB；允许联网后可补充爱奇艺公开片名联想，首字母对应多个片名时由用户选择。收藏和历史入口会打开最近选择的集数与有效断点；已结束或临近片尾的记录从该集开头播放。清空历史同时清除断点，保留收藏。

本版验证材料位于 `evidence/v2_3`；最终软件哈希、实际运行范围与检查结果由相应 2.3 发布验证记录给出。下文保留 2.2 基线的功能、来源与构建记录，历史测试结果仍按其原版本和环境解读。2.3 源码归档使用 `tools/package_windows_v2_3.py`，可先运行 `--audit-sources` 审计源码、许可和隐私输入。

## 本版功能

“来源与诊断”统一管理片源：搜索和筛选、启停、添加和编辑自定义 CMS 接口、删除自定义来源、单源或全部测速，以及更新来源配置。批量测速显示进度并可取消。测速校验目录接口或静态搜索结果，显示接口响应时间；影片的媒体可用性仍由搜索时的检测判断。

新增片源由用户提供的 2026-10-15 表格筛选、去重，并通过实际接口请求验证后整合。接口与订阅的筛选范围、检查时间和结果见 `evidence/v2_2/source_screening`。内置来源的启停、自定义来源与测速状态在更新时保留。

超分默认关闭。在“来源与诊断”设置“默认超分”后，下次启动采用该设置；播放工作台的临时切档只影响本次使用。旧版自动保存的轻量档不会成为新版默认值。关闭、轻量和增强保留：轻量为双线性放大加低强度 RCAS，增强为 FSR1 EASU 加 RCAS，输出上限 1920×1080，持续渲染压力下自动降档。

播放空白区域保留选集提示，下方“播放所选集”为实际播放入口。开发人员名单继续位于来源与诊断顶部：

- 翻翻蓄小激光 · cqh.vict1238@gmail.com
- 流风驻尘闲 · yuchihaobo@gmail.com

收藏、记录、来源设置沿用 `%LOCALAPPDATA%\YingXia\standalone`。超分默认值单独保存在 `video_quality.json`。测试使用隔离目录，不修改用户的历史记录。

## 源码和构建

Windows 客户端为 `native_windows_client`，后台为独立迭代的 `native_windows_core`。此前 `native_core`、Android、WebView 与已发布的各版软件保留。Go 依赖固定在 `go.mod` / `vendor`，Flutter 依赖固定在 `pubspec.lock`。

工具：Flutter 3.47.7、Dart 3.13.5、Go 1.27.2、MSVC 14.44.35207、Windows SDK 10.0.26100.0。先按后台 README 构建 `native_windows_core/build/yingxia_backend.exe`，再在本目录执行 `flutter pub get`，使用 Windows Flutter 工具链或便携构建脚本：

```powershell
./build_windows_native.ps1 -ToolsRoot <便携工具目录>
./launcher/build_launcher.ps1 -BundleRoot ./build/windows/native_bundle -OutputExe ../releases/YingXia_v2.2.0_Native_Windows_x64.exe
```

单 EXE 包含运行库和许可证，首次启动释放到原生缓存并逐文件检查哈希。构建脚本拒绝覆盖已有交付文件。SDK、缓存、签名材料和用户数据不进入源码包。

FSR 和播放器渲染源码沿用 2.1.0，原算法、许可及实际 RTX 4060 合成视频验证保存在 `assets/shaders`、`licenses/fsr1` 和历史 evidence 中。本版变更的验证见 `evidence/v2_2` 和相应发布验证记录。实际交付沿用 2.1 已验证的原始播放器插件 DLL（SHA-256 `d740f1a4c311827191e6a21957b41c7fb57d1778cf6a83037271a401b7ac8dc3`），153 份相关源码与着色器逐字节一致。构建脚本的可选 `-ReuseRendererDll <该原始DLL路径>` 可复用这个固定组件；不使用该参数则正常从相同源码编译。核显持续帧率仍需实际设备反馈。

拼音转换依赖固定为 go-pinyin 0.21.0（MIT）；原始许可见 `licenses/go/github.com/mozillazg/go-pinyin/LICENSE`，依赖版本和完整第三方说明见 `THIRD_PARTY_NOTICES.md`。
