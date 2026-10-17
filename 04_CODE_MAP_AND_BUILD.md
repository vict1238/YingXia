> 2026-10-16 最新交付：Windows 原生版与 Android 均为 **2.3.0**，软件、源码、许可与实际验证见 `releases/YingXia_v2.3.0_*`。本轮加入标记广告跳过、有限提前缓存、卡顿换源提示、收藏／历史断点续播、播放区控件和拼音／首字母／模糊搜索；Android 增加长按临时 2 倍速与电视大屏适配。Windows 56 项、Android 75 项 Flutter 测试通过，后台最新源码在隔离模拟器通过 37 项引擎测试、15 子例与 1 项生命周期测试。实际运行范围与失败尝试见发布验证记录，MatePad Pro Android 16 仍需真机复测。此前发布文件保持原字节。下文为历史版本说明。

# Android 2.2 当前代码与构建

Android客户端：`native_android_client/`；独立Go/JNI后台：`native_android_core/`。先阅读客户端、后台 README，以及 `releases/YingXia_v2.2.0_Android_VALIDATION.json`。外置工具构建后台 AAR，客户端 `tools/build_android.ps1` 生成已签名候选 APK，`tools/verify_android_apk.py` 检查三 ABI 与签名、NDK strip 后 JNI 对应。`tools/package_android_release.py` 只打包已验证的正式字节和对应源码，排除工具链、签名私钥、缓存及用户数据。Windows 当前代码仍为 `native_windows_client/` + `native_windows_core/`，未在本轮修改。下文为历史地图。

# 主线代码地图与构建

以下路径以交接包根目录为基准；开发工作目录为 `current_source/`。不要在 `legacy_python/` 打补丁后误以为修改了当前 EXE。

## 实际架构

```text
Windows x64 EXE（Go）
  ├─ 编入 HTML / CSS / JavaScript / sources.json / 演示 WebM
  ├─ 127.0.0.1 随机端口本机 HTTP 服务
  ├─ 会话、权限、目录适配、来源选择、JSON 存储
  └─ 启动已安装 Edge/Chrome 的独立配置应用窗口
        ├─ 查询本机 API
        ├─ HTML5 video：直连或本机演示媒体
        ├─ sandbox iframe：第三方网页播放器
        └─ 明确链接：用户打开官方播放页
```

这是浏览器应用窗口，不是 Qt/Tauri/Electron，也没有捆绑完整 Chromium。来源媒体通常由浏览器加载，Go 后台不是通用媒体下载/代理服务。

## 文件职责

| 文件 | 职责 |
|---|---|
| `main.go` | 数据目录、命令行、单实例、随机端口服务、浏览器启动、心跳与退出 |
| `platform_windows.go` | Windows 对话框、文件替换、互斥锁、默认浏览器和显式代理 |
| `platform_linux.go` | Linux 开发/测试对应实现 |
| `service.go` | 应用版本、资产 embed、配置、离线演示、搜索/导入/绑定/resolve/诊断 |
| `catalog.go` | suggest/详情接口、缓存、错误和分页范围说明、平台静态入口检索 |
| `models.go` | Film/Group/Episode、完整片名规范化、中文数字、季数、排序、多组和选集 |
| `network.go` | Fetcher 接口、超时、TLS、响应限额、重定向、DNS检查、探测 |
| `security.go` | 五个平台域名与页面白名单、公网 URL/地址判断、敏感参数清理 |
| `storage.go` | JSON 收藏/历史/设置/绑定、并发保护、原子替换 |
| `server.go` | 本机 HTTP API、Cookie/CSRF/Host/Origin、静态文件和媒体 Range |
| `sources.json` | 一条目录与七条解析服务配置，历史状态标签 |
| `web/index.html` | 页面结构；引入本机 bootstrap 和 app 资源 |
| `web/app.js` | 搜索/选集/播放/切源/收藏/诊断/停止竞态与状态 |
| `web/app.css` | 现有中文界面样式 |
| `web/demo/*.webm` | 自生成测试画面及合成音，不是影视素材 |
| `core_test.go`、`http_test.go` | 元数据、来源、接口、安全、存储与本机服务回归 |
| `ui_harness_test.go` | 显式启用的手工浏览器测试入口，正常测试会跳过 |
| `docs/browser_smoke.py` | 开发用浏览器回归脚本，非用户运行依赖 |

