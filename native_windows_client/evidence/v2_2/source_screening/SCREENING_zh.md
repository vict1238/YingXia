# 2026-10-15 视频源筛选

只读提取原表8个sheet、166条记录，SHA256 `7ec948449bc376b5e18fd457a4d6a55ff36425756dc69aed34f0ea68c28f074e`。单元格文字仅作数据处理。

新增30个可供原生客户端搜索的MacCMS JSON目录，与原17个注册源合计47个。计数包含品牌备用主机，不表示47个独立片库或47个已解码播放来源。

59条表内CMS加38订阅展开的37个不同endpoint，共96个候选。54个endpoint通过初次列表/详情JSON结构检查。进一步指定关键词与ID检测后，36个通过实际搜索命中、指定作品detail及播放字段检查；合并5个同endpoint协议/线路别名，并排除1个不符合既有80/443端口边界的站点，保留30个。18个虽然能列目录，但关键词搜索返回禁用/失败，不加入搜索名单。

完整决策在cms_screening_report.json，每条保留原表行号、URL、仓库证据、请求时间、状态、响应结构及SHA。原始表格内容在workbook_extracted.json。网络全响应和HTML在http/仅为本地研究缓存，交付源码包排除此目录，报告中的响应SHA仍可对比以后请求。

15个动漫基础站已由既有16条Kazumi规则覆盖，aafun/moonci保留旧ID兼容收藏。固定官方KazumiRules提交2fa4d28659f94e1eb52e7a38d79b7b9a5dcd605b的16条JSON在kazumi_current_pinned_registry.json；其中giriGiriLove由2.4变为2.5，API9增加sourceTemplate，需相应原生适配。官方搜索实测返回样本ID6547，GV6547详情包含选集容器。

42条解析服务均是接收已有URL的解析入口，不提供片名搜索目录；已有7条仅标注为已存在，其余未据此增加浏览器或执行远程脚本。11条网页/网盘索引缺少当前可直接使用的CMS JSON或Kazumi规则，逐条保留需要专用适配的原因。1条网页播放器为既有无损云参考入口，不增加搜索范围。38订阅仅解析HTTP响应数据，未执行Spider/JAR/JavaScript。

请求限制：普通接口最多2MiB，播放清单最多128KiB；只读取HLS清单，没有下载影片分片，也没有执行第三方影片解码实播。TLS验证与生产80/443端口限制均保留。来源是否能播放仍由软件的可达性检测和实际播放决定。

## 新增目录

| ID | 名称 | endpoint |
| --- | --- | --- |
| sheet_c001 | 电影天堂资源 | http://caiji.dyttzyapi.com/api.php/provide/vod |
| sheet_c003 | 如意资源 | https://cj.rycjapi.com/api.php/provide/vod |
| sheet_c004 | 暴风资源 | https://bfzyapi.com/api.php/provide/vod |
| sheet_c006 | 非凡资源 ffzy5.tv | http://ffzy5.tv/api.php/provide/vod |
| sheet_c007 | 360资源 | https://360zy.com/api.php/provide/vod |
| sheet_c008 | 爱奇艺资源 | https://www.iqiyizyapi.com/api.php/provide/vod |
| sheet_c010 | 极速资源 | https://jszyapi.com/api.php/provide/vod |
| sheet_c013 | 魔都资源 | https://www.mdzyapi.com/api.php/provide/vod |
| sheet_c014 | 最大资源 | https://api.zuidapi.com/api.php/provide/vod |
| sheet_c018 | iKun资源 | https://ikunzyapi.com/api.php/provide/vod |
| sheet_c022 | 猫眼资源 | https://api.maoyanapi.top/api.php/provide/vod |
| sheet_c024 | 360资源 360zyzz.com | https://360zyzz.com/api.php/provide/vod |
| sheet_c025 | 非凡资源 ffzyapi.com | https://api.ffzyapi.com/api.php/provide/vod |
| sheet_c026 | 新浪资源 | https://api.xinlangapi.com/xinlangapi.php/provide/vod |
| sheet_c028 | 速播资源 | https://subocaiji.com/api.php/provide/vod |
| sheet_c031 | U酷影视 | https://api.ukuapi88.com/api.php/provide/vod |
| sheet_c032 | 光速资源 | https://api.guangsuapi.com/api.php/provide/vod |
| sheet_c033 | 红牛资源 | https://www.hongniuzy2.com/api.php/provide/vod |
| sheet_c034 | 魔都动漫 | https://caiji.moduapi.cc/api.php/provide/vod |
| sheet_c035 | 豪华资源 | https://hhzyapi.com/api.php/provide/vod |
| sheet_c036 | 百度云资源 | https://api.apibdzy.com/api.php/provide/vod |
| sheet_c037 | 艾旦影视 | https://lovedan.net/api.php/provide/vod |
| sheet_c042 | 非凡资源 cj.ffzyapi.com | http://cj.ffzyapi.com/api.php/provide/vod |
| sheet_c049 | 非凡资源 ffzy1.tv | http://ffzy1.tv/api.php/provide/vod |
| sheet_c054 | 虎牙采集 | https://www.huyaapi.com/api.php/provide/vod/from/hym3u8 |
| sheet_c058 | 文才规则目录 | https://api.zeqaht.com/api.php/provide/vod |
| sheet_e006 | 非凡资源 ffzy.tv | http://ffzy.tv/api.php/provide/vod |
| sheet_e023 | 最大资源 zuida.xyz | https://zuida.xyz/api.php/provide/vod |
| sheet_e031 | 金鹰资源 | https://jyzyapi.com/provide/vod |
| sheet_e032 | 1080资源 | https://api.1080zyku.com/inc/apijson.php |

复现顺序：extract_workbook.py → probe_sources.py → qualify_candidates.py → check_port_compatibility.py → freeze_catalog.py；最终accepted_providers.json由freeze_catalog.py冻结。Kazumi取回与非CMS筛选分别见fetch_full_kazumi.py和screen_other_types.py。
