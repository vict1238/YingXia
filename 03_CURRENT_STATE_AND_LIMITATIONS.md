> 2026-10-16 最新交付：Windows 原生版与 Android 均为 **2.3.0**，软件、源码、许可与实际验证见 `releases/YingXia_v2.3.0_*`。本轮加入标记广告跳过、有限提前缓存、卡顿换源提示、收藏／历史断点续播、播放区控件和拼音／首字母／模糊搜索；Android 增加长按临时 2 倍速与电视大屏适配。Windows 56 项、Android 75 项 Flutter 测试通过，后台最新源码在隔离模拟器通过 37 项引擎测试、15 子例与 1 项生命周期测试。实际运行范围与失败尝试见发布验证记录，MatePad Pro Android 16 仍需真机复测。此前发布文件保持原字节。下文为历史版本说明。

> 2026-10-15 最新交付：**Android 原生版 2.2.0**。安装 `releases/YingXia_v2.2.0_Android.apk`（Android 8.0+，三 ABI，沿用 Android 2.0 包名和发布签名，可覆盖更新）。本轮按 Windows 2.2 移植全部业务与来源功能，独立源码为 `native_android_client/`、`native_android_core/`；Windows 2.2 及所有旧交付保持原样。

内置47来源（30 CMS、16 Kazumi、1既有目录），保留自动更新、来源测速和完整管理、访问检测与搜索进度、选集/倍速/全屏、收藏历史和暂停。超分默认关闭，临时档位与启动默认值分开；Android GPU 路径调整真实 Surface 并核实 FSR pass，支持自动降档和设备能力回退。模拟器正常使用兼容视频输出并关闭超分。诊断可通过 Android 系统保存窗口导出 JSON。

分析与49项Dart测试通过；Android实际Go引擎14项、5子例与1生命周期测试通过。正式APK和隔离合成媒体实测涵盖页面、默认值、更新、来源管理、HLS可见画面、进度、切集、暂停、停止重播和清空持久化。GPU探针的具体条件、实际结果及ARM/16KB设备/功耗未测边界见发布验证JSON。交付原字节未在验收后重编。

源码、许可和哈希：`releases/YingXia_v2.2.0_Android_Source.zip`、`releases/YingXia_v2.2.0_Android_README_zh.md`、`releases/YingXia_v2.2.0_Android_VALIDATION.json`、`releases/YingXia_v2.2.0_Android_PACKAGING.json`。下文是历史版本记录。

# 当前状态与验证边界

## 当前基线

- 主线：`current_source/`，Go module 为 `yingxia.local/desktop`，内部版本 `1.0.1-exe`。
- 原程序：`releases/YingXia_v1.0_Windows_x64.exe`，7,226,368 字节。
- SHA-256：`e7d47fe49c185c1d682c52713150f1098a19beda61e6e70128ddba21768a238b`。
- 目标格式：PE32+ / x86-64 / Windows GUI。代码面向 Windows 10/11 x64；没有 Windows 真机运行证据。
- 运行不依赖用户安装 Python、Go、Node；不包含浏览器，需要电脑已装 Edge/Chrome。
- 未数字签名。安全提示和杀毒软件兼容性没有实机验收，不要求用户关闭防护。

## 现有功能与边界

| 能力 | 已实现内容 | 不能据此承诺 |
|---|---|---|
| 目录搜索 | 无损云 suggest + 详情接口，缓存、错误处理、同目录详情搜索回退 | 全网目录、所有作品、实时可播 |
| 版本选择 | 片名、年份、季数排序，用户核对候选 | 自动无误确认所有重名/翻拍/语言版本 |
| 多组片源 | 保留返回的全部分组，严格按选中集或集名匹配 | 没有对应集时自动找到可靠替代 |
| 平台入口 | 五个平台静态入口检索；官方搜索/手动导入兜底 | 完整动态站点目录、一次导入自动生成全剧 |
| 播放入口 | `media`、`page`、`external` 三种 | 每种编解码器、HLS、DRM 或每个嵌入页都能播放 |
| 线路选择 | 一条目录、七个解析配置；HLS 默认关闭 | 七条都有内置独立片名搜索 |
| 收藏与历史 | 本地 JSON、同集平台绑定、打开/播放证据记录 | 已实现 Python SQLite 自动迁移或所有精确断点续播 |
| 网络诊断 | 有公共对照、HTTP/JSON 与脱敏报告 | 实际播放、全片完整性、版权/安全认证 |
| 离线演示 | 两段自生成 WebM、合成作品与缺集场景 | 真实影视片源或真实在线检索样本 |

