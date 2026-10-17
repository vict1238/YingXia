"""Review non-CMS workbook rows without executing JavaScript, Spider or JAR code."""
from pathlib import Path
from urllib.parse import urlsplit, quote
from concurrent.futures import ThreadPoolExecutor,as_completed
from datetime import datetime,timezone
import hashlib,json,re
import probe_sources as p
HERE=Path(__file__).resolve().parent
baseline=p.BASE["providers"]
by_base={}
for x in baseline:
    if x["kind"]=="kazumi":by_base.setdefault(p.norm(x["rule"]["baseURL"]),[]).append(x)

index_rec,index,_=p.fetch("https://raw.githubusercontent.com/Predidit/KazumiRules/main/index.json","kazumi_current_index",cap=500000)
latest={x["name"]:x for x in index or [] if isinstance(x,dict) and x.get("name")}

def site_probe(row):
    out=dict(row);rec,_,body=p.fetch(row["url"],row["record_id"]+"_base",cap=1048576)
    out["base_http_probe"]=rec
    if row["sheet"]=="02_动漫站点":
        existing=by_base.get(p.norm(row["url"]),[])
        out.update(decision="existing_kazumi",existing_provider_ids=[x["id"] for x in existing],reason="现有16条Kazumi规则已覆盖表内15个基础站；aafun/moonci相同站点保留旧ID兼容收藏记录",rule_versions=[{"name":x["rule"]["name"],"built_in":x["rule"]["version"],"current_index":latest.get(x["rule"]["name"],{}).get("version"),"requires_rule_update":x["rule"]["version"]!=latest.get(x["rule"]["name"],{}).get("version")} for x in existing])
    else:
        out.update(decision="needs_site_or_cloud_adapter",reason="提供网页/网盘索引，未提供当前MacCMS JSON接口或现有Kazumi规则；TVBox Spider/网盘授权不能直接作为原生搜索源",body_contains_challenge=bool(re.search(r"Just a moment|Checking your browser|cf-chl-|验证码",body,re.I)))
    print(row["record_id"],out["decision"],rec.get("status",rec.get("error")),flush=True)
    return out

sites=list(p.rows("02_动漫站点"))+list(p.rows("04_网页网盘"))
site_results=[]
with ThreadPoolExecutor(max_workers=16) as pool:
    for f in as_completed([pool.submit(site_probe,x) for x in sites]):site_results.append(f.result())
other=[]
existing_parser_templates={x["template"].replace("{url}",""):x["id"] for x in p.BASE["parsers"]}
for row in p.rows("03_解析服务"):
    out=dict(row)
    if row["url"] in existing_parser_templates:
        out.update(decision="existing_parser_template",existing_parser_id=existing_parser_templates[row["url"]],reason="已在原配置，解析入口不增加搜索目录；原生播放不启动浏览器")
    else:out.update(decision="parser_not_catalog",reason="接收已有播放页URL的解析服务，不提供片名目录；现有原生流程缺少已验证的媒体JSON契约，未作为CMS加入")
    out["network_probe_performed"]=False;out["decode_tested"]=False;other.append(out)
for row in p.rows("05_媒体播放器"):
    other.append({**row,"decision":"existing_web_player_reference","reason":"已有无损云网页播放器配置，原生客户端直接交libmpv播放媒体，不增加目录","network_probe_performed":False,"decode_tested":False})
report={"checked_at":datetime.now(timezone.utc).isoformat(),"kazumi_current_index":index_rec,"kazumi_index_count":len(latest),"anime_and_web_sites":sorted(site_results,key=lambda x:x["record_id"]),"parsers_and_media_player":other,"scope":{"base_http_response_is_not_search_or_playback":True,"executed_remote_javascript":False,"executed_remote_spider_or_jar":False,"downloaded_media_segments":0}}
p.save("other_types_screening_report.json",report)
print("KAZUMI_INDEX",len(latest),"RULE_VERSION_CHANGES",sum(x.get("requires_rule_update",False) for a in site_results for x in a.get("rule_versions",[])))
