# 来源与许可

应用源码及维护采用 GPL-3.0-or-later，完整许可见 `LICENSE.txt`。页面精简不影响包内开源许可的保留。

Go runtime / 标准库使用 BSD-3-Clause，见 `LICENSE_GO.txt` 和 `GO_PATENTS.txt`。程序没有 Python DLL，没有捆绑浏览器或影视素材。

## 88lin

目录及解析配置参考 [88lin/video_vip](https://github.com/88lin/video_vip/blob/main/video_vip.user.js) 3.2.9，原快照 blob `30260d8fb3db47b4f14fcc1a3faf8efadf99a43a`。2026-10-13 上游提交 `cc3df4f0f7ba6c8e6188ea39f04c8d8a8da0409c`。原项目 GPL-3.0，作者标注“茉灵智库”。本程序不打包、运行该用户脚本；更新器只解析目录和线路字段。

## Kazumi 规则

`providers.json` 中 16 个动漫规则来自 [Predidit/KazumiRules](https://github.com/Predidit/KazumiRules)，固定快照 `6b5ead789dcdbed2c67b76ca254beb5aa7ec9e95`，按 `index.json` 收录。规则使用 MIT，Copyright (c) 2024 Predidit；完整许可见 `licenses/KazumiRules_LICENSE.txt`。

[Predidit/Kazumi](https://github.com/Predidit/Kazumi) 只用于了解规则协议和监测兼容性。本工程独立实现 Go 规则适配，没有嵌入其 Flutter 程序、图标、播放器或超分代码。

## LibreTV

[LibreSpark/LibreTV](https://github.com/LibreSpark/LibreTV) 使用 AGPL-3.0-or-later。本版借鉴多来源 CMS 搜索、来源管理与 HLS 播放的功能组织，未复制、嵌入其 Next.js / TypeScript 程序。CMS 请求、来源命名空间、导入和界面均在本工程实现。其可配置的点播来源列表使用 name / url 字段，本版支持这些字段；不把 LibreTV 当成自带媒体服务。

## 播放库与 Go 模块

- hls.js 1.7.3：Apache-2.0，`web/vendor/hls.min.js`、`web/vendor/HLS_LICENSE.txt`、`licenses/Apache-2.0.txt`。哈希见 `docs/HLS_BUILD.json`。随包本地加载。
- github.com/antchfx/htmlquery v1.3.7：MIT。
- github.com/antchfx/xpath v1.3.9：MIT。
- github.com/ohler55/ojg v1.28.7：MIT。
- github.com/chromedp/chromedp v0.21.0：MIT。
- github.com/chromedp/cdproto v0.157.8：MIT。
- github.com/go-json-experiment/json：BSD-3-Clause。
- github.com/golang/groupcache：Apache-2.0。
- golang.org/x/net v0.61.0、golang.org/x/text v0.43.0：BSD-3-Clause。

各模块的完整许可、作者说明和版本保留在 `vendor/`，依赖锁定于 `go.mod`、`go.sum` 和 `vendor/modules.txt`。不使用账号 Cookie 作为规则配置。

## 测试素材

`testdata/demo` 的 WebM 和 `testdata/hls` 的 H.264/AAC 分片为 FFmpeg 生成的图案及合成音。只编入开发测试程序，不在生产 EXE 中。FFmpeg、Node、Playwright、浏览器测试程序和开发 Go 工具链不随运行 EXE 分发。

`docs/history_v1.0.1`、`docs/history_v1.0.2`、`docs/history_v1.1.0` 及对应的 `validation_v*` 目录保留历史证据。本轮 v1.2.0 记录位于 `docs/validation_v1.2.0`。搜索访问检查在本工程实现，本轮未新增第三方依赖。
