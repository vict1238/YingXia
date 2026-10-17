# 交付验证记录 — 2026-10-10

## 实际完成

- Python 核心 / 服务 / 本机 HTTP / CLI 启动关闭测试：**76 项通过**。
- 前端流程检查：**17 项通过**。
- Python 版本：3.13.5（Linux）；浏览器：Chromium 144.0.7559.96。
- 真实合成 WebM 解码：观测时间 1.220 秒，
  已解码帧数 34，分辨率 640 × 360。
- JavaScript 语法检查、Python compileall 通过；前端检查无未捕获 JavaScript 异常。

## 测试方法与限制（重要）

本环境系统 Chromium 的管理员策略禁止 URL 导航，包括本机地址。
**没有修改或关闭这些策略。**

因此前端检查在 Chromium 中内存装载本项目真实 HTML/CSS/JavaScript，
使用仅测试代码中的本机 HTTP 桥访问实际 Python 服务，测试视频以内存 Blob 装载。
接口请求经过真实服务的会话/CSRF/Origin/Host 检查；原生浏览器从地址栏打开本机服务的集成链路未在这里验证。
本机 HTTP 与 CLI 流程另外由真实 HTTP 客户端实测。

在线目录返回值采用明确标注的合成 fixtures，网页加载检查采用合成 iframe load 事件。
这些用于验证代码分支，不验证第三方目录、播放器和版权授权。
截图是此方式下的实际前端渲染，不是 Windows 真机截图。

## 没有完成 / 不作承诺

- 未完成任何真实第三方来源的搜索 → 匹配 → 选集 → 外部视频解码链路验收。
- 未运行 Windows 10/11 真机启动、Edge/Chrome 专用窗口和平台网页实播验收。
- 没有生成已签名或实测的 Windows EXE；BUILD_EXE.bat 只提供目标平台构建路径。
- 未做完整安全审计、恶意站点审计或浏览器/操作系统兼容矩阵。
- 不保证平台入口静态检索、原生 HLS、4K/HDR、DRM、完整弹幕、全剧集覆盖或长期线路稳定。

## 前端检查明细

- 通过：Home loaded with offline-by-default configuration
- 通过：Offline search returns explicitly labelled local demo only
- 通过：Local synthetic video decodes frames and time advances
- 通过：Next-episode control selects and plays the second local clip
- 通过：Switch to source missing episode 2 does not silently play episode 1
- 通过：Favorites and observed playback evidence persist in local SQLite
- 通过：Saved favorites survive UI reinitialization; browser URL navigation not tested
- 通过：Online query blocked until consent
- 通过：Synthetic online catalogue search sorts full title and season
- 通过：All synthetic VOD source groups remain selectable
- 通过：A synthetic iframe load event does not mark media playback successful; remote embedding not tested
- 通过：Manual confirmation remains labelled as user-reported evidence
- 通过：A delayed playback response cannot restart the player after Stop
- 通过：Manual official-page import strips private query parameters and does not invent episode list
- 通过：Diagnostic UI displays HTTP-only results without claiming real playback
- 通过：Search UI has no horizontal page overflow at 1340, 1024, 720 and 390 px
- 通过：No uncaught browser JavaScript errors

## 复现证据

- `evidence/unit_tests.txt`：离线与本机接口测试原始输出。
- `evidence/browser_tests.json`：前端检查、浏览器环境、合成视频帧数与时间。
- `evidence/browser_tests.txt`：前端检查原始输出。
- `evidence/ui_home.png` / `ui_demo_results.png` / `ui_player.png` / `ui_settings.png`：实际前端渲染。
- `tests/`：测试源码与合成响应。生产服务不导入 tests，不会在在线搜索失败时使用演示数据兜底。
