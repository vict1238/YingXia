# 映匣 v1.3.1

双击 `YingXia_v1.3.1_Windows_x64.exe` 使用。Windows 10/11 x64，使用映匣自己的窗口和内嵌 WebView2，不再打开 Chrome/Edge 应用模式窗口。需要 Microsoft WebView2 Runtime，不需要安装 Python、Go 或 Node。沿用极简界面和原数据目录。

## 本版功能

- 修复“清空记录”按钮。点击后在软件内确认，清空后立即显示“暂无记录”，重新打开软件仍为空；收藏、绑定和设置保留。请求失败时保留记录并显示错误，可重试。
- 主界面由映匣 EXE 创建和管理 Windows 窗口，WebView2 在窗口内显示现有页面，保留搜索、播放工作台、收藏与记录、来源与诊断。
- 关闭主窗口同时结束本机后台。重复启动聚焦已有窗口；最小化时暂停播放，恢复后点击“播放所选集”继续。
- 离开“播放工作台”自动暂停内置视频，保留当前位置；返回后点击“播放所选集”继续。网页播放因跨站控制限制，在离开时关闭播放页，返回后手动重新打开。
- 播放区空状态改为“选集之后请点击下方播放所选集”。
- 移除右上角“粘贴播放页”入口和弹窗，原有收藏、记录与逐集绑定继续可用。
- 17 个内置搜索来源：无损云，加 KazumiRules 官方索引中的 16 个动漫来源。原来的 7 条平台解析线路保留。
- 全部来源并行搜索，也可只搜动漫或指定来源。搜索后自动检查候选的详情、播放页或媒体样本，让可访问的结果排在前面，失败结果保留在后面。
- 搜索进度显示已完成的来源和候选检查数量，支持取消。结果保留来源和作品版本，单个来源失败不会丢掉其他结果。
- 点开结果优先选择本轮检测成功的播放分组；原来的逐集匹配和缺集保护保留。
- 可添加苹果 CMS 兼容接口，支持 `[{"name":"名称","url":"https://…"}]` 和 LibreTV 点播来源 `{"sources":[…]}` 批量导入。
- 本地打包 hls.js；内置 HLS 播放、倍速、可用时的画质选择、自动下一集。动漫播放页先尝试静态提取，再使用临时独立浏览器取流，未取得媒体地址时回退网页播放。
- 来源设置可开关各来源、移除自定义来源、手动检查更新、关闭自动更新。
- 本版不加入超分。

## 自动更新

允许联网且开启“自动更新来源”后，启动时检查是否距上次检查超过 24 小时；程序运行期间每小时检查一次到期状态。关闭联网后不再发起新的来源更新请求。也可随时点“来源与诊断 → 检查更新”。

跟踪三个官方仓库的提交：

