# 映匣 Android 原生版 2.2 后台

此目录是 Android 2.2 独立源码分支，版本为 `2.2.0-native-android`，客户端为 `../native_android_client`。业务、配置和 vendor 从 Windows 2.2 后台冻结复制；247 个文件的 SHA-256 对照见 `evidence/windows_source_correspondence.json`，唯一业务源码变化是 `service.go` 的版本字符串。

来源共 47 个：既有无损云 1 个、固定提交的 Kazumi 规则 16 个、表格筛选的 CMS 30 个。Kazumi 快照提交为 `2fa4d28659f94e1eb52e7a38d79b7b9a5dcd605b`，支持 API9 `sourceTemplate`；编码后的搜索 ID 不能改变详情地址的协议和主机。CMS 种子为 `internal/engine/reviewed_sources.json`，其筛选与真实元数据验证见 `../native_android_client/evidence/windows_2_2/source_screening`。

首次升级合并内置来源并提升已知较旧 Kazumi 规则，保留自定义来源、开关、更高版本规则和测速记录。来源管理支持增删改、启停、目录测速、异步进度和取消。测速并行上限为 6、单源上限为 8 秒；取消时本轮结果不持久化，完整完成时保存。测速验证目录读取能力，逐片媒体可用性由原搜索流程检查。88lin / KazumiRules / Kazumi 的手动与自动更新、可用性优先搜索、媒体接力、收藏和历史清空逻辑均沿用。

根包仅导出 gomobile 兼容的 `Start(dataDir)`、`Stop()`、`Running()`、`AwaitStopped()`；Java 接口为 `core.Core.start/stop/running/awaitStopped`。Android 宿主通过 `yingxia/core` 通道启动后台，数据位于应用私有目录。HTTP 监听随机的 `127.0.0.1` 端口，所有接口要求会话，写请求要求 CSRF；保留 Host/Origin、URL、DNS、重定向、TLS 和标准端口检查。凭据仅由连接描述返回宿主，不写入构建日志。

构建使用 Go 1.27.2、JDK17、NDK `28.2.13676358`，gomobile/gobind 固定为 `v0.0.0-20261011034528-197770444867`。仅设置当前进程环境：

```powershell
$env:GOTOOLCHAIN = 'local'
$env:GOFLAGS = '-mod=mod'
# 将 Go、JDK bin、gomobile/gobind bin 加入当前进程 PATH。
# 设置 JAVA_HOME、ANDROID_HOME、ANDROID_SDK_ROOT、ANDROID_NDK_HOME。
go install golang.org/x/mobile/cmd/gomobile@v0.0.0-20261011034528-197770444867
go install golang.org/x/mobile/cmd/gobind@v0.0.0-20261011034528-197770444867
New-Item -ItemType Directory -Force (Join-Path (go env GOPATH) 'pkg/gomobile') | Out-Null
gomobile bind '-target=android/arm64,android/amd64,android/arm' -androidapi=26 -trimpath '-ldflags=-s -w' -o build/yingxia_core.aar .
```

项目工具布局下可运行 `./build_android_aar.ps1 -ToolsRoot <工具目录>`；在独立解压目录中传入外置 `.tools` 的绝对路径即可，输出仍位于当前源码目录。添加 `-Fixture` 只生成测试 AAR。该脚本从 `go.mod` 指定的版本编译 gomobile 与 gobind，创建所需缓存目录；此固定上游版本的 `gomobile init` 会安装 `gobind@latest`，因此脚本使用已固定的 gobind。网络首次下载需要遵守 `go.sum`，SDK、NDK、JDK 与 Go 需由构建者预先准备。

正式 AAR 为 `build/yingxia_core.aar`，最低 API26，包含 `arm64-v8a`、`armeabi-v7a`、`x86_64`。其哈希、大小、manifest 和 JNI 构建信息见 `evidence/android_core_build.json`、`evidence/java_api.txt` 与 `evidence/go_build_info_*.txt`。客户端正式构建仅使用这一 AAR。

合成媒体回归使用 `-tags=native_fixture` 生成独立的 `build/yingxia_core_fixture.aar`，使用独立测试 APK 包名和数据目录。`fixture.go`、`core_start_fixture.go` 与测试媒体由构建标签隔离；正式 AAR 未传此标签。两个 AAR 的标签和哈希记录见 `evidence/android_fixture_core_build.json`。

Go 单元测试使用隔离目录与合成网络响应，外网来源验收沿用 Windows 2.2 的已保存证据；本轮不重复探测所有来源。Windows Application Control 阻止了本轮测试 EXE 执行，随后将同源测试交叉编译为 Android x86_64，在自建 API35 模拟器实际执行：14 个后台顶层测试、5 个结构子例与 1 个生命周期测试全部通过，日志为 `evidence/android_engine_tests.log` 和 `evidence/android_core_tests.log`；`go vet -mod=vendor ./...` 通过。测试目录为模拟器 `/data/local/tmp/yingxia_22_goqa_20261015/tmp`，没有操作应用数据。实际 APK 运行结果另见客户端 evidence。

应用源码为 GPL-3.0-or-later，许可证与上游出处保留在源码中。源码交付排除 SDK、私钥、用户数据、工具与构建缓存；AAR 和证据按交付清单独立收录。
