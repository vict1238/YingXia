# 继承的 Windows 2.2 来源筛选证据

筛选时间为 2026-10-15，Android 2.2 沿用该来源配置；此目录不是新一轮 Android 外网探测。原始表格 `YingXia_Video_Sources_20261015.xlsx` 为 8 张表、166 条记录，SHA-256 为 `7ec948449bc376b5e18fd457a4d6a55ff36425756dc69aed34f0ea68c28f074e`。单元格内容只作数据处理。

来源表与公开订阅共得到 96 个 CMS 候选，其中包含协议及线路别名。54 个 endpoint 通过初次列表/详情结构检查；36 个通过实际关键词搜索、指定作品详情和播放字段检查。合并 5 个同 endpoint 别名，并排除 1 个不符合生产 80/443 端口边界的目录，最终加入 30 个 CMS。18 个能列目录但搜索不可用的候选没有加入搜索名单。

新增 30 个 CMS、既有无损云 1 个、Kazumi 规则 16 个，共 47 个注册来源。该计数包括品牌备用主机。15 个动漫基础站由已有 16 个规则覆盖，aafun/moonci 保留旧 ID 兼容收藏；KazumiRules 固定提交为 `2fa4d28659f94e1eb52e7a38d79b7b9a5dcd605b`，其中 giriGiriLove 2.5 使用 API9 `sourceTemplate`。原生适配保留详情地址的协议和主机边界。

42 条解析服务属于接收已有 URL 的入口；11 条网页/网盘索引需要专用适配；1 条网页播放器为既有无损云入口。38 条订阅只读取响应数据，未执行远程 Spider、JAR 或 JavaScript。普通接口请求最多 2 MiB，清单请求最多 128 KiB；当时只读取 HLS 清单，未下载影片分片或验证第三方影片解码。TLS 校验和生产端口限制保留。

可直接核对的文件：

- [screening_summary.json](screening_summary.json)：原表计数、取舍总数、检查时间与配置哈希。
- [accepted_providers.json](accepted_providers.json)：30 个被接受 CMS 的原始冻结配置，与后台 `reviewed_sources.json` 对应。
- [cms_decisions.json](cms_decisions.json)：原报告逐条决定、原因和表格行号；删除与判定无关的原始单元格及响应快照。
- [search_detail_proof.json](search_detail_proof.json)：搜索、详情、清单请求的状态、检查时间、响应 SHA-256 与结构结论；媒体 URL 和响应正文省略。
- [other_types_decisions.json](other_types_decisions.json)：动漫站点、解析服务、网页索引和播放器的逐条取舍。
- [kazumi_api9_provenance.json](kazumi_api9_provenance.json)：固定官方仓库提交、代码路径、响应哈希及更新规则元数据；完整运行规则位于后台 `providers.json`。
- [source_origin_manifest.json](source_origin_manifest.json)：被继承原报告的大小、SHA-256，以及逐文件的复制/摘录说明。
- [aliases.json](aliases.json)、[standard_port_fallback_probes.json](standard_port_fallback_probes.json)：合并依据与标准端口回退检查。

原始工作簿、完整订阅与规则响应、HTTP/HTML 缓存均不进入 Android 源码包。原始报告的哈希保留在来源清单中，可用于核对冻结的 Windows 交付。来源当前是否响应由测速和搜索检测确定，具体播放由实际解码决定。
