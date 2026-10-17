# 映匣 Windows 原生版 2.2 后台

此目录是本轮 Windows 专用源码分支，版本为 `2.2.0-native-windows`。原共享后台 `../native_core`、Android 源码与旧版交付文件保持原样。当前 Windows Flutter 客户端为 `../native_windows_client`。

来源清单包含既有无损云、官方固定提交的 16 个 Kazumi 规则和表格筛选后的 30 个新 CMS，总计 47 个。CMS 清单为 `internal/engine/reviewed_sources.json`，筛选证据为 `../native_windows_client/evidence/v2_2/source_screening`。Kazumi 规则提交固定在 `2fa4d28659f94e1eb52e7a38d79b7b9a5dcd605b`；API 9 支持 `sourceTemplate` 将搜索 ID 映射为固定公网详情地址，替换值编码后不能改变主机和协议。

首次升级会将新 CMS 合并进已有 `source_registry.json`，只提升已知 Kazumi 规则的较旧版本，不回退用户已经下载的更高版本，保留自定义源、各源开关与测速记录。88lin、KazumiRules 和 Kazumi 的既有自动检查和手动更新继续工作。表格筛选结果随本次应用版本交付；未把任意订阅脚本当成可执行更新入口。

测速最多并行 6 个来源，单源最多 8 秒。CMS 测试真实 `ac=detail&pg=1` 目录并解析影片编号与名称；Kazumi 测试现有静态/API 搜索规则。HTTP 200 的验证页、空目录和错误结构均不会被算作可用。结果保存响应时间、检测时间、目录条数和状态，表示目录读取能力，`playback_verified` 固定为 false；逐影片媒体检测仍由原有搜索流程完成。健康且快的来源优先，24 小时前的结果显示待测。尚未测速时，通用搜索优先调度 CMS/目录，再调度动漫规则；单独动漫搜索照常使用动漫源。调度在主队列获取名额，避免线程启动顺序打乱测速优先级。

私有 API 继续验证认证 Cookie、Origin、Host 和写请求 CSRF，仅监听随机的 `127.0.0.1` 端口。原 URL、DNS、重定向、TLS 证书与标准端口边界保持生效。

- `GET /api/sources`：统一清单、上游版本和各源 `health`，以及 `editable`、`custom`、`endpoint`。
- `POST /api/sources/add`：`{name,url}`，或原有批量 `items`。
- `POST /api/sources/edit`：`{id,name,url}`，仅自定义来源；URL 留空保留当前地址。
- `POST /api/sources/remove`：`{id}`，仅自定义来源。
- `POST /api/settings`：`{enabled_providers:{id:boolean}}`，按 ID 原子合并，不重置其他开关。
- `POST /api/sources/update`：88lin / Kazumi 上游更新。
- `POST /api/sources/speed/start`：`{ids:[id,...]}`；省略 `ids` 测全部，含禁用来源。
- `GET /api/sources/speed/status?id=...`：`state`、`completed`、`total`、0–100 的 `progress`/`percent`、`results` 与最新 `sources`。
- `POST /api/sources/speed/cancel`：`{id}`；临时结果留在任务中供查看，本轮结果不保存，消息明确为“已取消，本轮结果未保存”。完整完成时持久化。

`cmd/native_backend` 经标准输出返回一行连接信息，宿主关闭管道或请求 `/api/quit` 后退出。Windows 默认数据目录仍为 `%LOCALAPPDATA%\YingXia\standalone`，与既有版本共用数据锁。测试只使用隔离临时目录。

从本目录构建，使用项目 Go 1.27.2 及随源码保存的 vendor：

```powershell
New-Item -ItemType Directory -Force build | Out-Null
& '../.tools/next/go/bin/go.exe' build -mod=vendor -trimpath '-ldflags=-s -w -H windowsgui' -o build/yingxia_backend.exe ./cmd/native_backend
```

本轮 Go 验证为 `evidence/engine_tests.log` 中 14 个顶层测试及 5 个 JSON 结构子例、`evidence/core_tests.log` 中 1 个生命周期检查、`evidence/vet.log` 中 Go vet PASS。覆盖认证 HTTP 接口、异步进度、开关与自定义源管理、取消、重启持久化、内置迁移、API 9 模板与真正的规则替换更新、私网请求拦截；网络内容为本地控制的合成响应。最初 `go test ./...` 生成的临时目录程序被 Application Control 阻止，随后使用 `go test -c -o evidence/...test.exe` 编译后直接执行项目内测试文件，未修改系统策略。

正式构建不使用 `native_fixture`；该测试标签和 `fixturedata` 只用于合成媒体回归。测试可执行文件与缓存不放入交付软件。真实外网目录测速以客户端 `evidence/v2_2` 的实际传输验收记录为准，不把合成响应测试当作外网可播放验证。