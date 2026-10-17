# 映匣 Android 原生版 2.3.0

安装 `YingXia_v2.3.0_Android.apk` 后可在手机、平板或 Android TV 独立运行。界面使用 Flutter 原生控件，Go 后台通过 JNI 在应用内启动，视频由 media_kit / libmpv 解码播放。最低 Android 8.0（API 26），通用 APK 包含 `arm64-v8a`、`armeabi-v7a`、`x86_64` 三种 ABI；沿用既有包名和发布签名更新可保留应用数据。

2.3 在播放区域与全屏采用一致的控制条。按住播放区域可临时 2 倍速，松开恢复原倍速；也可选择固定倍速。播放设置提供“标准／提前缓存”，显示已缓冲进度并限制内存占用；接力缓存按需预取后续分片，关闭磁盘缓存。HLS 广告跳过按清单的明确标记处理，可在设置关闭。播放卡顿时可检查版本、季数和集数一致的其他来源，用户确认后保留当前集数与播放位置切换。

搜索支持中文近似输入、全拼和首字母联想；本机索引来自实际搜索结果、收藏和历史，最多 2000 条、2 MiB。允许联网后可补充爱奇艺公开片名联想，首字母对应多个片名时由用户选择。收藏和历史入口直接打开最近选择的集数与有效断点；已结束或临近片尾时从该集开头播放。清空历史同时清除断点，保留收藏。

Android TV 模式提供电视启动入口、放大控件、方向键焦点与播放区遥控按键；平板与大屏按可用尺寸调整布局。华为 MatePad Pro（Android 16）的真机兼容性、持续播放、功耗与超分性能验证仍待该设备实测。

2.3 验证材料位于 `evidence/v2_3`，最终 APK 哈希和实际运行范围以相应 2.3 发布验证记录为准。源码归档使用 `tools/package_android_v2_3.py`，可先运行 `--audit-sources` 审计正式 AAR、依赖许可与源码输入。下文保留 2.2 基线的功能、来源、图形路径和构建记录；其中测试结果按原版本、设备与环境解读。

## 使用与功能

搜索片名后，等待逐片访问检测，核对作品版本和片源，再选集并点击“播放所选集”。搜索显示来源与媒体检测进度，可取消；可访问结果优先，进入工作台时保留已检测的集数。网页型来源提示切换片源。工作台提供切集、播放/暂停、停止重播、倍速、全屏、来源切换与平台绑定。

内置 47 个来源：既有无损云 1 个、固定 Kazumi 规则 16 个、从 2026-10-15 来源表筛选的 CMS 30 个。来源与诊断页统一提供查找、状态筛选、分页、启停、自定义来源添加/编辑/删除、单源/全部测速、进度和取消，以及手动和自动更新。更新保留自定义来源、开关与测速记录。测速反映目录接口响应；搜索中的逐片媒体检测继续判断具体播放入口。

47 是本版源码内置数量。运行时的上游更新与自定义添加会改变实际来源数量，以来源管理页当前列表为准。

手机竖屏、窄屏和短横屏使用底部导航；播放状态、超分说明、默认设置与来源控件按可用宽度换行。空白播放区仅显示选集提示，下方按钮是实际播放入口。切页或进入后台暂停，返回前台等待用户继续播放。Android 停止操作等待解码器确认并保留视频表面；重播等待停止完成，加载期的旧回调不写入新影片进度。

收藏、历史和设置保存在 Android 应用私有目录，与电脑数据分开。清空记录先确认，取消或写入失败时保留原记录；清空后收藏保留，重启后读取持久化结果。沿用相同包名与发布签名安装更新可保留应用数据；卸载应用会由 Android 清除其私有数据。

点击“导出诊断”会打开 Android 系统“保存到文件”，可选择保存位置，默认文件名为 `yingxia_diagnostics.json`；取消会安静返回。报告使用后台已脱敏的诊断和来源状态，不含本机会话凭据。此功能不申请存储权限。

开发人员名单保留在来源与诊断顶部：

- 翻翻蓄小激光 · cqh.vict1238@gmail.com
- 流风驻尘闲 · yuchihaobo@gmail.com

## 超分设置

