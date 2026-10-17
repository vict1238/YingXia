"""Freeze importable sources using the observed search/detail contracts."""
from pathlib import Path
from urllib.parse import urlsplit
from datetime import datetime,timezone
import hashlib,json
import probe_sources as p
HERE=Path(__file__).resolve().parent
load=lambda n:json.loads((HERE/n).read_text(encoding="utf-8"))
initial=load("cms_initial_probes.json");extra=load("expanded_probes.json");checks=load("search_detail_playlist_probes.json")
observed={x.get("record_id"):x for x in checks}
aliases={"E008":("C031","实际HTTP重定向到同一个U酷目录"),"E013":("C032","实际HTTP重定向到同一个光速目录"),"E018":("C003","同主机at/json协议别名"),"E033":("C018","同主机from/ikm3u8线路筛选别名"),"C029":("E031","同主机金鹰线路筛选别名；保留不限定线路的目录")}
clean_names={"C001":"电影天堂资源","C006":"非凡资源 ffzy5.tv","C008":"爱奇艺资源","C020":"豆瓣资源 dbzy5.com","C024":"360资源 360zyzz.com","C025":"非凡资源 ffzyapi.com","C035":"豪华资源","C042":"非凡资源 cj.ffzyapi.com","C049":"非凡资源 ffzy1.tv","E006":"非凡资源 ffzy.tv","E009":"建安资源","E023":"最大资源 zuida.xyz","E031":"金鹰资源","E032":"1080资源"}
providers=[];records=[]
for row in sorted(initial+extra,key=lambda x:x["record_id"]):
    rid=row["record_id"];check=observed.get(rid,{})
    rec={k:v for k,v in row.items() if k not in ("probe",)}
    rec["probe"]=row.get("probe");rec["search_detail_playlist"]=check or None
    if urlsplit(row["url"]).port not in (None,80,443):
        rec.update(final_decision="unsupported_nonstandard_port",final_reason="现有生产网络边界只允许80/443；标准HTTP同路径404、HTTPS证书IP不匹配，保留原安全边界不导入",standard_port_fallback_evidence="standard_port_fallback_probes.json")
    elif rid in aliases:
        rec.update(final_decision="duplicate_alias",duplicate_of=aliases[rid][0],final_reason=aliases[rid][1])
    elif check.get("compatible_search_detail"):
        url=p.norm(row["probe"].get("final_url",row["url"]))
        provider={"id":"sheet_"+rid.lower(),"name":clean_names.get(rid,row["name"]),"kind":"cms","url":url,"origin":"reviewed_20261015","enabled":True}
        providers.append(provider);rec.update(final_decision="integrate",provider=provider,final_reason="搜索命中作品且单作品detail返回匹配ID、名称和播放字段")
    elif row["decision"]=="accepted_json":
        sr=check.get("search",{});reason=sr.get("error") or f"HTTP {sr.get('status')}搜索返回0条或未命中样本"
        if sr.get("body_file"):
            text=(HERE/sr["body_file"]).read_text(encoding="utf-8").strip()
            if len(text)<300:reason=text
        rec.update(final_decision="not_searchable_now",final_reason=reason)
    else:rec.update(final_decision=row["decision"],final_reason=row.get("reason",""))
    records.append(rec)
catalog={"schema":1,"revision":"20261015","providers":providers}
p.save("accepted_providers.json",catalog)
report={"checked_at":datetime.now(timezone.utc).isoformat(),"workbook_sha256":p.EXTRACT["sha256"],"workbook_size":p.EXTRACT["size"],"sheet_count":len(p.EXTRACT["sheets"]),"workbook_records":{"cms":59,"anime_base_sites":15,"parser_services":42,"web_or_cloud_sites":11,"media_player":1,"subscriptions":38,"total":166},"expanded_cms_candidates":len(extra),"tested_cms_endpoints":len(initial)+len(extra)-sum(x["decision"]=="duplicate_protocol" for x in initial+extra),"json_list_detail_valid":len(checks),"search_detail_contract_valid":sum(x.get("compatible_search_detail",False) for x in checks),"integrated_new_cms_count":len(providers),"catalog_sha256":hashlib.sha256((HERE/"accepted_providers.json").read_bytes()).hexdigest(),"scope":{"executed_remote_code":False,"downloaded_film_segments":0,"decode_playback_tested":False,"playlist_probe_is_not_playback":True,"request_body_limit":2097152,"playlist_body_limit":131072,"tls_verification_disabled":False},"records":records}
p.save("cms_screening_report.json",report)
print("FROZEN",len(providers),report["catalog_sha256"])
for item in providers:print(item["id"],item["name"],item["url"])
