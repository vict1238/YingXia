# v1.3.0 验证记录

日期：2026-10-14；Windows x64，Go 1.27.2 windows/amd64。原始记录位于 `validation_v1.3.0/`。

## 本轮结果

- Go 离线 vendor 回归：195 条 PASS 记录（含顶层及子用例），3 个手动入口跳过。测试 EXE 编译到工作区后执行；`go_test.txt` 保留完整输出。
- `go vet -mod=vendor ./...`：通过，输出文件 `go_vet.txt` 为空。
- JavaScript 语法检查：通过。发布 EXE 再次构建字节一致，历史 v1.0 至 v1.2.1 EXE 哈希不变；重建证据见 `build_reproducibility.json`。
- 原浏览器 UI 17 项、搜索筛选 10 项、暂停专项 12 项：全部通过。Chrome 155.0.8059.39 使用合成目录、WebM/HLS 和隔离数据，记录分别为 `browser_tests.json`、`search_filter_browser_tests.json`、`playback_pause_tests.json`。
- Windows 发布 EXE 后台回归 8 项：通过，覆盖内嵌来源/HLS、中文与空格目录、会话、演示素材排除、单实例、界面、退出和收藏重启保存。此组以 `--no-browser` 运行；`release_tests.json` 的界面检查由独立无头浏览器完成。
- Windows 发布 EXE 自有窗口回归 12 项：通过，见 `native_window_tests.json`。测试对象 SHA-256 为 `8fe7b69df5dc200af57097d8113f5106d3afcf45cae7210af373f459fd328d45`；WebView2 Runtime 为 155.0.4283.45。`native_window.png` 记录包含 Windows 标题栏的实际窗口。
- 原生边界专项 4 项：通过，见 `native_policy_test.txt`，覆盖 COM 字符串/BOOL/事件 token 的真实 Windows ABI、回调接口生命周期、顶层导航和外部窗口策略；包含在上述 195 条 PASS 中，不重复累加。

## 独立窗口实测

发布 EXE 自己拥有唯一的 Win32 主窗口，渲染进程为其 `msedgewebview2.exe` 子进程，没有本程序的 Edge/Chrome `--app` 窗口。四个页面和 17 个内置来源加载，搜索选择后合成 WebM 有实际解码帧和播放时间推进。

通过 Windows 窗口消息实际最小化后，视频暂停且时间冻结；恢复保持暂停，手动播放复用原播放器和位置。全屏移除标题栏并覆盖实际显示器，退出后恢复原矩形和窗口样式。重复启动恢复已有主窗口且沿用首个后台 PID。关闭窗口或 API 退出均结束 EXE、清理会话及窗口。

脚本外部顶层导航不能替换应用界面；无用户手势的弹窗不生成嵌入页或外部浏览器进程。普通启动可显示主窗口且没有测试使用的调试端口。调试只通过 `YINGXIA_WEBVIEW_DEBUG_PORT` 显式启用并绑定回环地址。

`native_window_harness_tests.json` 是另一次 12 项测试，使用显式 Go 测试入口调用生产窗口代码，标记 `production_gui_tested=false`；发布 EXE 的证据是 `native_window_tests.json`，标记 `production_gui_tested=true`，两者分开保留。

## 实现与范围

窗口使用 go-webview2 的 `pkg/edge`、`pkg/combridge` 和默认纯 Go loader。没有导入该依赖有初始化回调问题的生成包 `pkg/webview2`；原生策略在本工程声明所需的最小 COM 接口，按继承顺序、指针输出和按值参数处理边界，没有修改 vendor 来绕过问题。

早期中间构建曾被本机应用控制阻止。修订后的最终发布 EXE 已在同一机器启动并完成上述后台及窗口测试，过程中未更改系统防护策略。EXE 仍未数字签名；此记录不代表其他电脑、SmartScreen 或所有防护配置的验证。

所有回归使用临时数据目录和独立配置，没有改动正常收藏或日常浏览器账号。目录和视频为合成测试内容；生产 EXE 不包含测试视频。本轮不重新执行真实第三方站点播放，不以旧版搜索成功数量表示当前来源健康状态。

本轮未使用 race detector。全屏检查中的 CDP 合成 Escape 成功退出；物理键盘 Escape/F11、多显示器及不同 DPI、所有代理分支、缺失 Runtime 的干净系统、真实影片完整播放未分别验收。精确构建、重建和发布文件信息见 `BUILD_INFO.json` 及同目录相关记录。
