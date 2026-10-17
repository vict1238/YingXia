# 独立 EXE 构建与验证记录

## 实际生成
- 文件：`YingXia_v1.0_Windows_x64.exe`
- 大小：7,226,368 字节。
- SHA-256：`e7d47fe49c185c1d682c52713150f1098a19beda61e6e70128ddba21768a238b`
- 格式：PE32+ / 0x8664 / Windows GUI 子系统 2。
- Authenticode：未签名，PE Security Directory 为 0。
- 用 Go 1.23.2 交叉编译为 Windows x64，不包含 Python DLL，不是运行时下载器。
- 静态导入表仅系统 kernel32.dll；网络、浏览器与对话框使用系统 API。
- 页面、CSS、JS、来源配置和两个自生成 WebM 均内置，不依赖旁边的源码文件。

## 实际执行的检查
1. `go test -race -count=1 -v ./...`：110 个子用例通过，连同顶层测试共 119 条 PASS 记录。含元数据、多源、权限、持久化、HTTP 和关闭服务检查。Race detector 没有报告竞态。
2. 生产 Linux 后台进程：9 项检查，通过真实 HTTP 获取会话、离线目录、媒体 Range、收藏落盘、重复进程复用、关闭及重启保持。使用带中文和空格的数据目录。
3. Chromium 界面：17 项流程通过；无未捕获 JavaScript 错误。合成媒体观测到 34 帧、时间推进到 1.230667 秒。界面经内存加载、本机 API 桥接，不修改浏览器策略、不宣称浏览器 URL 导航已验收。
4. `go vet ./...` 及 `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go vet ./...` 无输出，通过静态检查。
5. Windows 目标真实编译成功，`objdump` 与二进制结构解析确认目标格式。

## 不能据此声称
没有 Windows 真机、Wine 或 Windows VM 运行记录。未实测 Windows 的 Edge/Chrome 启动、系统代理分支、Defender 兼容或 SmartScreen 行为。没有验证任何第三方影片实播。所有在线目录单元/界面测试都显式使用 fixtures；并未混入生产搜索。

## 与 Python v1.0 的不同
后台为 Go 标准库移植；用户不必安装 Python。数据由 SQLite 改为独立子目录的 JSON，不自动迁移旧数据库。平台选择器改用固定顺序，避免 JSON 对象键顺序改变默认平台。窗口是已安装浏览器的应用窗口，不是打包 Chromium。