超分默认关闭。工作台临时切换“关闭／轻量／增强”只影响本次使用；在来源与诊断设置“默认超分”，下次启动才采用该档位。默认值单独保存在应用私有目录的 `video_quality.json`，旧版临时保存的选择迁移为关闭。

轻量使用双线性放大与低强度 FSR1 RCAS；增强使用 EASU 与 RCAS。放大目标按视频比例和实际播放区计算，最高 1920×1080。没有放大需求时保留原尺寸；持续 VO 掉帧或 GPU 处理耗时超出预算时逐档下降。没有加入插帧。

Android GPU 路径调整真实原生 Surface 尺寸，并核对 mpv 实际输出尺寸与正在执行的着色器阶段。解码输出不支持、HDR 输入、着色器未执行或输出尺寸无法核实时自动回退关闭，并在界面显示原因。模拟器默认使用兼容的 `mediacodec_embed` 输出，因此请求超分时会明确回退关闭。手机默认使用播放器 GPU 输出；实际可用档位取决于设备图形驱动与解码路径。

FSR 算法、AMD MIT 许可和固定提交见 [着色器说明](assets/shaders/README_zh.md)、[来源清单](licenses/fsr1/source-manifest.json) 与 [历史算法证据说明](evidence/fsr_shader/README_zh.md)。Windows 的 RTX 4060 结果仅属于历史桌面验证。Android ARM 真机持续性能、耗电与发热仍需实测；本轮实际运行范围以发布验证记录为准。

## 源码、构建与验证

客户端在本目录，Android 后台在 [`../native_android_core`](../native_android_core/README_zh.md)。业务与来源配置继承 Windows 2.2 独立分支，旧项目和历史发布件保持原样。来源筛选的时间、取舍、原报告哈希与公开出处见 [来源证据](evidence/windows_2_2/source_screening/SCREENING_zh.md)；本轮未重新检查所有外网来源。

构建工具为 Flutter 3.47.7、Dart 3.13.5、Go 1.27.2、JDK 17、Android SDK 36 与 NDK `28.2.13676358`。依赖固定在 `pubspec.lock`、Go `go.mod` / `vendor` 和 Gradle 文件。先按后台 README 构建正式 AAR，再构建 APK：

```powershell
# 在 native_android_core 中，工具链放在源码包之外。
./build_android_aar.ps1 -ToolsRoot <外置工具目录>

# 在 native_android_client 中。
flutter pub get
$env:YINGXIA_ANDROID_KEY_PROPERTIES = '<源码外的发布签名 properties 路径>'
./tools/build_android.ps1 -Online
```

私有 properties 包含 `storeFile`、`storePassword`、`keyAlias`、`keyPassword`。构建辅助脚本允许用 `YINGXIA_FLUTTER`、`JAVA_HOME`、`ANDROID_HOME` 指定外置工具；离线环境须另行准备依赖缓存。默认生产构建从 `../native_android_core/build/yingxia_core.aar` 复制正式 AAR。具体原生宿主流程见 [Android 构建说明](android/README_build_zh.md)。

```powershell
flutter analyze
flutter test
```

单元/控件测试使用隔离数据，覆盖 47 源管理、320/360 像素及横屏布局、默认值独立性、访问检测、清空记录与播放策略。本轮分析与测试日志、安装和解码实测分别保存在 `evidence/android`；最终软件 SHA-256、检查结果与限制见源码包顶层发布验证 JSON。

`-Fixture` 仅生成独立测试包名的合成媒体 APK；`-Fixture -QaGpu` 再启用 GPU 模拟器探针。正式包不启用这些选项。[GPU 采证工具](tool/android_gpu/README.md) 仅保存脱敏指标，不读取应用私有文件。

源码归档工具为 `tools/package_android_release.py`，仅打包时需要 Python 3.11 或更新版本。它核对最终生产 APK、正式 AAR、逐文件 SHA-256 与 ZIP CRC，拒绝覆盖已有归档。源码包不包含 SDK、构建缓存、签名私钥、用户数据或测试 APK；保留用于直接构建的正式 AAR、依赖源码、原始许可证及核验记录。许可详情见 [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)。

拼音转换依赖固定为 go-pinyin 0.21.0（MIT）；原始许可见 `licenses/go/github.com/mozillazg/go-pinyin/LICENSE`，依赖版本和完整第三方说明见 `THIRD_PARTY_NOTICES.md`。
