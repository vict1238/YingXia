from pathlib import Path
from concurrent.futures import ThreadPoolExecutor,as_completed
import probe_sources as p
checks=["http://154.219.117.232/jacloudapi.php/provide/vod","https://154.219.117.232/jacloudapi.php/provide/vod"]
def check(url):
    rec,obj,_=p.fetch(p.query(url,ac="detail",pg="1"),"E009_standard_"+url.split(":",1)[0])
    rec["cms_valid"]=bool(isinstance(obj,dict) and isinstance(obj.get("list"),list) and any(isinstance(x,dict) and x.get("vod_id") and x.get("vod_name") for x in obj["list"]))
    return rec
with ThreadPoolExecutor(max_workers=2) as pool:results=list(pool.map(check,checks))
p.save("standard_port_fallback_probes.json",results)
for r in results:print(r)
