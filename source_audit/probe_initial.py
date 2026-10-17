import requests, concurrent.futures, json, time, hashlib, pathlib, datetime
ROOT=pathlib.Path('/mnt/data/video_validation')
URLS={
 'github_raw':'https://raw.githubusercontent.com/88lin/video_vip/main/video_vip.user.js',
 'cdn_script':'https://cdn.jsdmirror.com/gh/88lin/video_vip@main/video_vip.user.js',
 'wsy_search':'https://wsyzy.cc/index.php/ajax/suggest?mid=1&wd=%E4%B8%89%E4%BD%93',
 'wsy_api':'https://api.wsyzy.net/api.php/provide/vod/?ac=list&wd=%E4%B8%89%E4%BD%93',
 'wsy_player':'https://wsyzy.vip/m3u8/',
 'qilin':'https://free.maccms.xyz/',
 'txnq':'https://bfq.txnp.cn/player',
 'qige':'https://jx.202617.xyz/tv.php',
 'fongmi':'https://json.fongmi.cc/web',
 'bingdou':'https://bd.jx.cn/',
 'hls':'https://jx.hls.one/',
 'playerjy':'https://jx.playerjy.com/',
}
def run(k,u):
 t=time.monotonic(); info={'name':k,'url':u,'tested_utc':datetime.datetime.now(datetime.timezone.utc).isoformat()}
 try:
  r=requests.get(u, timeout=(8,15), headers={'User-Agent':'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/131.0.0.0 Safari/537.36'},allow_redirects=True)
  info.update(status=r.status_code,final_url=r.url,content_type=r.headers.get('Content-Type'),bytes=len(r.content),sha256=hashlib.sha256(r.content).hexdigest(),snippet=r.text[:200],headers={k:v for k,v in r.headers.items() if k.lower() in ['server','x-frame-options','content-security-policy','x-mitmproxy-blocked-reason','location','content-type']})
  (ROOT/'evidence'/f'{k}.body').write_bytes(r.content)
 except Exception as e: info.update(error=type(e).__name__+': '+str(e)[:500])
 info['seconds']=round(time.monotonic()-t,2)
 print(json.dumps(info,ensure_ascii=False),flush=True); return info
with concurrent.futures.ThreadPoolExecutor(max_workers=6) as ex: results=list(ex.map(lambda kv:run(*kv),URLS.items()))
(ROOT/'initial_results.json').write_text(json.dumps(results,ensure_ascii=False,indent=2),encoding='utf-8')
