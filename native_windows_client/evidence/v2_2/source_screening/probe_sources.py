"""Bounded public HTTP probes; remote configuration is parsed as data only."""
from pathlib import Path
from concurrent.futures import ThreadPoolExecutor, as_completed
from urllib.parse import urlsplit, urlunsplit, parse_qsl, urlencode
from datetime import datetime, timezone
import hashlib, ipaddress, json, re, socket, time
import urllib.request, urllib.error

HERE=Path(__file__).resolve().parent
ROOT=HERE.parents[3]
EXTRACT=json.loads((HERE/"workbook_extracted.json").read_text(encoding="utf-8"))
BASE=json.loads((ROOT/"native_core/internal/engine/providers.json").read_text(encoding="utf-8-sig"))
(HERE/"http").mkdir(exist_ok=True)

def rows(name):
    s=next(x for x in EXTRACT["sheets"] if x["name"]==name)
    for r in s["rows"]:
        if r["row"]<6: continue
        d={re.sub(r"[0-9]+$","",c["cell"]):c["value"] for c in r["cells"]}
        yield {"record_id":d.get("A"),"name":d.get("B"),"url":d.get("C"),"type":d.get("D"),"note":d.get("E"),"original":d.get("F"),"evidence_url":d.get("H"),"sheet":name,"row":r["row"]}

def norm(url):
    p=urlsplit(url)
    q=[(k,v) for k,v in parse_qsl(p.query,keep_blank_values=True) if k not in ("ac","pg","wd","ids")]
    return urlunsplit((p.scheme.lower(),p.netloc.lower(),p.path.rstrip("/"),urlencode(q),""))

def query(url,**params):
    p=urlsplit(url); q=dict(parse_qsl(p.query,keep_blank_values=True));q.update(params)
    return urlunsplit((p.scheme,p.netloc,p.path,urlencode(q),""))

def public(url):
    p=urlsplit(url)
    if p.scheme not in ("http","https") or not p.hostname or p.username or p.password: raise ValueError("unsupported/public URL required")
    for a in socket.getaddrinfo(p.hostname,p.port or (443 if p.scheme=="https" else 80),type=socket.SOCK_STREAM):
        addr=ipaddress.ip_address(a[4][0])
        if not addr.is_global: raise ValueError("non-public address rejected")

def fetch(url,key,cap=2097152):
    started=time.perf_counter(); rec={"url":url,"checked_at":datetime.now(timezone.utc).isoformat(),"redirects":[]}
    try:
        target=url
        for _ in range(5):
            public(target)
            p=urlsplit(target)
            from urllib.parse import quote
            wire=urlunsplit((p.scheme,p.netloc.encode("idna").decode("ascii"),quote(p.path,safe="/%:@"),quote(p.query,safe="=&%?:/@+"),""))
            class NoRedirect(urllib.request.HTTPRedirectHandler):
                def redirect_request(self,*args,**kwargs):return None
            request=urllib.request.Request(wire,headers={"User-Agent":"Mozilla/5.0 YingXiaSourceReview/2.2","Accept":"application/json,text/plain,text/html;q=0.8,*/*;q=0.1"})
            try:r=urllib.request.build_opener(NoRedirect()).open(request,timeout=6)
            except urllib.error.HTTPError as e:r=e
            if r.status in (301,302,303,307,308):
                from urllib.parse import urljoin
                nxt=urljoin(target,r.headers["Location"]);rec["redirects"].append({"from":target,"to":nxt,"status":r.status}); r.close();target=nxt;continue
            chunks=[];size=0
            while True:
                c=r.read(16384)
                if not c:break
                size+=len(c)
                if size>cap: raise ValueError("body exceeds bounded probe limit")
                chunks.append(c)
            body=b"".join(chunks);r.close()
            rec.update(status=r.status,final_url=target,content_type=r.headers.get("Content-Type",""),bytes=len(body),sha256=hashlib.sha256(body).hexdigest())
            # No response cookies/headers or media segments are retained.
            text=body.decode("utf-8-sig",errors="replace")
            fn="http/"+re.sub(r"[^a-zA-Z0-9_.-]","_",key)+".txt"
            (HERE/fn).write_text(text,encoding="utf-8")
            rec["body_file"]=fn
            try: obj=json.loads(text);rec["json"]=True
            except Exception: obj=None;rec["json"]=False
            rec["elapsed_ms"]=round((time.perf_counter()-started)*1000)
            return rec,obj,text
        raise ValueError("redirect limit")
    except Exception as e:
        rec.update(error=f"{type(e).__name__}: {e}",elapsed_ms=round((time.perf_counter()-started)*1000))
        return rec,None,""