## 数据模型与播放返回

Film 区分作品 ID、title/year/kind/area、说明、来源、groups 和 demo。Group 有稳定 key、name、episodes；Episode 的 key/name/ordinal 与后台保留的 URL 分开。平台内部 ep ID 不能直接当自然集数。

`Service.Resolve` 返回 `kind = media | page | external`，以及 URL、source_label、title、episode、history_id、film/group/episode 标识。返回入口时不声称已实播。

平台手工导入只生成一个“当前播放页（集数未核验）”条目；逐集绑定是用户提供的对应关系，不自动断言作品匹配。

## API 速查

GET：`/api/state`、`/api/library`、`/api/detail?id=...`、`/api/bindings?id=...&episode=...`、`/api/report`。

POST：`/api/search`、`/api/settings`、`/api/import-platform`、`/api/bind`、`/api/match`、`/api/resolve`、`/api/favorite`、`/api/progress`、`/api/clear-history`、`/api/diagnose`、`/api/ping`、`/api/quit`。

根页面设置会话 Cookie，`/bootstrap.js` 提供前端 CSRF 配置。API 需会话；POST 需 `X-YingXia-CSRF` 和 JSON。返回 `{ok:true,data:...}` 或 `{ok:false,error:...,message:...}`。接口不应暴露到 0.0.0.0 或公网。

## 数据位置

Windows：`%LOCALAPPDATA%\YingXia\standalone`。

- `library.json`：收藏、历史、设置、平台绑定。
- `app.log`：运行日志；分享前检查私人数据。
- `session.json`：当次本机服务信息，正常退出后清理。
- `browser-profile/`：本程序浏览器配置，不能导入用户日常账号。
- 可选 `sources.json`：本地覆盖来源配置，重启后读取。

开发调试可用 `--data-dir` 或 `YINGXIA_DATA_DIR` 指向临时目录，避免碰真实用户数据。旧 Python SQLite 不自动迁移，文件不被覆盖。

## 可复现构建

历史与本次一致性复建使用 Go 1.23.2（Linux/amd64），`go.mod` 写 go 1.23；无外部 Go 模块。下面是开发环境命令，不要求最终用户执行。

Linux shell，在 current_source 内：

```sh
go test -race -count=1 -v ./...
go vet ./...
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go vet ./...
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath \
  -ldflags='-H windowsgui -s -w' \
  -o YingXia_v1.0_Windows_x64.exe .
```

包中已有 `BUILD_WINDOWS.bat` 与 `build_windows.sh`。不要将 `legacy_python/BUILD_EXE.bat` 当作主线构建命令。

本次离线核验设置了 `GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off`，使用已有工具链，没有下载依赖。Go 测试中的假网络和合成在线目录位于 `_test.go`，不进入普通生产构建。

## 可选开发检查

```sh
# 只起本机后台，不自动启动浏览器；使用新目录
go run . --no-browser --data-dir ./tmp-dev-data

# Go 测试二进制 + Python/Playwright 浏览器测试（仅开发者）
mkdir -p build
go test -c -o build/yingxia_testserver .
CHROMIUM_PATH=/usr/bin/chromium python docs/browser_smoke.py
```

先阅读脚本、确认当前环境有相应工具。历史浏览器受导航策略限制而使用内存装载/API桥；不修改管理策略，不把这一方式当作真实 Windows 浏览器启动验收。

## 可复用与许可

当前及早期应用源码以包内 GPL 文件为准，保留来源说明；Go runtime/标准库相关许可与专利说明也已保留。原配置参考 88lin 快照，不代表与第三方服务合作。不要删除许可，不随包分发字体、账号、Cookie、第三方电影或密钥。
