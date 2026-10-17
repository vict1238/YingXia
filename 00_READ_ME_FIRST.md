> 2026-10-16 最新交付：Windows 原生版与 Android 均为 **2.3.0**，软件、源码、许可与实际验证见 `releases/YingXia_v2.3.0_*`。本轮加入标记广告跳过、有限提前缓存、卡顿换源提示、收藏／历史断点续播、播放区控件和拼音／首字母／模糊搜索；Android 增加长按临时 2 倍速与电视大屏适配。Windows 56 项、Android 75 项 Flutter 测试通过，后台最新源码在隔离模拟器通过 37 项引擎测试、15 子例与 1 项生命周期测试。实际运行范围与失败尝试见发布验证记录，MatePad Pro Android 16 仍需真机复测。此前发布文件保持原字节。下文为历史版本说明。

> 2026-10-15 最新交付：**Android 原生版 2.2.0**。安装 `releases/YingXia_v2.2.0_Android.apk`（Android 8.0+，三 ABI，沿用 Android 2.0 包名和发布签名，可覆盖更新）。本轮按 Windows 2.2 移植全部业务与来源功能，独立源码为 `native_android_client/`、`native_android_core/`；Windows 2.2 及所有旧交付保持原样。

内置47来源（30 CMS、16 Kazumi、1既有目录），保留自动更新、来源测速和完整管理、访问检测与搜索进度、选集/倍速/全屏、收藏历史和暂停。超分默认关闭，临时档位与启动默认值分开；Android GPU 路径调整真实 Surface 并核实 FSR pass，支持自动降档和设备能力回退。模拟器正常使用兼容视频输出并关闭超分。诊断可通过 Android 系统保存窗口导出 JSON。

分析与49项Dart测试通过；Android实际Go引擎14项、5子例与1生命周期测试通过。正式APK和隔离合成媒体实测涵盖页面、默认值、更新、来源管理、HLS可见画面、进度、切集、暂停、停止重播和清空持久化。GPU探针的具体条件、实际结果及ARM/16KB设备/功耗未测边界见发布验证JSON。交付原字节未在验收后重编。

源码、许可和哈希：`releases/YingXia_v2.2.0_Android_Source.zip`、`releases/YingXia_v2.2.0_Android_README_zh.md`、`releases/YingXia_v2.2.0_Android_VALIDATION.json`、`releases/YingXia_v2.2.0_Android_PACKAGING.json`。下文是历史版本记录。

> 2026-10-15 最新交付：**Windows 原生版 2.2.0**。运行 `releases/YingXia_v2.2.0_Native_Windows_x64.exe`，先退出旧版。当前源码为 `native_windows_client/` 与独立后台 `native_windows_core/`；此前共享后台、WebView、Android 和旧发布文件均保留。

本版从来源表筛选新增30个CMS，总计47个来源，并更新Kazumi API9规则兼容。来源与诊断统一提供搜索/筛选、分页、启停、自定义来源编辑删除、单源/全部测速、进度与取消、更新与自动更新入口。测速显示目录接口响应，原有搜索中的逐片媒体检测继续保留。

超分默认关闭；在来源与诊断设置显式默认档位，下次启动生效，工作台临时切档不改默认值。播放空屏的假播放图标已删除，文字提示和下方实际播放按钮保留。开发人员名单保留。

30项Dart检查、Go后台测试与vet、12项正式后台接口检查、同一最终EXE分批8项窗口检查通过。来源表8张表166记录均有筛选依据；30个新增CMS经过搜索及详情验证。本版没有重复GPU/影片解码实测。说明、完整源码、许可、哈希和原始验证分别见 `releases/YingXia_v2.2.0_Native_Windows_README_zh.md`、`releases/YingXia_v2.2.0_Native_Windows_Source.zip`、`releases/YingXia_v2.2.0_Native_Windows_VALIDATION.json` 和 `releases/YingXia_v2.2.0_Native_Windows_PACKAGING.json`。下文是此前版本历史。

> 2026-10-14 最新交付：Windows 原生版 **2.1.0**，源码为 `native_windows_client/`，共享 Go 后台 `native_core/` 保持原样。运行 `releases/YingXia_v2.1.0_Native_Windows_x64.exe`，启动前关闭旧版映匣。

播放工作台提供“关闭／轻量／增强”，默认轻量。轻量使用双线性放大与 FSR1 RCAS，增强使用 EASU 与 RCAS；按播放区域和比例输出，最高 1920×1080。持续渲染掉帧或 GPU 耗时超出预算时逐档下降。来源与诊断顶部已加入翻翻蓄小激光（cqh.vict1238@gmail.com）和流风驻尘闲（yuchihaobo@gmail.com）。

最终 EXE 的同一字节完成 22 项实际窗口检查（9 项生产后台、13 项隔离合成视频，4 份分批原始报告），三档实际 GPU 执行和 720p24/30/60、HLS、暂停恢复、清空记录及播放中退出均有证据。27 项控制器/界面检查与着色器 RGB 校验通过。实测显卡为 RTX 4060 Laptop GPU；核显吞吐、旧 GLES2 上下文和 HDR 尚未实测；自动降档由受控性能数据测试验证。

