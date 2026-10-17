# v1.0 实现结构

Python 标准库 HTTP 后台 + 原生 HTML/CSS/JavaScript 界面 + 已安装 Edge/Chrome 应用窗口。
这一版没有采用 PySide6：为避免为用户安装大型 Qt 与额外浏览器内核，使用已有浏览器。
本机服务随机端口、绑定 127.0.0.1。默认禁用第三方联网，需用户确认。

## 模块

- `main.py`：命令行、用户目录、专用浏览器窗口、关闭与空闲回收。
- `yingxia/models.py`：作品/来源组/集数、标题清洗、中文数字、严格集数匹配。
- `yingxia/security.py`：公网地址检查、平台白名单、敏感查询参数清理。
- `yingxia/net.py`：限额、超时、TLS 校验、重定向校验；不附带账号 Cookie。
- `yingxia/catalog.py`：无损云联想/详情接口、5 分钟缓存、静态平台入口检索。
- `yingxia/service.py`：目录与播放器适配、同集绑定、诊断、用户确认边界。
- `yingxia/storage.py`：SQLite 收藏、设置、最近打开记录及绑定。
- `yingxia/server.py`：本机服务、随机会话/CSRF、Host/Origin、CSP、媒体 Range。
- `yingxia/web/`：界面与本机合成测试视频；不加载第三方前端框架。

## 播放状态

resolve 返回 `kind = media | page | external`，并始终保留 `playback_verified:false`。
media 使用 HTML5 video；page 使用 sandbox iframe；external 由用户打开官方网页。

第三方页面的 `load` 事件只更新“网页已加载 / 待确认”。
原生 video 检测视频帧、非暂停且非 seeking 的时间推进，记录 `browser_time_advanced`。
手工确认记录 `user_confirmed`。二者不能提升为“整个来源已可用”。

## 接口

GET：state、detail、library、bindings、report。
POST：search、settings、import-platform、bind、match、resolve、favorite、progress、clear-history、diagnose、ping、quit。
API 响应统一为 `{ok:true,data:...}` 或 `{ok:false,error:code,message:...}`。
所有 API 需本机会话，所有写入另需 CSRF，校验 Host / Origin / Sec-Fetch-Site。
每个端口使用独立会话 Cookie 名，避免两次启动的 Cookie 冲突。

## 非目标

不是通用网络代理，不缓存影视文件，不抓取账号，不绕过 DRM。
不提供完备平台目录协议；静态搜索失效时只能回到官方搜索和用户导入流程。
公网 URL / DNS 检查与 iframe 是风险降低措施，不是恶意站点全链路安全认证。
专用浏览器是单独配置目录，不是虚拟机，也不能替代操作系统安全措施。