| 上游 | 本程序更新的内容 |
|---|---|
| [88lin/video_vip](https://github.com/88lin/video_vip) | 目录地址和解析线路配置；只读取字段，不执行用户脚本 |
| [Predidit/KazumiRules](https://github.com/Predidit/KazumiRules) | 官方索引中的来源与规则，按同一提交下载并整体验证 |
| [Predidit/Kazumi](https://github.com/Predidit/Kazumi) | 程序版本、提交和规则 API 等级；接口不兼容时显示“需要升级程序” |

接口兼容时自动更新来源数据；程序本身需要新适配时明确提示，EXE 仍由新版本交付。不会把所有 Kazumi 程序提交都当成映匣必须升级。

更新失败继续用原配置；自定义 CMS、来源开关、收藏和历史保留。配置原子写入并保留上一份有效备份。

## 来源与验证

内置动漫来源：7sefun、aafun、AGE、akianime、baimao、dalvdm、DM84、ezdmw、giriGiriLove、mgnacg、moonci、mutefun、MXdm、sorani、xfdmneo、xfdmnext。

搜索结果的检测状态分为“媒体可访问”“页面可访问”及失败或未完成。媒体检查读取有上限的媒体样本；HLS 还检查清单与样本分片。页面检查确认相应播放页响应，两种成功状态分别显示。它们反映检查时的访问情况，实际解码和完整播放仍以播放器为准。

检查最多并发 6 个任务，整个搜索任务最多 150 秒。成功结果缓存 3 分钟，失败结果缓存 45 秒，避免每次搜索重复等待。取消搜索后停止余下任务，可以立即重新搜索。

本轮验证记录见 `docs/validation_v1.3.1/`，汇总见 `docs/VALIDATION.md`。最终发布 EXE 的 8 项清空记录专项检查通过，实际在 WebView2 窗口中验证取消、失败重试、重复点击、延迟响应、磁盘保存和软件重启。v1.3.0 的独立窗口、浏览器界面、搜索筛选和暂停回归保留在 `docs/validation_v1.3.0/`，是该历史版本的证据。此前 v1.2.0 的搜索访问实测保留在 `docs/history_v1.2.0/VALIDATION.md` 和 `docs/validation_v1.2.0/`，不作为当前来源健康状态。

## 窗口结构

Windows EXE 创建 Win32 窗口并内嵌 WebView2，Go 后台继续在 `127.0.0.1` 随机端口提供界面和 API。HTML/CSS/JavaScript 在这个窗口内运行，主界面不再调用 Edge/Chrome 的 `--app` 模式。WebView2 仍是网页渲染内核，任务管理器出现 `msedgewebview2.exe` 子进程属于正常现象。

本版使用 `github.com/wailsapp/go-webview2 v1.0.23` 的纯 Go 加载路径；不是 Wails 框架项目。Microsoft WebView2 Runtime 由电脑提供，EXE 未打包完整浏览器运行时。Windows 通常已有运行时；若启动提示缺少，可安装微软的 [WebView2 Evergreen Runtime](https://developer.microsoft.com/microsoft-edge/webview2/)。

界面与现有日常浏览器资料独立。部分动漫来源的播放页在静态解析失败后，仍调用已安装 Edge/Chrome 的临时无头实例取流；这与主界面窗口分开，没有桌面浏览器时该步骤不可用，仍会尝试网页播放回退。

## 数据与升级

直接运行新版 EXE 即可，默认使用 `%LOCALAPPDATA%\YingXia\standalone`。

- `library.json`：收藏、记录、绑定、开关。
- `source_registry.json`：同步后的来源、规则和上游状态。
- `source_registry.json.previous`：上一份有效来源配置。
- `webview-profile`：WebView2 的应用独立配置。
- `browser-profile`：旧版界面配置保留；新版主窗口不再使用它。
- 取流使用临时 `resolver-profile-*`，任务结束后清理。

旧版 EXE 保留。升级前关闭旧版窗口及后台，再运行新版，避免连接到旧进程。旧 Python SQLite 不自动迁移。新版正常关闭主窗口时结束后台；`--no-browser` 仍供开发和自动回归使用。

## 源码与构建

Go 后台 + Win32/WebView2 窗口 + HTML/CSS/JavaScript。依赖和许可随 `vendor/`、`web/vendor/`、`licenses/` 提供，离线构建无需另行下载模块。本次编译器为 Go 1.27.2 windows/amd64；模块最低声明为 Go 1.26。构建无需 CGo 或 C/C++ 编译器；运行仍需 WebView2 Runtime。

```powershell
$env:GOTOOLCHAIN='local'
$env:GOPROXY='off'
$env:GOSUMDB='off'
$env:CGO_ENABLED='0'
$env:GOOS='windows'
$env:GOARCH='amd64'
go test -mod=vendor -count=1 -v ./...
go vet -mod=vendor ./...
go build -mod=vendor -trimpath '-ldflags=-H windowsgui -s -w' -o YingXia_v1.3.1_Windows_x64.exe .
```

也可运行 `BUILD_WINDOWS.bat` / `build_windows.sh`。精确构建信息见 `docs/BUILD_INFO.json`。

Windows 图标和 DPI/权限清单已编入随源码提供的 `resource_windows_amd64.syso`，普通离线构建不需要安装资源工具。仅在修改图标或 `assets/windows/yingxia.manifest` 后重新生成：

```powershell
# 本机 PowerShell 生成图标；使用已有 Microsoft YaHei UI 字体，不分发字体文件
powershell -NoProfile -File docs/create_windows_icon.ps1
# 开发者首次使用需联网取得 rsrc v0.10.2；随后重新执行上述 go build
$env:GOPROXY='https://proxy.golang.org,direct'
$env:GOSUMDB='sum.golang.org'
go run -mod=mod github.com/akavel/rsrc@v0.10.2 -arch amd64 -ico assets/windows/yingxia.ico -manifest assets/windows/yingxia.manifest -o resource_windows_amd64.syso
```

可选开发回归需要本机 Node 和 Playwright，均不是软件运行依赖：

```powershell
go test -mod=vendor -c -o build/yingxia_testserver.exe .
$env:PLAYWRIGHT_MODULE='本机 Playwright 模块目录'
$env:CHROMIUM_PATH='本机 Chrome 或 Edge 路径'
node docs/browser_smoke.cjs
# 切页暂停、手动继续、迟到响应与空状态回归
node docs/playback_pause_smoke.cjs
# 搜索进度、访问检查与排序回归
node docs/search_filter_smoke.cjs
$env:YINGXIA_EXE='发布 EXE 绝对路径'
node docs/release_smoke.cjs
# Windows 发布 EXE 的自有窗口、实际最小化、全屏和关闭回归
node docs/native_window_smoke.cjs
# 本版 Windows 发布 EXE 的清空记录、磁盘保存及重启回归
node docs/clear_history_smoke.cjs
# 可选真实外联检查
node docs/live_search_filter.cjs
# 可选独立浏览器动态取流回归（合成页面）
$env:YINGXIA_BROWSER_TEST='1'
go test -mod=vendor -run '^TestBrowserMediaSniff$' -v .
```

`native_window_smoke.cjs` 为隔离测试临时设置 `YINGXIA_WEBVIEW_DEBUG_PORT`，连接回环地址上的 WebView2 调试端口；普通启动默认没有这个端口。手动调试时可将该变量设为一个可用端口，结束后删除变量。测试通过 DevTools 提供合成目录和视频，不访问第三方影片。

产品能力借鉴 LibreTV 的多来源聚合和播放方式，具体适配在本工程独立实现。本次清空记录修复见 `docs/ITERATION_v1.3.1.md`，窗口迁移见 `docs/ITERATION_v1.3.0.md`，切页暂停与界面精简见 `docs/ITERATION_v1.2.1.md`，搜索检查说明见 `docs/ITERATION_v1.2.0.md`，来源与更新机制见 `docs/ITERATION_v1.1.0.md`，许可见 `THIRD_PARTY_NOTICES.md`。本包为 WebView2 Windows 版；不含 Android APK 或无网页内核 Windows 版。
