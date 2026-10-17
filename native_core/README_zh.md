# 映匣原生版共享后台

此目录供 Windows Flutter 窗口和 Android APK 共用。界面和视频解码由原生客户端负责；后台不包含 Chrome、WebView2、HTML 界面、JavaScript 执行器或浏览器取流组件。

`core.go` 只导出可由 gomobile 绑定的 `Start(dataDir)`、`Stop()`、`Running()`、`AwaitStopped()`。原后台业务位于 `internal/engine`，保留片源配置、Kazumi 静态规则、影视搜索和可用性检测、配置自动更新、媒体接力、本地收藏和记录，以及 URL/DNS/TLS 校验。Windows `cmd/native_backend` 启动后经标准输出管道返回一行连接信息，Android 通过 JNI 直接取得相同信息。

连接信息含 `origin`、`cookie`、`csrf`、`version`。监听地址固定为 `127.0.0.1` 随机端口，所有 HTTP 路径均要求会话，写接口要求 CSRF。凭据只交给宿主，不通过根页面、bootstrap 接口或日志发布。宿主关闭标准输入管道或调用 `/api/quit` 时后台退出，取消查询和更新并释放数据锁。

Windows 默认沿用 `%LOCALAPPDATA%\YingXia\standalone`，Android 使用应用私有目录。两个 Windows 版本共用数据锁，避免同时覆盖收藏和记录。清空记录成功时会使之前尚未结束的播放解析失效，防止延迟返回的请求重新写入历史；新播放仍正常记录。停止和重新启动按顺序完成，退出中的后台会封闭写入，再释放数据锁。

播放器使用媒体直连或可由静态 HTML / iframe 解析的地址。仅能访问动态网页且没有可提取媒体的候选在搜索时排后；原生版不会为了这些入口启动浏览器。已有 `page`、`external` 返回类型保留作为无法在原生播放器播放的结果，客户端显示更换片源提示。此边界应随软件交付说明保留。

构建 Windows 后台（本目录内）：

```powershell
& '../.tools/next/go/bin/go.exe' build -mod=vendor -trimpath -ldflags='-s -w -H windowsgui' -o build/yingxia_backend.exe ./cmd/native_backend
```

构建 Android AAR 时使用已初始化的 gomobile 工具和 Android NDK，绑定本目录的根包而非 `internal/engine`。桥接参考 `android_bridge/MainActivity.kt`；实际包名须与客户端一致，Flutter 通道名固定为 `yingxia/core`，客户端需要网络权限和回环 HTTP 播放支持。

本轮使用未修改的上游 gomobile / gobind，固定版本为 `v0.0.0-20261011034528-197770444867`。在本目录中设置当前进程的 `JAVA_HOME`（JDK 17）、`ANDROID_HOME` / `ANDROID_SDK_ROOT`（SDK 根目录）、`ANDROID_NDK_HOME`（其下 `ndk/28.2.13676358`），并将 Go 1.27.2、JDK、gomobile 的可执行目录加入当前进程 PATH。`GOPATH` 和 `GOBIN` 可指向工作目录中的工具缓存，`GOMODCACHE` 可用 Go 默认缓存；不需修改系统环境或工具源码。完整工具安装和正式绑定命令为：

```powershell
$env:GOTOOLCHAIN = 'local'
$env:GOFLAGS = '-mod=mod'
go install golang.org/x/mobile/cmd/gomobile@v0.0.0-20261011034528-197770444867
go install golang.org/x/mobile/cmd/gobind@v0.0.0-20261011034528-197770444867
gomobile init
gomobile bind '-target=android/arm64,android/amd64,android/arm' -androidapi=26 -trimpath '-ldflags=-s -w' -o build/yingxia_core.aar .
```

正式绑定不传 `-tags=native_fixture`。后续 APK 构建只使用上述正式 AAR；独立测试 APK 的 fixture AAR 不放入发布源码中的 `app/libs`。

本轮验证：6 个 Go 回归检查私有接口及清空历史持久化、失败回滚、清空时仍在等待网络的解析、退出中旧后台写入、静态媒体与动态来源排序，以及并发停止与重新启动；Windows 发布后台进程 9 项管道、接口和跨进程重启持久化检查见 `evidence/windows_transport.json`。独立合成测试后台 14 项检查覆盖真实媒体接力、WebM Range、HLS 主清单、子清单和分片，见 `evidence/windows_fixture_transport.json`。Go vet 通过。Android AAR 已用 NDK 28.2 和 gomobile 固定版本编译，包含 API 26 的 `arm64-v8a`、`armeabi-v7a`、`x86_64` JNI 库，导出 Java API 经 javap 检查。APK 安装和解码验证以另存的实际客户端记录为准。

`native_fixture` 是仅测试构建标签，编入自生成 WebM/HLS 与测试目录；正常 Windows 后台和 Android 发布 AAR 构建均不设置此标签。Android 测试 AAR 经独立 APK 包名使用私有测试目录，不混入交付软件。

最终 Android 发布 APK 的 21 项模拟器运行检查全部通过，报告位于 `../native_client/evidence/android/android_runtime_tests.json`。发布包的三个 Go JNI 库经标准剥离后的字节比对均与正式 AAR 一致；独立测试 APK 的 12 个 AOT/Flutter/mpv/播放器辅助库均与正式包一致。最终 HLS 合成测试采用 30 秒连续编码、15 个独立分片、900 帧递增时间戳；此前重复时间线分片在模拟器上的停顿仍保留为单独诊断记录，未计作正常结束测试通过。

应用源码为 GPL-3.0-or-later，保留源配置中的上游出处。依赖固定在 `go.mod` 与 `vendor`，其许可证随各模块保存。