具体代码以 `service.go`、`catalog.go`、`models.go`、`web/app.js` 为准。

## 来源配置快照（不是本日联网验收）

| 来源 | 类型 | 当前配置 |
|---|---|---|
| 无损云 | 一条目录，含搜索、详情及网页播放器配置 | 状态：未实播验证 |
| 麒麟 | 需官方播放页的解析入口 | 启用，但未实播验证 |
| TXNQ | 同上 | 启用，但未实播验证 |
| 七哥 | 同上 | 启用，但未实播验证 |
| fongmi | 同上 | 启用，但未实播验证 |
| 冰豆 | 同上 | 启用；历史只有入口页面证据 |
| HLS | 同上 | 默认禁用；历史疑似入口异常 |
| Player-JY | 同上 | 启用，但未实播验证 |

原地址保留在 `current_source/sources.json`，出处也在其中。不以一次失败永久判死，不以域名可达标记全线路可播。本次没有重新发起第三方实播测试。

## 测试分层

### 本次交接重新执行

`handoff_verification/go_test_handoff.txt`：Go race 回归产生 119 条 PASS（9 个顶层 + 110 个子用例）；1 个手工浏览器测试入口按设计跳过。Linux 和 Windows 目标 `go vet` 均成功。由当前源码交叉重建的 EXE 与原交付 EXE 字节相同。

这些检查覆盖离线/合成数据与本机接口，不是 Windows 执行或外部影视实播。空的 vet/build 日志表示对应命令没有输出，退出结果由 `handoff_checks.json` 记录。

### 原交付中的历史证据

`current_source/docs/native_tests.txt`、`docs/production_cli_test.json`、`docs/browser/browser_tests.json`：分别记录 Go 回归、9 项 Linux 生产进程检查、17 项前端检查。浏览器通过内存装载界面 + 本机 API 桥接，在合成目录和 WebM 上检查。

旧 Python 76/17 测试记录位于 `legacy_python/evidence/`；不可把旧结果当成所有当前代码都重复验收。

### 仍没有证据

Windows 真机启动、实际浏览器窗口/退出体验、系统代理分支、PAC/bypass 等复杂代理、SmartScreen/Defender 兼容；真实在线搜索到正确作品并完成媒体解码、时间推进与换集；所有线路或所有影片可用性。

## 需要保留的诚实状态

`resolve` 返回的 `playback_verified` 默认为 false。
`iframe.onload` 仅表示页面事件。
`browser_time_advanced` 是本次浏览器媒体观测。
`user_confirmed` 是用户手工报告。
任何一项都不能自动升级成“整个来源已验证可用”。

## 安全与维护边界

现有实现含本机回环监听、会话 Cookie、CSRF、Host/Origin 校验、TLS 校验、URL/DNS 检查、iframe 限制、独立浏览器配置与首次外联确认。它们是风险降低措施，不是对第三方网页完整安全性的认证。

现有 Windows 代理代码读取环境代理和显式系统代理文本；对自动代理/PAC、绕过列表不能当作已经支持并验收。外部 iframe 不由 Go 后台完整控制，浏览器兼容和隐私风险仍需分层说明。

`current_source/THIRD_PARTY_NOTICES.md` 的前半部分沿用了旧 Python 版描述，后半已有 Go 独立版补充。后续可整理文案一致性，但本次保留原件，不据旧段落误判运行依赖。
