# v1.3.2 验证记录

日期：2026-10-14；Windows x64，Go 1.27.2 windows/amd64。当前原始记录位于 `validation_v1.3.2/`；历史版本保留在对应 `validation_v*` 与 `history_v*`。

## 当前发布 EXE 的清空记录检查

最终 EXE：`YingXia_v1.3.2_Windows_x64.exe`，SHA-256 `63ea2d697008c28b9d423c0a803ecc420760fb8d2f6de39e39e03724573ab84e`，11,376,128 字节。Windows 文件版本和产品版本为 1.3.2；WebView2 Runtime 155.0.4283.45。

`clear_history_smoke.cjs` 启动该 EXE 的自有 Windows/WebView2 窗口，通过明确启用的回环调试端口操作实际界面。数据目录、收藏、历史、绑定均为独立合成数据；测试过程没有访问第三方影片。10 项检查全部通过：

1. 实际软件内确认框存在，前端不调用浏览器 `confirm()`。
2. “取消”保留界面和磁盘记录。
3. Escape 取消而不清空。
4. 模拟失败响应显示错误、保留记录并允许重试。这是前端失败处理测试，实际磁盘失败由 Go 回归覆盖。
5. 清空过程中禁用重复点击和取消，只发出一个清空请求。
6. 清空等待最新库刷新；清空前的旧查询迟到不会恢复界面上的记录。
7. 真实 API 清空磁盘历史，收藏、绑定、设置保留。
8. 完整退出并重启同一个 EXE，历史仍为空，收藏仍可见。
9. 实际生产后台的目录详情响应被测试代理暂时阻塞，清空后释放响应，先前开始的 `resolve` 返回空历史 ID，磁盘记录保持为空。
10. 清空后新的主动播放正常生成记录，再经实际按钮清空、退出重启后历史为空。

第 9 项使用公开域名 example.com 的合成目录 URL，通过仅对测试子进程设置的回环 HTTP 代理返回数据；没有修改系统代理、DNS、TLS 策略或生产代码。代理只服务该测试目录，浏览器未请求任何外部页面。报告没有 JavaScript 异常或浏览器脚本弹窗。原始报告 `clear_history_tests.json`、执行输出 `clear_history_smoke.log`，截图 `clear_history_dialog.png`、`clear_history_empty.png`。

## Go 与构建验证

完整 vendor 回归重新执行：197 条 PASS（含顶层和子用例）、3 个手动入口 SKIP、0 FAIL，最终 PASS。实际日志 `validation_v1.3.2/go_test.txt`；测试 EXE 在工作区编译后执行。`go vet -mod=vendor ./...` 退出 0，日志 `go_vet.txt` 为空。

新增两个回归覆盖：解析在详情请求处等待，清空成功后不会重新生成记录，而清空后新的播放正常保存；真实文件替换失败时保留原记录和清空代次，先前等待的解析仍可保存。失败条件以同名目录阻止原子替换，独立于测试进程权限。

前端及测试脚本 Node 语法检查通过。发布 EXE 用相同依赖和资源重新构建，哈希相同，见 `build_reproducibility.json`。构建参数和资源哈希见 `BUILD_INFO.json`、`WINDOWS_RESOURCES.json`。

## 范围

存储继续使用 schema 1 的 `library.json`，清空代次只存在本次进程内；并未改变旧数据格式。网络解析从进入 `Resolve` 时取得代次，成功清空后使旧解析不能写入新记录。清空失败不会改变代次。迟到的播放进度不会创建不存在的记录。

此前 v1.3.0 的浏览器 UI 17 项、搜索筛选 10 项、暂停 12 项、发布后台 8 项和自有窗口 12 项，及 v1.3.1 的确认框 8 项，均为各自历史版本的证据；未将它们计入 v1.3.2 的 10 项专项。

本轮没有真实第三方影片完整播放或 race detector 验证。生产 EXE 不含测试视频，仍需要系统的 Microsoft WebView2 Runtime，未数字签名。无网页内核 Windows 原生版及 Android APK 使用各自独立构建和实际验证记录。