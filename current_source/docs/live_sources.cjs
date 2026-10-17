// Optional live validation against public upstreams; never uses the normal data directory.
const fs=require('node:fs'),os=require('node:os'),path=require('node:path'),{spawn}=require('node:child_process');
const delay=ms=>new Promise(r=>setTimeout(r,ms));
const root=path.resolve(__dirname,'..'),out=path.join(__dirname,'validation_v1.1.0');
const packaged=path.resolve(root,'../YingXia_v1.1.0_Windows_x64.exe');
const executable=process.env.YINGXIA_EXE||(fs.existsSync(packaged)?packaged:path.resolve(root,'../releases/YingXia_v1.1.0_Windows_x64.exe'));
const report={date:new Date().toISOString(),executable:path.basename(executable),scope:'Live upstream update, catalogue search, episode metadata and limited playback attempts',details:[],playback:[]};
report.sha256=require('node:crypto').createHash('sha256').update(fs.readFileSync(executable)).digest('hex');
let browser,proc,api,dir;
async function main(){
 dir=fs.mkdtempSync(path.join(os.tmpdir(),'yingxia-live-'));
 proc=spawn(executable,['--no-browser','--data-dir',dir],{windowsHide:true,stdio:'ignore'});
 for(let i=0;i<100&&!fs.existsSync(path.join(dir,'session.json'));i++)await delay(100);
 const {origin}=JSON.parse(fs.readFileSync(path.join(dir,'session.json')));
 const home=await fetch(origin),cookie=home.headers.get('set-cookie').split(';')[0];await home.text();
 const boot=await fetch(origin+'/bootstrap.js',{headers:{Cookie:cookie}}),csrf=JSON.parse((await boot.text()).split('=')[1].replace(/;$/,'')).csrf;
 api=async(p,d)=>{const r=await fetch(origin+'/api/'+p,{method:d?'POST':'GET',headers:{Cookie:cookie,Origin:origin,'X-YingXia-CSRF':csrf,'Content-Type':'application/json'},...(d?{body:JSON.stringify(d)}:{})});const j=await r.json();if(!j.ok)throw Error(j.message);return j.data;};
 await api('settings',{network_consent:true,auto_update_sources:false});
 console.log('Checking actual upstream repositories...');
 report.updates=await api('sources/update',{});console.log(JSON.stringify(report.updates.upstreams,null,2));
 const start=Date.now(),result=process.env.YINGXIA_LIVE_REUSE_SEARCH==='1'?(()=>{const r=JSON.parse(fs.readFileSync(path.join(out,'live_sources.json'))).search;return {items:r.items.map(f=>({...f,provider_kind:f.id.startsWith('rule:')?'kazumi':'legacy'})),source_results:r.sources,source_count:r.source_count};})():await api('search',{query:'葬送的芙莉莲',mode:'all'});
 report.search={query:'葬送的芙莉莲',elapsed_ms:Date.now()-start,source_count:result.source_count,sources:result.source_results,items:result.items.map(f=>({id:f.id,title:f.title,source:f.source}))};
 if(process.env.YINGXIA_LIVE_REUSE_SEARCH==='1'){report.search.reused_from_prior_run=true;report.search.elapsed_ms=null;}
 console.log(JSON.stringify(result.source_results,null,2));
 const candidates=result.items.filter(f=>f.provider_kind==='kazumi');
 for(const f of candidates.slice(0,5)){try{const d=await api('detail?id='+encodeURIComponent(f.id));report.details.push({source:f.source,title:d.title,groups:d.groups.length,episodes:d.groups.reduce((n,g)=>n+g.episodes.length,0)});}catch(e){report.details.push({source:f.source,error:e.message});}}
 if(process.env.PLAYWRIGHT_MODULE){
  const {chromium}=require(process.env.PLAYWRIGHT_MODULE);
  browser=await chromium.launch({headless:true,executablePath:process.env.CHROMIUM_PATH||'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'});
  const context=await browser.newContext({viewport:{width:1340,height:920},locale:'zh-CN'});
  const page=await context.newPage();
  await page.route('**/*',r=>new URL(r.request().url()).origin===origin?r.continue():r.fulfill({contentType:'text/html',body:'<title>Page fallback</title>'}));
  await page.goto(origin);await page.waitForFunction(()=>document.querySelector('#player-source').options.length>0);
  const names=(process.env.YINGXIA_LIVE_PLAYBACK_SOURCES||'7sefun,AGE').split(',');
  for(const f of names.map(n=>result.items.find(f=>f.source===n)).filter(Boolean)){
   const observation={source:f.source,title:f.title,decoded:false};
   try{
    await api('favorite',{film_id:f.id});await page.locator('[data-view=library]').click();
    await page.locator('#favorites .film-card').filter({hasText:f.source}).first().click();
    await page.waitForFunction(()=>document.querySelectorAll('#episodes button').length>0);
    const resolve=page.waitForResponse(r=>r.url().endsWith('/api/resolve'),{timeout:60000});
    await page.locator('#play').click();const resolved=await (await resolve).json();
    observation.resolve_kind=resolved.data?.kind;observation.resolve_error=resolved.message;
    if(resolved.data?.kind==='media'){
     await page.waitForFunction(()=>{const v=document.querySelector('#stage video');return v?.currentTime>2&&v?.getVideoPlaybackQuality().totalVideoFrames>0;},null,{timeout:35000});
     Object.assign(observation,await page.locator('#stage video').evaluate(v=>({time:v.currentTime,frames:v.getVideoPlaybackQuality().totalVideoFrames,width:v.videoWidth,height:v.videoHeight,decoded:true})));
     await page.screenshot({path:path.join(out,'live_player.png'),fullPage:true});
    }
    await page.locator('#stop').click();
   }catch(e){observation.error=String(e.message).split('\n')[0];await page.locator('#stop').click().catch(()=>{});}
   report.playback.push(observation);console.log(JSON.stringify(observation));
   if(observation.decoded)break;
  }
 }
}
main().catch(e=>{report.failure=String(e.stack||e);console.error(report.failure);process.exitCode=1;}).finally(async()=>{
 if(browser)await browser.close();
 if(api)try{await api('quit',{});}catch{}
 if(proc){await delay(500);if(proc.exitCode===null)proc.kill();}
 fs.mkdirSync(out,{recursive:true});fs.writeFileSync(path.join(out,'live_sources.json'),JSON.stringify(report,null,2));
 if(dir&&path.dirname(dir)===os.tmpdir()&&path.basename(dir).startsWith('yingxia-live-'))fs.rmSync(dir,{recursive:true,force:true});
});
