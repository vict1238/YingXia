# v1.3.1 验证记录

日期：2026-10-14；Windows x64，Go 1.27.2 windows/amd64。当前原始记录位于 `validation_v1.3.1/`；v1.3.0 原始记录保留在 `validation_v1.3.0/`，说明归档至 `history_v1.3.0/`。

## 当前发布 EXE 的清空记录检查

最终 EXE SHA-256：`ab1a8321fc885f392b69fd7293f3b24e189eaa0e5ee22098b8f67e163920edb7`。WebView2 Runtime：155.0.4283.45。

`clear_history_smoke.cjs` 直接启动该 EXE 的自有 Windows/WebView2 窗口，通过显式启用的回环调试端口操作实际界面。它使用独立临时数据目录、两个合成历史记录、一个合成收藏及绑定，无第三方请求，没有修改正常用户库。

8 项检查通过：

1. 发布 EXE 编入软件内确认框，前端不再调用浏览器 `confirm()`。
2. “取消”保留界面及磁盘上的两条记录。
3. Escape 可取消，没有执行清空。
4. 模拟存储失败响应显示错误，保留界面和磁盘记录，允许重试。此项是前端失败处理验证，不是实际文件权限故障测试。
5. 清空过程中禁用重复点击和取消，只有一个清空请求。
6. 成功后等待新库刷新；清空前的旧查询迟到，不会重新显示已删除的记录。
7. 真实 API 清空后 `library.json` 的历史为空；收藏、绑定、设置保持原值。
8. 完整退出并重启同一个 EXE，历史仍为空，收藏仍可见。

没有 JavaScript 异常、浏览器脚本弹窗或外部请求。报告为 `clear_history_tests.json`，截图为 `clear_history_dialog.png` 和 `clear_history_empty.png`。`clear_history_dev/` 保留较早临时测试构建的专项证据，与上述最终发布 EXE 分开。

## Go 与既有功能回归

本轮 Go vendor 回归重新执行：195 条 PASS 记录（含顶层及子用例），3 个手动入口跳过，0 条 FAIL，最终结果为 PASS。实际日志为 `validation_v1.3.1/go_test.txt`，测试 EXE 编译到工作区后执行。v1.3.0 的旧日志保留在 `validation_v1.3.0/`。

本轮 `go vet -mod=vendor ./...` 退出码为 0，输出日志 `validation_v1.3.1/go_vet.txt` 为空。前端和专项测试脚本的 Node 语法检查通过。

此前 v1.3.0 的浏览器 UI 17 项、搜索筛选 10 项、暂停专项 12 项、发布 EXE 后台 8 项和自有窗口 12 项均通过。记录分别为 `validation_v1.3.0/browser_tests.json`、`search_filter_browser_tests.json`、`playback_pause_tests.json`、`release_tests.json` 和 `native_window_tests.json`。该历史版验证实际合成视频解码、最小化暂停、全屏及恢复、重复启动、窗口关闭和 API 退出。本轮清空记录专项不替代这些完整测试组。

## 范围与限制

修改仅涉及确认交互和库刷新，没有改变本机存储格式、宿主策略、来源解析、播放器或窗口生命周期。现有本机会话、CSRF、Host/Origin、公共 URL/DNS 检查保留。

本轮使用合成记录，无真实第三方影片完整播放检查。生产 EXE 不含测试视频，运行仍需要 Microsoft WebView2 Runtime。缺失 Runtime 的干净系统、多显示器/不同 DPI、所有代理分支、SmartScreen/所有防护配置和真实影片完整播放不在本轮专项证据范围内。EXE 未数字签名，本轮没有使用 race detector。

精确构建、重建和发布文件信息见 `BUILD_INFO.json` 及对应原始记录。本包为 WebView2 Windows 版；无网页内核 Windows 版和 Android 版需要各自的构建及验证证据。