交付文件、源码与许可证：`releases/YingXia_v2.1.0_Native_Windows_Source.zip`（881 文件，CRC 及解压后 SHA-256 已核验）。说明与证据见 `releases/YingXia_v2.1.0_Native_Windows_README_zh.md`、`releases/YingXia_v2.1.0_Native_Windows_VALIDATION.json`、`releases/YingXia_v2.1.0_Native_Windows_PACKAGING.json`。此前 WebView 1.3.2、Windows 原生 2.0、Android 2.0 软件及其源码交付保留；下文是历史背景。

> 2026-10-14 最新状态：Windows 原生版已迭代至 **2.1.0**。本轮仅修改 `native_windows_client/`，WebView 1.3.2、Windows 原生 2.0、Android 2.0 及原三版合集均保留。

新版软件：`releases/YingXia_v2.1.0_Native_Windows_x64.exe`（60,420,096 字节；SHA-256 `6e799468cf8d4ca744b44d2fac915dfa5f58e9982585c8b80c2e2cd61909d08c`）。源码：`releases/YingXia_v2.1.0_Native_Windows_Source.zip`（881 个文件，CRC 与全部解压文件哈希均已验证）。交付说明、实际证据和归档记录分别见 `releases/YingXia_v2.1.0_Native_Windows_README_zh.md`、`releases/YingXia_v2.1.0_Native_Windows_VALIDATION.json`、`releases/YingXia_v2.1.0_Native_Windows_PACKAGING.json`。

超分提供“关闭／轻量／增强”，默认轻量。轻量使用双线性放大加温和 RCAS，增强使用 EASU 加 RCAS；实际输出最大为 1920×1080，不需放大时保留原尺寸。持续渲染掉帧或 GPU 耗时过高时逐档下降，不加入插帧。保留简洁界面和原有清空记录修复，“来源与诊断”顶部新增：翻翻蓄小激光 `cqh.vict1238@gmail.com`、流风驻尘闲 `yuchihaobo@gmail.com`。

同一 SHA 的最终 EXE 分四批完成 22 项实际检查（18＋1＋2＋1），含正式后台 9 项与隔离合成播放 13 项；发布时直接复制通过验收的字节。实际 RTX 4060 Laptop GPU 上观察到 720p SDR 视频输出至 1080p、EASU／RCAS 执行、HLS、暂停恢复、反复切档与播放中退出。另有 27 项 Dart／控制器／界面测试、无问题分析及 ANGLE RGB 算法核验。核显吞吐、真正的旧 GLES2 上下文和 HDR 未实测；自动降档由可控输入的控制器测试验证。最新开发主线为 `native_windows_client/`，共享 Go 后台 `native_core/` 不变。Windows 各版共用数据与互斥，运行新版前先退出旧版。

> 2026-10-14 历史状态（上一轮三版交付）：三个修好“清空记录”的版本均已生成并完成以下实际验证。源码、许可证、最终哈希和范围随合集交付。

| 版本 | 软件 | 实际验证 |
|---|---|---|
| WebView Windows 1.3.2 | `releases/YingXia_v1.3.2_Windows_x64.exe` | 最终 EXE 清空专项 10 项；Go 197 PASS、3 SKIP；vet 和相同哈希重建通过。 |
| Windows 原生 2.0.0 | `releases/YingXia_v2.0.0_Native_Windows_x64.exe` | 无网页内核；实际窗口检查 10 项，含正式后台 7 项与合成播放后台 3 项。 |
| Android 原生 2.0.0 | `releases/YingXia_v2.0.0_Android.apk` | API 35 x86_64 模拟器共 21 项：正式包 8 项及同版界面/播放器的合成播放包 13 项，包含清空重启、实际 HLS/WebM 画面、换集、停止重播、结束重播和暂停。 |

清空确认、取消、失败重试、旧读取返回及清空前未结束解析重新写入历史的情况均已修复。收藏、绑定、设置和原 `library.json` 格式保留；Windows 两版共用原数据目录及互斥，切换前关闭另一版。测试使用隔离数据，没有清空用户 Windows 记录。Android 使用应用私有目录，不依赖电脑后台。

合集为 `releases/YingXia_Three_Editions_20261014.zip`；对应源码为 `releases/YingXia_v1.3.2_Windows_x64.zip` 和 `releases/YingXia_v2.0.0_Native_Source.zip`。最终说明见 `releases/YingXia_Three_Editions_README_zh.md`，实际证据与哈希见 `releases/YingXia_Three_Editions_VALIDATION.json`、`project_state.json`。已发布 Windows 原生版的 Flutter 源码保存在 `native_client/windows_v2.0.0_source`，后续 Android 修正不覆盖该快照。

