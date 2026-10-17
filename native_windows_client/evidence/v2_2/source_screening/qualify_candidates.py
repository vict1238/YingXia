"""Check actual search + detail and bounded playlist availability; never fetch film segments."""
from pathlib import Path
from urllib.parse import urlsplit
from concurrent.futures import ThreadPoolExecutor, as_completed
import json, re
import probe_sources as p

HERE=Path(__file__).resolve().parent
initial=json.loads((HERE/"cms_initial_probes.json").read_text(encoding="utf-8"))
expanded=json.loads((HERE/"expanded_probes.json").read_text(encoding="utf-8"))
items=[x for x in initial+expanded if x["decision"]=="accepted_json"]

def validrows(obj):
    if not isinstance(obj,dict) or not isinstance(obj.get("list"),list):return []
    return [x for x in obj["list"] if isinstance(x,dict) and str(x.get("vod_id","")).strip() and str(x.get("vod_name","")).strip()]

def check(item):
    out={"record_id":item["record_id"],"url":item["url"],"name":item["name"]}
    search,obj,_=p.fetch(p.query(item["url"],ac="detail",wd=item["sample_name"],pg="1"),item["record_id"]+"_search")
    rs=validrows(obj)
    out["search"]={**search,"query":item["sample_name"],"valid_items":len(rs),"matched_sample_id":any(str(x["vod_id"])==item["sample_id"] for x in rs)}
    detail,obj,_=p.fetch(p.query(item["url"],ac="detail",ids=item["sample_id"]),item["record_id"]+"_single_detail")
    rs=validrows(obj);match=next((x for x in rs if str(x["vod_id"])==item["sample_id"]),None)
    out["detail"]={**detail,"requested_id":item["sample_id"],"valid_items":len(rs),"matched_sample_id":match is not None,"has_play_fields":bool(match and match.get("vod_play_url"))}
    out["compatible_search_detail"]=bool(out["search"]["matched_sample_id"] and match and match.get("vod_play_url"))
    if match:
        # Prefer explicitly playable playlists; preserve media endpoints solely as data.
        raw=str(match.get("vod_play_url",""))
        urls=re.findall(r'https?://[^\s$#]+',raw)
        hls=next((u for u in urls if ".m3u8" in urlsplit(u).path.lower()),None)
        if hls:
            rec,_,body=p.fetch(hls,item["record_id"]+"_playlist",cap=131072)
            out["playlist"]={**rec,"hls_header":body.lstrip().startswith("#EXTM3U"),"media_segments_downloaded":0,"decode_tested":False}
        else:
            out["playlist"]={"checked":False,"reason":"no_explicit_hls_url_in_first_detail","media_segments_downloaded":0,"decode_tested":False}
    print(out["record_id"],"search/detail",out["compatible_search_detail"],"hls",out.get("playlist",{}).get("hls_header"),flush=True)
    return out

results=[]
with ThreadPoolExecutor(max_workers=18) as pool:
    for f in as_completed([pool.submit(check,x) for x in items]):
        try:results.append(f.result())
        except Exception as e:results.append({"error":str(e)})
p.save("search_detail_playlist_probes.json",sorted(results,key=lambda x:x.get("record_id","")))
print("COMPATIBLE",sum(x.get("compatible_search_detail",False) for x in results),flush=True)
