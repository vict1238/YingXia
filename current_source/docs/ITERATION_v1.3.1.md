# 映匣 v1.3.1

修复 Windows 独立窗口中“清空记录”按钮无效的问题。沿用 v1.3.0 的 Win32/WebView2 宿主、界面、影视源和数据目录。

## 原因与行为

旧按钮调用浏览器 `confirm()`，而 WebView2 宿主禁用了默认脚本对话框，因此确认返回取消，清空请求没有发出。

按钮改为软件内的 HTML 对话框，沿用现有配色，提供“取消”和“清空”。默认焦点落在“取消”，Escape 可取消。确认后暂时禁用按钮，等待存储清空和界面刷新；成功显示“暂无记录”，请求失败保留记录并显示错误，可重试。清空不修改收藏、绑定和设置。

为收藏与记录刷新增加请求序号，清空前发起的旧查询即使迟到，也不能重新绘制已删除的记录。没有改变宿主脚本弹窗策略或本机 API 的会话、CSRF 和来源校验。

## 验证

`docs/clear_history_smoke.cjs` 使用最终发布 EXE、实际 WebView2 窗口、临时中文路径和合成库。8 项专项检查通过，覆盖取消与 Escape、模拟存储错误、重复点击保护、查询响应竞态、真实磁盘写入、收藏/绑定/设置保留以及完整 EXE 重启后的空记录。Go vendor 回归也重新执行，195 条 PASS、3 个手动入口跳过，详见 `VALIDATION.md` 和 `validation_v1.3.1/go_test.txt`。

测试对象 `YingXia_v1.3.1_Windows_x64.exe` SHA-256：`ab1a8321fc885f392b69fd7293f3b24e189eaa0e5ee22098b8f67e163920edb7`，WebView2 Runtime：155.0.4283.45。原始报告和截图位于 `validation_v1.3.1/`；`clear_history_dev/` 是最终发布前的临时开发构建记录，正式发布证据为该目录根部的 `clear_history_tests.json`。

v1.3.0 的完整说明、构建与验证已归档到 `history_v1.3.0/`。本包仅交付修复后的 WebView2 Windows 版；无网页内核 Windows 版和 Android 版单独开发、构建和验证。
