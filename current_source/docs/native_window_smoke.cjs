// Tests the shipped Win32/WebView2 host, not a separate browser approximation.
// Debug CDP is explicitly opted into for this isolated development run only.
// Catalogue/media fixtures are intercepted in DevTools; no third-party film is fetched.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const net = require('node:net');
const crypto = require('node:crypto');
const {spawn, execFile} = require('node:child_process');
const {promisify} = require('node:util');
const {chromium} = require(process.env.PLAYWRIGHT_MODULE || 'playwright');
const execFileAsync = promisify(execFile);
const root = path.resolve(__dirname, '..');
const productionExecutable = process.env.YINGXIA_EXE || path.resolve(root, '..', 'releases', 'YingXia_v1.3.0_Windows_x64.exe');
const nativeHarness = !!process.env.YINGXIA_NATIVE_TESTSERVER;
const executable = nativeHarness ? process.env.YINGXIA_NATIVE_TESTSERVER : productionExecutable;
const out = path.join(__dirname, 'validation_v1.3.0');
const outputPrefix = nativeHarness ? 'native_harness' : 'native';
const outputReport = nativeHarness ? 'native_window_harness_tests.json' : 'native_window_tests.json';
const temp = fs.mkdtempSync(path.join(os.tmpdir(), 'yingxia-native-window-'));
const dataDir = path.join(temp, '中文 独立窗口');
const className = 'YingXiaWindow_' + crypto.createHash('sha256').update(path.resolve(dataDir).toLowerCase()).digest('hex').slice(0,16);
const report = {environment:`${process.platform}/${process.arch}`, executable:path.basename(executable),
  sha256:crypto.createHash('sha256').update(fs.readFileSync(executable)).digest('hex'),
  production_executable:path.basename(productionExecutable),
  production_sha256:fs.existsSync(productionExecutable) ? crypto.createHash('sha256').update(fs.readFileSync(productionExecutable)).digest('hex') : null,
  production_gui_tested:!nativeHarness,
  test_artifact:nativeHarness ? 'explicit Go test harness calling unmodified production run(); not the shipping EXE' : 'shipping EXE',
  startup:`${nativeHarness ? 'explicit native Go test harness' : 'release EXE'}; owned Win32 window + embedded WebView2; opt-in loopback CDP; isolated data directory`,
  fixtures:'synthetic catalogue and WebM clip intercepted in DevTools', real_third_party_playback_tested:false,
  checks:[], observations:[], page_errors:[], intercepted_external_requests:[]};
