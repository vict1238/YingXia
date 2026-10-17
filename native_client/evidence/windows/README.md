# Windows 原生版实测记录

最终软件为 `releases/YingXia_v2.0.0_Native_Windows_x64.exe`，60,237,824 字节，SHA-256：

`dd2062a005e12626033ab5f4281be7a540c4616a2f9d0edae16654cbb6889f3b`

Windows 文件版本及产品版本均为 2.0.0。运行包含 166 个文件，完整清单见 `runtime_manifest.json`；运行包 SHA-256 为 `70f2473493d3b5601cb3dab2236ca5d547ea0674f01bb797d73b692d272afb8c`。

`native_windows_tests.json` / `native_windows_smoke.log` 记录最终 EXE 的 10 项实际窗口检查，全部通过。前 7 项使用软件所含的正式 Go 后台：自动显示 Flutter 窗口、四页切换、取消清空、实际只读文件写入失败后的保留与重试、磁盘记录清空及收藏/绑定/设置保留、重复启动恢复原窗口、完整退出重启后记录仍空且后台子进程退出。实际进程树只包含启动器、Flutter 窗口和 Go 后台。

后 3 项使用同一个正式 EXE 内的 Flutter 窗口与 libmpv，通过明确的开发测试环境变量切换到另行编译的 `native_fixture` 后台：自制 WebM 解码/时间推进/历史记录、离开工作台暂停及手动继续、HLS 经鉴权中转解码、最小化后暂停且恢复窗口仍暂停。该测试后台和自制媒体未打入发布 EXE；这些检查不能代表任意第三方影视源可播放。

可视截图均从最终 EXE 的实际窗口获取：`native_home.png`、`native_history_cleared.png`、`native_webm_player.png`、`native_hls_player.png`、`native_paused_player.png`。播放器截图分别显示暂停的 WebM 实际画面、播放中的 HLS 实际画面以及最小化后暂停的画面。

`launcher_tests.txt` 为最终启动器源码及运行包的 Go 回归结果：10 条 PASS（含 6 个路径拒绝子用例）、1 条仅供子进程测试调用的 harness SKIP、0 条 FAIL。`launcher_vet.txt` 为空且命令退出 0。检查覆盖解包与缓存损坏修复、路径/重复文件/内容损坏拒绝、删除范围限制及真实 Windows Job 对子进程的关闭行为。

复验入口为 `native_client/tools/windows_native_smoke.cjs`，辅助脚本 `windows_ui_probe.ps1` 只操作按进程 ID 和窗口类匹配的该软件窗口，并使用隔离的临时数据与运行缓存。回归会关闭测试窗口，保留的截图与日志在本目录。

`diagnostics/` 保存从未交付的第一个候选 EXE 的失败记录，候选 SHA 为 `9739e43c6ba8a51151ac3e7feed383187888ed871f3130b0cbdfa4ba3d60cf3f`。该记录发现了原生播放器高频进度回调无法积累播放证据的问题；现已修复，并以最终 SHA 重新执行全部 10 项检查。诊断记录不是最终版本测试结果。


第二个未交付候选 SHA 为 `e07b69230d32d48252828fe97b6fde317ef71d0a24147cb08e5531ae1ce8e341`，保存为 diagnostics/candidate2_*。该候选完成上述窗口与播放检查后，另行源码审阅确认了正在解析的请求可在清空后新增历史、以及后台停止和启动交错的问题。最终版本已换用修正后台，SHA 为 `0e2febcd74110cda5547da784efaccfe7025e68ae65d6219173b2502acd24077`，并重新执行所有 10 项实际窗口检查。受控解析竞态和生命周期回归详见 native_core/evidence；当前测试媒体后台为单独的 fixture_v2，哈希保存在 native_windows_tests.json。