def cms_probe(item):
    item=dict(item)
    if "/at/xml" in item["url"]:
        item.update(decision="duplicate_protocol",reason="XML是同一目录的协议入口，当前适配器仅JSON；对应JSON单独实测")
        return item
    rec,obj,_=fetch(query(item["url"],ac="detail",pg="1"),item["record_id"]+"_detail")
    item["probe"]=rec
    entries=obj.get("list") if isinstance(obj,dict) else None
    good=[x for x in entries or [] if isinstance(x,dict) and str(x.get("vod_id","")).strip() and str(x.get("vod_name","")).strip()]
    if rec.get("status")==200 and good:
        first=good[0]; item.update(decision="accepted_json",reason="MacCMS detail/list可解析且包含作品编号/名称",item_count=len(good),sample_id=str(first["vod_id"]),sample_name=first["vod_name"],has_play_fields=any(x.get("vod_play_url") for x in good))
    else:
        item.update(decision="not_usable_now",reason=rec.get("error") or f"HTTP {rec.get('status')}；未返回受支持的非空MacCMS JSON list")
    print(item["record_id"],item["decision"],rec.get("status",rec.get("error","")),flush=True)
    return item

def sub_probe(item):
    rec,obj,text=fetch(item["url"],item["record_id"]+"_subscription")
    item=dict(item);item["probe"]=rec; found=[]
    if isinstance(obj,dict):
        for site in obj.get("sites",[]) if isinstance(obj.get("sites"),list) else []:
            if not isinstance(site,dict):continue
            u=site.get("api",""); name=site.get("name",site.get("key",""))
            if site.get("type") in (0,1,"0","1") and isinstance(u,str) and u.startswith(("http://","https://")):
                if re.search(r"成人|情色|情色|福利|福利视频|AV专|AV片|色站|涩涩|🔞|18禁|18禁|18\+|萝莉",name,re.I):continue
                found.append({"name":name,"url":u,"origin_subscription":item["url"],"origin_subscription_record":item["record_id"],"evidence_url":item["evidence_url"]})
    # Extract only explicit CMS endpoint strings from non-JSON config data. Never interpret scripts.
    if not found and rec.get("status")==200:
        for u in re.findall(r'https?://[^\s\"\'<>]+/(?:api\.php/provide/vod[^\s\"\'<>]*|provide/vod[^\s\"\'<>]*)',text):
            found.append({"name":"订阅目录 "+urlsplit(u).hostname,"url":u,"origin_subscription":item["url"],"origin_subscription_record":item["record_id"],"evidence_url":item["evidence_url"]})
    item["extracted_cms"]=found
    item["decision"]="parsed_cms_candidates" if found else "no_compatible_cms"
    print(item["record_id"],item["decision"],len(found),rec.get("status",rec.get("error","")),flush=True)
    return item

def parallel(fn,items,workers=20):
    result=[]
    with ThreadPoolExecutor(max_workers=workers) as pool:
        jobs={pool.submit(fn,x):x for x in items}
        for future in as_completed(jobs):
            try:result.append(future.result())
            except Exception as e:result.append({**jobs[future],"decision":"probe_exception","error":str(e)})
    return result

def save(name,value):(HERE/name).write_text(json.dumps(value,ensure_ascii=False,indent=2),encoding="utf-8")

def main():
    cms=list(rows("01_影视采集API"));subs=list(rows("06_订阅入口"))
    save("cms_candidates_raw.json",cms)
    with ThreadPoolExecutor(max_workers=2) as phase:
        a=phase.submit(parallel,cms_probe,cms);b=phase.submit(parallel,sub_probe,subs)
        cms_results=a.result();sub_results=b.result()
    save("cms_initial_probes.json",sorted(cms_results,key=lambda x:x["record_id"]))
    save("subscription_probes.json",sorted(sub_results,key=lambda x:x["record_id"]))
    known={norm(x["url"]) for x in cms}
    extra=[]
    for sub in sub_results:
        for item in sub.get("extracted_cms",[]):
            try:key=norm(item["url"])
            except Exception:continue
            if key in known:continue
            known.add(key);extra.append({**item,"record_id":f"E{len(extra)+1:03d}","sheet":"subscription_expansion"})
    save("expanded_candidates_raw.json",extra)
    print("EXPANDED_CANDIDATES",len(extra),flush=True)
    extra_results=parallel(cms_probe,extra)
    save("expanded_probes.json",extra_results)
    accepted=[];baseline_by_url={norm(p["url"]):p for p in BASE["providers"] if p.get("url")}
    aliases=[];unique={}
    for item in sorted(cms_results+extra_results,key=lambda x:x["record_id"]):
        if item["decision"]!="accepted_json":continue
        canonical=norm(item["probe"].get("final_url",item["url"]))
        # Strip probe query action; preserve custom endpoint paths and other stable parameters.
        if canonical in baseline_by_url:
            aliases.append({"record_id":item["record_id"],"existing_id":baseline_by_url[canonical]["id"],"reason":"already_present","url":item["url"]});continue
        if canonical in unique:
            aliases.append({"record_id":item["record_id"],"duplicate_of":unique[canonical],"reason":"same_final_endpoint","url":item["url"]});continue
        unique[canonical]=item["record_id"]
        accepted.append({"id":"sheet_"+item["record_id"].lower(),"name":item["name"],"kind":"cms","url":canonical,"origin":"reviewed_20261015","enabled":True})
    out={"schema":1,"revision":"20261015","providers":accepted}
    save("accepted_providers.json",out);save("aliases.json",aliases)
    print("ACCEPTED",len(accepted),"BASELINE_OR_DUPLICATE",len(aliases),flush=True)

if __name__=="__main__":main()
