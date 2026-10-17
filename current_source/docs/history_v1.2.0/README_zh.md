# 映匣 v1.2.0

双击 `YingXia_v1.2.0_Windows_x64.exe` 使用。Windows 10/11 x64，电脑已有 Edge 或 Chrome 即可，不需要安装 Python、Go 或 Node。沿用极简界面和原数据目录。

## 本版功能

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

2026-10-14 真实查询“葬送的芙莉莲”：27 条候选全部检查，1 条媒体样本可访问、21 条播放页可访问、5 条访问失败；失败项自动后置，全流程约 38 秒。本轮 191 条 Go PASS 记录、27 项浏览器回归及 8 项实际 EXE 检查通过。原始记录见 `docs/validation_v1.2.0/`，汇总见 `docs/VALIDATION.md`。历史 v1.1.0 的联网结果保留在旧记录中，不作为当前来源健康状态。

## 数据与升级

直接运行新版 EXE 即可，默认使用 `%LOCALAPPDATA%\YingXia\standalone`。

- `library.json`：收藏、记录、绑定、开关。
- `source_registry.json`：同步后的来源、规则和上游状态。
- `source_registry.json.previous`：上一份有效来源配置。
- `browser-profile`：应用界面独立配置；取流使用临时 `resolver-profile-*`，任务结束后清理。

旧版 EXE 保留。旧 Python SQLite 不自动迁移。关闭窗口后后台沿用空闲退出机制。

## 源码与构建

Go 后台 + HTML/CSS/JavaScript。依赖和许可随 `vendor/`、`web/vendor/`、`licenses/` 提供，离线构建无需另行下载模块。本次编译器为 Go 1.27.2 windows/amd64；模块最低声明为 Go 1.26。

```powershell
$env:GOTOOLCHAIN='local'
$env:GOPROXY='off'
$env:GOSUMDB='off'
$env:CGO_ENABLED='0'
$env:GOOS='windows'
$env:GOARCH='amd64'
go test -mod=vendor -count=1 -v ./...
go vet -mod=vendor ./...
go build -mod=vendor -trimpath '-ldflags=-H windowsgui -s -w' -o YingXia_v1.2.0_Windows_x64.exe .
```

也可运行 `BUILD_WINDOWS.bat` / `build_windows.sh`。精确构建信息见 `docs/BUILD_INFO.json`。

可选开发回归需要本机 Node 和 Playwright，均不是软件运行依赖：

```powershell
go test -mod=vendor -c -o build/yingxia_testserver.exe .
$env:PLAYWRIGHT_MODULE='本机 Playwright 模块目录'
$env:CHROMIUM_PATH='本机 Chrome 或 Edge 路径'
node docs/browser_smoke.cjs
# 搜索进度、访问检查与排序回归
node docs/search_filter_smoke.cjs
$env:YINGXIA_EXE='发布 EXE 绝对路径'
node docs/release_smoke.cjs
# 可选真实外联检查
node docs/live_search_filter.cjs
# 可选独立浏览器动态取流回归（合成页面）
$env:YINGXIA_BROWSER_TEST='1'
go test -mod=vendor -run '^TestBrowserMediaSniff$' -v .
```

产品能力借鉴 LibreTV 的多来源聚合和播放方式，具体适配在本工程独立实现。搜索检查说明见 `docs/ITERATION_v1.2.0.md`，来源与更新机制见 `docs/ITERATION_v1.1.0.md`，许可见 `THIRD_PARTY_NOTICES.md`。