Android 真机、16KB 页设备和可听音质尚未实测；该轮未验证所有第三方影片。原生版遇到仅能动态网页播放的来源需切换其他媒体片源。重复时间戳分段样本在模拟器 MediaCodec 的异常保留为诊断，最终正常连续 HLS 的跨段和完整播放检查通过。该轮 2.0 版本未加入超分；Windows 原生 2.1 的实现见顶部。签名私钥、SDK、缓存及用户数据不进入交付包。

> 2026-10-14 历史更新：当时主线 v1.3.0-exe，映匣使用自己的 Windows 窗口和内嵌 WebView2，主界面不再启动 Edge/Chrome 应用模式窗口。电脑需有 Microsoft WebView2 Runtime；完整运行时不随 EXE 打包。保留原界面、收藏、历史及来源功能。部分网页动态取流仍使用独立临时桌面浏览器。

> 2026-10-14 历史更新：当时主线 v1.2.1-exe，增加切页暂停、调整播放区提示、移除全局粘贴入口；该历史版本为 Go 后台 + Edge/Chrome 应用窗口。

> 2026-10-14 历史更新：当时主线 v1.2.0-exe，搜索增加访问检查、排序与进度。交付在 releases，说明见 current_source/README_zh.md；原根目录 MANIFEST 和交接记录描述历史基线。

> 2026-10-13 历史更新：当时主线为 v1.1.0-exe，交付文件在 releases。说明见 current_source/README_zh.md 与 current_source/docs/VALIDATION.md。下文保留 2026-10-12 原始交接背景。

# 映匣 / YingXia · Work 项目交接包（2026-10-12 历史基线）

原始交接日期：2026-10-12。当时主线：**1.0.1-exe**，对外文件名为 v1.0。以下正文保留当时的工程背景，最新主线与验收范围见本文顶部。

## 用途

这是可独立阅读、继续开发的工程交接包：包含本项目的对话要点、已确认需求、技术决策、当前源码、既有 EXE、历史版本、测试证据与待办。它不是平台导出的完整聊天数据库，不依赖原聊天链接、旧 sandbox 路径或原对话的隐藏状态。

将整个 ZIP 提供给接手对话，并粘贴 `01_PROMPT_FOR_WORK.txt`。用户无需解读源码，也无需安装 Python。接手模型应实际读取附件，不要只根据提示词描述推断工程状态。

## 阅读顺序

1. `02_PROJECT_CONTEXT.md`：用户目标、既定约束、版本演变。
2. `03_CURRENT_STATE_AND_LIMITATIONS.md`：实现与验证边界。
3. `04_CODE_MAP_AND_BUILD.md`：主线代码、数据、接口和构建。
4. `05_NEXT_STEPS_AND_ACCEPTANCE.md`：接手动作、优先级、验收。
5. `current_source/README_zh.md`、`current_source/docs/VALIDATION.md`：原交付说明。
6. `07_HANDOFF_VERIFICATION.md` 与 `handoff_verification/`：本次新增核验。

**事实优先级：实际文件与运行证据 > 当前主线说明 > 本次项目摘要 > 历史聊天中的计划。** 历史报告只能证明当时、当地、该测试范围内的观察；它们不是现在线路健康证明。

## 不要弄混的三个版本

| 位置 | 身份 | 接手方式 |
|---|---|---|
| `current_source/` | 已交付 Windows EXE 的 Go 源码 | 继续开发的主线 |
| `releases/YingXia_v1.0_Windows_x64.exe` | 原交付 Windows x64 EXE | 保留基线；不要在不支持的环境谎称运行成功 |
| `legacy_python/` | 早期 Python v1.0 | 历史对照，不是当前运行依赖 |

早期曾讨论 PySide6 + Qt WebEngine，但没有作为最终实现。最终界面通过已安装 Edge/Chrome 显示，不内置浏览器。

## 目录速查

- `current_source/`：完整 Go 工程、Web 界面、合成测试视频、构建脚本、测试与许可。
- `legacy_python/`：解开的早期源码、旧指南、旧测试和截图。
- `source_audit/`：88lin 来源与匹配预检、测试工具、原始结果。
- `releases/`：直接可交付的原 EXE。
- `original_deliverables/`：此前的三个 ZIP 和两份 HTML 原件，未改动。
- `history_screenshots/`：单独上传的中间版/独立版截图；不是 Windows 实播证据。
- `handoff_verification/`：本次回归日志、静态检查、重建一致性证据。
- `project_state.json`、`MANIFEST.json`、`SHA256SUMS.txt`、`FILE_TREE.txt`：状态与清单。

## 三个关键结论

原始交接时的 EXE 无需用户安装 Python 或 Go，但需要本机已有浏览器。当时 Windows 真机运行尚未验收，真实第三方影片播放也未验收。网络代码和界面已经实现，不等于全部来源或全部影片可用。

原始交接核验没有修改应用源码。重新编译所得 EXE 与原交付文件逐字节一致；离线 Go 回归通过。详见当时核验报告。