const passed = name => {report.checks.push({name,passed:true}); console.log('PASS:',name);};
const delay = ms => new Promise(resolve => setTimeout(resolve, ms));
let proc, browser, page, context, session, cookie, csrf, port, resolveRequests = 0;
async function win(action = 'Snapshot',capturePath = null) {
  assert.ok(['Snapshot','Minimize','Restore','Close','Processes','CaptureFrame'].includes(action));
  assert.match(className,/^YingXiaWindow_[0-9a-f]{16}$/);assert.ok(Number.isInteger(proc.pid));
  // Run the checked-in helper as a command without changing the machine's
  // script execution policy. All inserted values are enum/integer/hash only.
  if(capturePath)assert.equal(path.resolve(capturePath),path.join(out,outputPrefix+'_window.png'));
  const outputArgument=capturePath ? ` -OutputPath '${capturePath.replace(/'/g,"''")}'` : '';
  const script=`& {\n${fs.readFileSync(path.join(__dirname,'native_window_test_helpers.ps1'),'utf8')}\n} -Action '${action}' -OwnerPid ${proc.pid} -ClassName '${className}'${outputArgument}`;
  const encoded=Buffer.from(script,'utf16le').toString('base64');
  const {stdout} = await execFileAsync('powershell.exe', ['-NoProfile','-NonInteractive','-EncodedCommand',encoded], {windowsHide:true,timeout:20000,maxBuffer:1024*1024,encoding:'utf8'});
  return JSON.parse(stdout.replace(/^\uFEFF/,''));
}
async function windowState() {
  const items = (await win()).windows;
  assert.equal(items.length,1,'exactly one native main window must belong to EXE PID');
  return items[0];
}
async function eventually(check, description, timeout = 15000) {
  const until = Date.now()+timeout; let error;
  while(Date.now()<until) {try {const value = await check(); if(value) return value;} catch(e) {error=e;} await delay(120);}
  throw new Error(`${description}${error ? ': '+error.message : ''}`);
}
async function reservePort() {
  const server=net.createServer(); await new Promise((resolve,reject)=>{server.once('error',reject);server.listen(0,'127.0.0.1',resolve);});
  const result=server.address().port; await new Promise(resolve=>server.close(resolve));return result;
}
function start(debug = true) {
  const env={...process.env};
  if(debug)env.YINGXIA_WEBVIEW_DEBUG_PORT=String(port);else delete env.YINGXIA_WEBVIEW_DEBUG_PORT;
  if(nativeHarness)env.YINGXIA_NATIVE_TEST_DIR=dataDir;
  const args=nativeHarness ? ['-test.run=^TestNativeWindowHarness$','-test.v'] : ['--data-dir',dataDir];
  const child=spawn(executable,args,{env,windowsHide:nativeHarness,stdio:'ignore'});
  child.on('error',error=>{report.process_error=String(error);});return child;
}
async function waitExit(child) {await eventually(()=>child.exitCode!==null,'process did not exit',15000);assert.equal(child.exitCode,0);}
async function readSession() {
  session=await eventually(()=>{try{return JSON.parse(fs.readFileSync(path.join(dataDir,'session.json'),'utf8'));}catch{return null;}},'session did not appear');
  assert.equal(session.pid,proc.pid);assert.equal(session.version,'1.3.0-exe');
  const home=await eventually(async()=>{try{return await fetch(session.origin);}catch{return null;}},'local HTTP service did not become ready');
  cookie=home.headers.get('set-cookie').split(';')[0];
  const boot=await fetch(session.origin+'/bootstrap.js',{headers:{Cookie:cookie}});
  csrf=JSON.parse((await boot.text()).split('=',2)[1].replace(/;$/,'')).csrf;
}
async function api(endpoint,data) {
  const response=await fetch(`${session.origin}/api/${endpoint}`,{method:data===undefined?'GET':'POST',headers:{Cookie:cookie,Origin:session.origin,'Content-Type':'application/json','X-YingXia-CSRF':csrf},...(data===undefined?{}:{body:JSON.stringify(data)})});
  assert.equal(response.status,200);const result=await response.json();assert.equal(result.ok,true,JSON.stringify(result));return result.data;
}
async function attach() {
  await eventually(async()=>{try {return (await fetch(`http://127.0.0.1:${port}/json/version`)).ok;}catch{return false;}},'WebView2 CDP did not start',40000);
  browser=await chromium.connectOverCDP(`http://127.0.0.1:${port}`);
  context=browser.contexts()[0];assert.ok(context,'embedded WebView2 browser context missing');
  page=await eventually(()=>context.pages().find(item=>item.url().startsWith(session.origin)),'embedded page did not navigate');
  page.setDefaultTimeout(15000);page.on('pageerror',error=>report.page_errors.push(String(error)));
  await page.waitForFunction(()=>document.querySelector('#player-source')?.options.length>0);
  report.webview_version=await browser.version();
}
async function installFixtures() {
  const clip=fs.readFileSync(path.join(root,'testdata','demo','clip1.webm'));
  const film={id:'native-fixture',title:'独立窗口合成测试',year:'2026',kind:'测试',area:'本地',remarks:'',summary:'',actors:'',director:'',source:'测试来源',provider_id:'wsyzy',groups:[{key:'g0',name:'测试线路',episodes:[{key:'e0',name:'第01集',ordinal:1},{key:'e1',name:'第02集',ordinal:2}]}],demo:false};
  const search={id:'native-job',phase:'done',percent:100,source_total:1,source_completed:1,source_count:1,successful_sources:1,available_count:1,checked_count:1,check_total:1,items:[{...film,availability:{state:'media_ok',label:'可播放',group_key:'g0',episode_key:'e0'}}],source_results:[{name:'合成来源',ok:true,count:1}]};
  await page.route('**/*',async route=>{
    const u=new URL(route.request().url());
    if(u.origin!==session.origin) {report.intercepted_external_requests.push(u.toString());return route.fulfill({status:200,contentType:'text/html',body:'<!doctype html><title>External fixture</title>'});}
    const json=data=>route.fulfill({status:200,contentType:'application/json',body:JSON.stringify({ok:true,data})});
    if(u.pathname==='/api/search/start'||u.pathname==='/api/search/status')return json(search);
    if(u.pathname==='/api/detail'&&u.searchParams.get('id')===film.id)return json(film);
    if(u.pathname==='/api/bindings'&&u.searchParams.get('id')===film.id)return json([]);
    if(u.pathname==='/api/resolve') {resolveRequests++;return json({kind:'media',url:session.origin+'/__native_fixture__/clip.webm',external_url:'',format:'webm',source_label:'合成测试视频',title:film.title,episode:'第01集',history_id:'',demo:true});}
    if(u.pathname==='/__native_fixture__/clip.webm') {
      const range=route.request().headers().range;const match=range?.match(/^bytes=(\d+)-(\d*)$/);
      if(match) {const begin=Number(match[1]),end=Math.min(match[2]?Number(match[2]):clip.length-1,clip.length-1);return route.fulfill({status:206,headers:{'Content-Type':'video/webm','Accept-Ranges':'bytes','Content-Range':`bytes ${begin}-${end}/${clip.length}`},body:clip.subarray(begin,end+1)});}
      return route.fulfill({status:200,contentType:'video/webm',headers:{'Accept-Ranges':'bytes'},body:clip});
    }
    return route.continue();
  });
}
async function videoState() {return page.locator('#stage video').evaluate(v=>({time:v.currentTime,paused:v.paused,ended:v.ended,frames:v.getVideoPlaybackQuality().totalVideoFrames,width:v.videoWidth,marker:v.dataset.nativeTestMarker,hidden:document.hidden}));}
async function main() {
  assert.equal(process.platform,'win32','native host test requires Windows');fs.mkdirSync(out,{recursive:true});
  port=await reservePort();proc=start();await readSession();await attach();
  if(nativeHarness){await win('Restore');report.harness_visibility_adjustment='The console test artifact uses SW_HIDE to suppress its console; explicitly restored its owned HWND before UI checks. Production GUI startup visibility remains unverified.';}
  const native=await windowState();assert.equal(native.title,'映匣 · YingXia');assert.equal(native.visible,true);assert.equal(native.pid,proc.pid);
  report.observations.push({native_window:native});
  passed('Main HWND is owned by the tested executable with the app title and unique Win32 class');
  const processes=await win('Processes');report.observations.push({processes:{descendants:processes.descendants,browsers:processes.browsers.map(({ProcessId,Name})=>({ProcessId,Name}))}});
  assert.ok(processes.descendants.some(item=>item.Name.toLowerCase()==='msedgewebview2.exe'),'embedded runtime child processes are required');
  assert.equal(processes.browsers.some(item=>(item.CommandLine||'').includes('--app=')&&(item.CommandLine||'').toLowerCase().includes(dataDir.toLowerCase())),false);
  passed('Rendering runs in embedded msedgewebview2 children without a Chrome/Edge --app window');
  assert.equal(await page.locator('#import-open, #import-dialog').count(),0);
  for(const view of ['workspace','library','settings','search']) {await page.locator(`[data-view=${view}]`).click();assert.equal(await page.locator(`#view-${view}`).isVisible(),true);}
  await page.screenshot({path:path.join(out,outputPrefix+'_home.png'),fullPage:true});
  await win('CaptureFrame',path.join(out,outputPrefix+'_window.png'));
  report.owned_window_screenshot=outputPrefix+'_window.png';
  assert.equal((await api('state')).source_registry.providers.length,17);
  passed('Existing minimal UI and all four navigation pages load inside the native host');
  await api('settings',{network_consent:true,auto_update_sources:false,auto_next:false});await page.reload();await page.waitForFunction(()=>document.querySelector('#player-source').options.length>0);
  await installFixtures();await page.locator('#query').fill('独立窗口合成测试');await page.locator('#search-submit').click();
  await page.waitForFunction(()=>document.querySelector('#search-progress-bar').value===100);await page.locator('#results .film-card').click();
  await page.waitForFunction(()=>document.querySelector('#film-title').textContent==='独立窗口合成测试');await page.locator('#play').click();
  await page.waitForFunction(()=>document.querySelector('#stage video')?.currentTime>0.6);const playing=await videoState();assert.ok(playing.frames>0&&playing.width>0);report.observations.push({decoded_video:playing});
  passed('Search selection and Play decode real synthetic WebM frames in embedded WebView2');
  await page.locator('#stage video').evaluate(v=>{v.loop=true;v.dataset.nativeTestMarker='original-player';});
  const resolvesBefore=resolveRequests;await win('Minimize');await page.waitForFunction(()=>document.querySelector('#stage video')?.paused===true);
  const minimized=await windowState();assert.equal(minimized.minimized,true);const paused=await videoState();await delay(750);const frozen=await videoState();assert.ok(Math.abs(paused.time-frozen.time)<0.08);assert.equal(frozen.paused,true);
  await win('Restore');await eventually(async()=>!(await windowState()).minimized,'window did not restore');await delay(250);assert.equal((await videoState()).paused,true);
  await page.locator('#play').click();await page.waitForFunction(time=>{const v=document.querySelector('#stage video');return v&&!v.paused&&v.currentTime>time+.12;},paused.time);
  assert.equal((await videoState()).marker,'original-player');assert.equal(resolveRequests,resolvesBefore);report.observations.push({minimize_pause:{paused,frozen,resumed:await videoState()}});
  passed('Actual native minimize freezes video; restore stays paused; manual Play resumes the same player');
  const beforeFull=await windowState();await page.locator('#fullscreen').click();await page.waitForFunction(()=>!!document.fullscreenElement);
  const full=await eventually(async()=>{const w=await windowState();return JSON.stringify(w.rect)===JSON.stringify(w.monitor)?w:null;},'fullscreen window did not cover the monitor');
  assert.equal(full.style&0x00c00000,0,'fullscreen must remove the window caption');
  await page.screenshot({path:path.join(out,outputPrefix+'_fullscreen.png'),fullPage:true});
  await page.keyboard.press('Escape');
  const syntheticEscapeExited=await page.waitForFunction(()=>!document.fullscreenElement,null,{timeout:1200}).then(()=>true,()=>false);
  // CDP keyboard events do not own the OS foreground window. If Chromium does
  // not handle the synthetic Escape, exercise its fullscreen API directly;
  // never inject a global key into whatever application the user is using.
  if(!syntheticEscapeExited)await page.evaluate(()=>document.exitFullscreen());
  await page.waitForFunction(()=>!document.fullscreenElement);
  const restored=await eventually(async()=>{const w=await windowState();return JSON.stringify(w.rect)===JSON.stringify(beforeFull.rect)?w:null;},'fullscreen exit did not restore previous bounds');
  assert.equal(restored.style,beforeFull.style);report.observations.push({fullscreen:{before:beforeFull,full,restored,synthetic_escape_exited:syntheticEscapeExited,exit_method:syntheticEscapeExited?'CDP synthetic Escape':'document.exitFullscreen()'}});
  report.native_keyboard_escape_tested=false;
  report.keyboard_test_limit='The native HWND is not guaranteed to own OS foreground focus. CDP Escape is observed separately; physical Escape is not verified.';
  passed('Fullscreen removes the caption and covers the real monitor; fullscreen exit restores bounds and window style');
  await win('Minimize');const second=start();await waitExit(second);const reactivated=await eventually(async()=>{const w=await windowState();return !w.minimized?w:null;},'second launch did not restore first window');
  assert.equal(reactivated.hwnd,native.hwnd);assert.equal(JSON.parse(fs.readFileSync(path.join(dataDir,'session.json'),'utf8')).pid,proc.pid);
  passed('Repeated launch restores the existing native HWND and retains the first backend PID');
  await page.evaluate(()=>{location.href='https://example.invalid/blocked-native-navigation';});await delay(700);assert.ok(page.url().startsWith(session.origin));assert.equal(await page.locator('#view-workspace').isVisible(),true);
  passed('Script-driven top-level external navigation cannot replace the app UI');
  const pagesBefore=context.pages().length;const browsersBefore=(await win('Processes')).browsers.map(item=>item.ProcessId).sort();
  // Runtime.evaluate exposes an explicit userGesture flag; Playwright's usual
  // evaluate helper may grant one, so force false for this host policy check.
  const cdp=await context.newCDPSession(page);
  await cdp.send('Runtime.evaluate',{expression:"window.open('https://example.invalid/nonuser-popup','_blank'); void 0",userGesture:false});
  await cdp.detach();await delay(700);
  assert.equal(context.pages().length,pagesBefore);assert.deepEqual((await win('Processes')).browsers.map(item=>item.ProcessId).sort(),browsersBefore);
  passed('A popup without a user gesture creates neither an embedded tab nor an external browser process');
  assert.deepEqual(report.page_errors,[]);await page.screenshot({path:path.join(out,outputPrefix+'_workspace.png'),fullPage:true});
  await win('Close');await waitExit(proc);assert.equal(fs.existsSync(path.join(dataDir,'session.json')),false);await eventually(async()=>!(await win()).windows.length,'closed HWND still exists');
  passed('Closing the native window exits the EXE and removes the backend session');
  browser=null;page=null;context=null;session=null;
  await eventually(async()=>{try{await fetch(`http://127.0.0.1:${port}/json/version`);return false;}catch{return true;}},'CDP listener remained after native close');
  proc=start(false);await readSession();
  if(nativeHarness){await eventually(async()=>!!(await windowState()),'ordinary harness window did not appear');await win('Restore');}
  await eventually(async()=>{const w=await windowState();return w.visible&&!w.minimized;},'ordinary launch did not create native window');await delay(500);
  let debuggingAvailable=false;try{debuggingAvailable=(await fetch(`http://127.0.0.1:${port}/json/version`)).ok;}catch{}
  assert.equal(debuggingAvailable,false);passed('Ordinary launch opens its native window without the explicitly opted-in CDP listener');
  await api('quit',{});await waitExit(proc);assert.equal(fs.existsSync(path.join(dataDir,'session.json')),false);
  assert.equal((await win()).windows.length,0);passed('API quit shuts down both the service and native window');
  browser=null;session=null;
}
main().catch(error=>{report.failure=String(error.stack||error);console.error(report.failure);process.exitCode=1;}).finally(async()=>{
  if(proc&&proc.exitCode===null){try{await win('Close');await waitExit(proc);}catch{proc.kill();await eventually(()=>proc.exitCode!==null,'test process termination timed out').catch(()=>{});}}
  if(path.dirname(temp)===os.tmpdir()&&path.basename(temp).startsWith('yingxia-native-window-')){
    try{fs.rmSync(temp,{recursive:true,force:true,maxRetries:40,retryDelay:100});}catch(error){report.retained_test_directory=temp;report.cleanup_error=String(error);}
  }
  report.check_count=report.checks.length;fs.mkdirSync(out,{recursive:true});fs.writeFileSync(path.join(out,outputReport),JSON.stringify(report,null,2));
});
