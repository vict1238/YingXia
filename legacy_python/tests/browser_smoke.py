"""Optional UI integration test. Requires Playwright; NOT a production dependency.
Runs synthetic catalogue responses and local WebM; no real third-party playback claim.
"""
import json
import base64
import http.client
import os
from pathlib import Path
import tempfile
import threading
from playwright.sync_api import sync_playwright
from yingxia.service import Service
from yingxia.server import Server
from tests.fixtures import FakeNetwork

OUT=Path(__file__).resolve().parents[1]/'evidence'

def main():
 OUT.mkdir(exist_ok=True)
 report={'environment':'Linux + system Chromium; HTML/CSS/JS injected in memory because managed browser blocks URL navigation; no policies changed; API bridge to actual loopback HTTP server; not Windows','catalogue':'synthetic test fixtures',
         'media':'local synthetic WebM clips','real_source_playback_verified':False,'checks':[],'console_errors':[]}
 def passed(name):
  report['checks'].append({'name':name,'passed':True});print('PASS:',name,flush=True)
 with tempfile.TemporaryDirectory() as tmp:
  service=Service(Path(tmp),FakeNetwork());server=Server(service)
  thread=threading.Thread(target=server.serve_forever,daemon=True);thread.start()
  try:
   with sync_playwright() as p:
    kwargs={'headless':True,'args':['--no-sandbox']}
    if os.environ.get('CHROMIUM_PATH'):kwargs['executable_path']=os.environ['CHROMIUM_PATH']
    browser=p.chromium.launch(**kwargs)
    context=browser.new_context(viewport={'width':1340,'height':920},locale='zh-CN',device_scale_factor=1)
    page=context.new_page();page.set_default_timeout(10000);page.on('pageerror',lambda err:report['console_errors'].append(str(err)))
    # This harness does not navigate or bypass administrator policies. Browser networking
    # is unused. Production HTML/JS/CSS are rendered in memory; API calls go through the
    # separate HTTP client to the actual server already covered by HTTP security tests.
    def local_http(source,path,options):
     assert path.startswith('/api/')
     c=http.client.HTTPConnection(*server.server_address,timeout=10)
     h={'Cookie':server.cookie_name+'='+server.session,'Origin':server.origin}
     h.update(options.get('headers',{}))
     c.request(options.get('method','GET'),path,body=options.get('body','').encode('utf-8') if options.get('body') else None,headers=h)
     r=c.getresponse();data=r.read().decode();status=r.status;c.close()
     return {'status':status,'body':data}
    page.expose_binding('__yx_http',local_http)
    root=Path(__file__).resolve().parents[1]/'yingxia'/'web'
    html=root.joinpath('index.html').read_text().replace('<link rel="stylesheet" href="/app.css">','').replace('<script src="/bootstrap.js" defer></script><script src="/app.js" defer></script>','')
    clips={f'/demo/clip{i}.webm':base64.b64encode((root/'demo'/f'clip{i}.webm').read_bytes()).decode() for i in (1,2)}
    def load_ui():
     page.set_content(html)
     page.add_style_tag(content=root.joinpath('app.css').read_text())
     page.evaluate('''(payload)=>{
       window.YX_BOOT=payload.boot;
       window.__clips={};
       for (const [path,encoded] of Object.entries(payload.clips)) {
         const bytes=Uint8Array.from(atob(encoded),c=>c.charCodeAt(0));
         window.__clips[path]=URL.createObjectURL(new Blob([bytes],{type:'video/webm'}));
       }
       window.fetch=async(path,options={})=>{
         if(!path.startsWith('/api/'))throw Error('No browser network in test harness');
         const result=await window.__yx_http(path,options);
         const obj=JSON.parse(result.body);
         if(path==='/api/resolve' && obj.ok){
           if(obj.data.demo)obj.data.url=window.__clips[obj.data.url];
           else if(obj.data.kind==='page')obj.data.url='about:blank';
         }
         return new Response(JSON.stringify(obj),{status:result.status,headers:{'Content-Type':'application/json'}});
       };
     }''', {'boot':{'csrf':server.token,'origin':server.origin},'clips':clips})
     page.add_script_tag(content=root.joinpath('app.js').read_text())
     page.wait_for_function("document.querySelector('#player-source').options.length > 0")
    load_ui();page.locator('#demo-btn').wait_for()
    page.screenshot(path=str(OUT/'ui_home.png'),full_page=True);passed('Home loaded with offline-by-default configuration')
    assert not service.store.get('network_consent',False)
    page.locator('#demo-btn').click();page.wait_for_function("document.querySelectorAll('#results .film-card').length === 2")
    assert not service.network.calls;passed('Offline search returns explicitly labelled local demo only')
    page.screenshot(path=str(OUT/'ui_demo_results.png'),full_page=True)
    page.locator('#results .film-card').first.click();page.wait_for_function("document.querySelectorAll('#episodes button').length === 2")
    page.locator('#play').click();page.wait_for_function("document.querySelector('#stage video')?.currentTime > 1.2",timeout=15000)
    page.wait_for_function("document.querySelector('#playback-status').textContent.includes('视频帧')")
    quality=page.locator('#stage video').evaluate('(v)=>({time:v.currentTime,frames:v.getVideoPlaybackQuality().totalVideoFrames,width:v.videoWidth,height:v.videoHeight})')
    report['local_media_evidence']=quality;assert quality['frames']>0;passed('Local synthetic video decodes frames and time advances')
    page.screenshot(path=str(OUT/'ui_player.png'),full_page=True)
    page.locator('#next').click();page.wait_for_function("document.querySelector('#stage video')?.currentSrc === window.__clips['/demo/clip2.webm']")
    page.wait_for_function("document.querySelector('#stage video')?.currentTime > 0.4")
    assert '第02集' in page.locator('#selected-episode').inner_text();passed('Next-episode control selects and plays the second local clip')
    page.locator('#group-select').select_option('g1')
    page.wait_for_function("document.querySelector('#workspace-message').textContent.includes('不会自动跳')")
    assert page.locator('#play').is_disabled();assert page.locator('#stage video').count()==0;passed('Switch to source missing episode 2 does not silently play episode 1')
    page.locator('#episodes button').first.click();page.locator('#favorite-btn').click();page.wait_for_function("document.querySelector('#favorite-btn').textContent.includes('已收藏')")
    page.locator('[data-view=library]').click();page.wait_for_function("document.querySelectorAll('#favorites .film-card').length === 1")
    assert '浏览器报告时间推进' in page.locator('#history').inner_text();passed('Favorites and observed playback evidence persist in local SQLite')
    load_ui();page.locator('[data-view=library]').click();page.wait_for_function("document.querySelectorAll('#favorites .film-card').length === 1")
    passed('Saved favorites survive UI reinitialization; browser URL navigation not tested')
    page.locator('[data-view=search]').click();page.locator('#query').fill('Example Journey 第二季');page.locator('#search-form button').click()
    page.locator('#consent-dialog').wait_for(state='visible');assert not service.network.calls;passed('Online query blocked until consent')
    page.locator('#consent-accept').click();page.wait_for_function("document.querySelectorAll('#results .film-card').length === 2")
    assert '第二季' in page.locator('#results .film-card h3').first.inner_text();passed('Synthetic online catalogue search sorts full title and season')
    page.locator('#results .film-card').first.click();page.wait_for_function("document.querySelectorAll('#episodes button').length === 2")
    assert page.locator('#group-select option').count()==2;passed('All synthetic VOD source groups remain selectable')
    # A remote renderer fixture only: does not prove the actual external player is working.
    page.locator('#play').click();page.locator('#stage iframe').wait_for();page.locator('#stage iframe').evaluate("f=>f.dispatchEvent(new Event('load'))");page.wait_for_function("document.querySelector('#playback-status').textContent.includes('网页加载事件')")
    assert 'good' not in page.locator('#playback-dot').get_attribute('class');passed('A synthetic iframe load event does not mark media playback successful; remote embedding not tested')
    page.locator('#confirm-playing').click();page.wait_for_function("document.querySelector('#playback-status').textContent.includes('用户已确认')")
    passed('Manual confirmation remains labelled as user-reported evidence')
    page.locator('#stop').click()
    # Check that a slow resolve response cannot undo a user's Stop click.
    page.evaluate('''()=>{window.__originalFetch=window.fetch;window.fetch=async(path,opt)=>{const r=await window.__originalFetch(path,opt);if(path==='/api/resolve')await new Promise(ok=>setTimeout(ok,800));return r;};}''')
    page.locator('#play').click();page.locator('#stop').click();page.wait_for_timeout(1200)
    assert page.locator('#stage iframe').count()==0 and page.locator('#stage video').count()==0
    page.evaluate('()=>{window.fetch=window.__originalFetch;}')
    passed('A delayed playback response cannot restart the player after Stop')
    # Explicit platform import remains a single unverified page, not fabricated episodes.
    page.locator('#import-open').click();page.locator('#import-title').fill('Synthetic Platform Page');page.locator('#import-url').fill('https://v.qq.com/x/cover/test/ep.html?token=secret&vid=abc')
    page.locator('#import-form button[type=submit]').click();page.wait_for_function("document.querySelector('#film-title').textContent==='Synthetic Platform Page'")
    assert page.locator('#episodes button').count()==1
    assert 'secret' not in page.locator('#binding-url').input_value();passed('Manual official-page import strips private query parameters and does not invent episode list')
    page.locator('[data-view=settings]').click();page.locator('#diagnose').click();page.wait_for_function("document.querySelector('#diagnostics table') !== null")
    assert 'fixture HTTP only' in page.locator('#diagnostics').inner_text();passed('Diagnostic UI displays HTTP-only results without claiming real playback')
    page.screenshot(path=str(OUT/'ui_settings.png'),full_page=True)
    for width in (1340,1024,720,390):
     page.set_viewport_size({'width':width,'height':920});page.locator('[data-view=search]').click()
     assert page.evaluate('document.documentElement.scrollWidth <= innerWidth+1'),f'horizontal overflow at {width}'
    passed('Search UI has no horizontal page overflow at 1340, 1024, 720 and 390 px')
    assert not report['console_errors'],report['console_errors'];passed('No uncaught browser JavaScript errors')
    report['browser_version']=browser.version;browser.close()
  finally:
   server.shutdown();server.server_close();service.store.close()
 OUT.joinpath('browser_tests.json').write_text(json.dumps(report,ensure_ascii=False,indent=2),encoding='utf-8')
 print(json.dumps(report,ensure_ascii=False,indent=2))

if __name__=='__main__':main()
