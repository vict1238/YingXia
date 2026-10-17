// Regression for the owned Win32/WebView2 window and real persistent storage.
// Only an isolated temporary library is touched. No third-party content is used.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const net = require('node:net');
const crypto = require('node:crypto');
const {spawn} = require('node:child_process');
const {chromium} = require(process.env.PLAYWRIGHT_MODULE || 'playwright');
const root = path.resolve(__dirname, '..');
const executable = process.env.YINGXIA_EXE || path.resolve(root, '..', 'releases', 'YingXia_v1.3.1_Windows_x64.exe');
const out = process.env.YINGXIA_VALIDATION_DIR || path.join(__dirname, 'validation_v1.3.1');
const temp = fs.mkdtempSync(path.join(os.tmpdir(), 'yingxia-clear-history-'));
const dataDir = path.join(temp, '中文 清空记录');
const libraryPath = path.join(dataDir, 'library.json');
const delay = ms => new Promise(resolve => setTimeout(resolve, ms));
const fixture = {
  schema:1, settings:{network_consent:false,auto_update_sources:false,save_history:true,auto_next:false},
  favorites:[{id:'clear-test-favorite',title:'保留的收藏',year:'2026',kind:'测试',source:'本机合成数据'}],
  history:[1,2].map(n=>({id:'clear-history-'+n,film_id:'clear-test-'+n,title:'历史测试 '+n,episode:'第01集',source:'本机合成数据',mode:'media',position:0,evidence:'opened',created_at:'2026-10-14T00:00:00Z',updated_at:'2026-10-14T00:00:00Z'})),
  bindings:[{film_id:'clear-test-favorite',episode:'第01集',platform:'qq',url:'https://v.qq.com/x/cover/test/ep.html'}]
};
const report = {environment:`${process.platform}/${process.arch}`,executable,
  sha256:crypto.createHash('sha256').update(fs.readFileSync(executable)).digest('hex'),
  startup:'actual owned Win32/WebView2 window with opt-in loopback CDP and isolated temporary data',
  fixtures:'synthetic favorites, history and binding in library.json; no third-party film requests',
  checks:[],page_errors:[],browser_dialog_events:[],external_requests:[]};
const passed = name => {report.checks.push({name,passed:true});console.log('PASS:',name);};
let proc,browser,context,page,session,cookie,csrf,port,clearRequests=0;
async function eventually(check,description,timeout=30000) {
  const end=Date.now()+timeout;let error;
  while(Date.now()<end){try{const result=await check();if(result)return result;}catch(e){error=e;}await delay(100);}
  throw new Error(description+(error?': '+error.message:''));
}
async function reservePort() {
  const server=net.createServer();await new Promise((resolve,reject)=>{server.once('error',reject);server.listen(0,'127.0.0.1',resolve);});
  const value=server.address().port;await new Promise(resolve=>server.close(resolve));return value;
}
async function start() {
  port=await reservePort();proc=spawn(executable,['--data-dir',dataDir],{env:{...process.env,YINGXIA_WEBVIEW_DEBUG_PORT:String(port)},windowsHide:false,stdio:'ignore'});
  proc.on('error',e=>{report.process_error=String(e);});
  session=await eventually(()=>{try{const value=JSON.parse(fs.readFileSync(path.join(dataDir,'session.json'),'utf8'));return value.pid===proc.pid?value:null;}catch{return null;}},'session missing');
  const home=await fetch(session.origin);assert.equal(home.status,200);cookie=home.headers.get('set-cookie').split(';')[0];
  const boot=await fetch(session.origin+'/bootstrap.js',{headers:{Cookie:cookie}});csrf=JSON.parse((await boot.text()).split('=',2)[1].replace(/;$/,'')).csrf;
  await eventually(async()=>{try{return (await fetch(`http://127.0.0.1:${port}/json/version`)).ok;}catch{return false;}},'WebView2 debugging listener missing',40000);
  browser=await chromium.connectOverCDP(`http://127.0.0.1:${port}`);context=browser.contexts()[0];
  page=await eventually(()=>context.pages().find(p=>p.url().startsWith(session.origin)),'embedded app page missing');
  page.setDefaultTimeout(15000);page.on('pageerror',e=>report.page_errors.push(String(e)));
  page.on('dialog',async dialog=>{report.browser_dialog_events.push(dialog.type());await dialog.dismiss().catch(()=>{});});
  await page.route('**/*',route=>{
    const url=new URL(route.request().url());
    if(url.origin!==session.origin){report.external_requests.push(url.toString());return route.abort();}
    return route.continue();
  });
  await page.waitForFunction(()=>document.querySelector('#player-source')?.options.length>0);
  report.version=session.version;report.webview_version=await browser.version();
}
async function api(endpoint,data) {
  const response=await fetch(`${session.origin}/api/${endpoint}`,{method:data===undefined?'GET':'POST',headers:{Cookie:cookie,Origin:session.origin,'Content-Type':'application/json','X-YingXia-CSRF':csrf},...(data===undefined?{}:{body:JSON.stringify(data)})});
  assert.equal(response.status,200);const body=await response.json();assert.equal(body.ok,true,JSON.stringify(body));return body.data;
}
async function stop() {
  await api('quit',{});await eventually(()=>proc.exitCode!==null,'application did not quit',15000);assert.equal(proc.exitCode,0);
  assert.equal(fs.existsSync(path.join(dataDir,'session.json')),false);browser=null;page=null;context=null;session=null;
}
async function openLibrary() {
  await page.locator('[data-view=library]').click();await page.waitForFunction(()=>document.querySelector('#favorites-count').textContent==='1 个收藏');
}
async function openClear() {
  await page.locator('#clear-history').click();await page.locator('#clear-history-dialog').waitFor({state:'visible'});
  assert.equal(await page.locator('#clear-history-cancel').evaluate(e=>e===document.activeElement),true);
}
async function main() {
  assert.equal(process.platform,'win32');fs.mkdirSync(out,{recursive:true});fs.mkdirSync(dataDir,{recursive:true});fs.writeFileSync(libraryPath,JSON.stringify(fixture,null,2));
  await start();await openLibrary();await page.waitForFunction(()=>document.querySelectorAll('#history tbody tr').length===2);
  assert.equal(await page.locator('#clear-history-dialog').count(),1);
  assert.doesNotMatch(await (await fetch(session.origin+'/app.js',{headers:{Cookie:cookie}})).text(),/\bconfirm\s*\(/);
  passed('Shipping embedded UI contains an app-owned dialog and no browser confirm call');
  await openClear();await page.screenshot({path:path.join(out,'clear_history_dialog.png')});await page.locator('#clear-history-cancel').click();
  assert.equal(await page.locator('#clear-history-dialog').isVisible(),false);assert.equal((await api('library')).history.length,2);
  assert.equal(JSON.parse(fs.readFileSync(libraryPath,'utf8')).history.length,2);
  passed('In-app confirmation opens; Cancel preserves both records on disk');
  await openClear();await page.keyboard.press('Escape');await page.locator('#clear-history-dialog').waitFor({state:'hidden'});assert.equal((await api('library')).history.length,2);
  passed('Escape cancels the in-app dialog without clearing data');
  await page.route('**/api/clear-history',route=>route.fulfill({status:500,contentType:'application/json',body:JSON.stringify({ok:false,error:'storage',message:'测试写入失败'})}));
  await openClear();await page.locator('#clear-history-confirm').click();await page.waitForFunction(()=>document.querySelector('#clear-history-message').textContent==='测试写入失败');
  assert.equal(await page.locator('#clear-history-dialog').isVisible(),true);assert.equal(await page.locator('#clear-history-confirm').isEnabled(),true);assert.equal(await page.locator('#history tbody tr').count(),2);
  assert.equal((await api('library')).history.length,2);assert.equal(JSON.parse(fs.readFileSync(libraryPath,'utf8')).history.length,2);
  await page.locator('#clear-history-cancel').click();await page.unroute('**/api/clear-history');
  passed('Failed clear request shows an error and retains records; controls allow retry');
  // Hold an older GET until after the clear and its fresh library GET complete.
  let releaseOld,oldCapturedResolve;const oldCaptured=new Promise(resolve=>oldCapturedResolve=resolve);const releasePromise=new Promise(resolve=>releaseOld=resolve);let captured=false;
  await page.route('**/api/library',async route=>{
    if(captured)return route.continue();captured=true;const response=await route.fetch();oldCapturedResolve();await releasePromise;return route.fulfill({response});
  });
  await page.locator('[data-view=search]').click();await page.locator('[data-view=library]').click();await oldCaptured;
  let clearStartedResolve;const clearStarted=new Promise(resolve=>clearStartedResolve=resolve);
  await page.route('**/api/clear-history',async route=>{clearRequests++;const response=await route.fetch();clearStartedResolve();await delay(800);return route.fulfill({response});});
  await openClear();await page.locator('#clear-history-confirm').click();await clearStarted;
  assert.equal(await page.locator('#clear-history-confirm').isDisabled(),true);assert.equal(await page.locator('#clear-history-cancel').isDisabled(),true);assert.equal(await page.locator('#clear-history').isDisabled(),true);
  await page.keyboard.press('Escape');assert.equal(await page.locator('#clear-history-dialog').isVisible(),true);
  await page.evaluate(()=>document.querySelector('#clear-history-confirm').click());assert.equal(clearRequests,1);
  passed('Pending clear disables repeated actions and keeps the dialog open');
  await page.locator('#clear-history-dialog').waitFor({state:'hidden'});await page.waitForFunction(()=>document.querySelector('#history').textContent==='暂无记录');
  releaseOld();await delay(350);assert.equal(await page.locator('#history tbody tr').count(),0);assert.equal(await page.locator('#history').innerText(),'暂无记录');
  assert.equal(clearRequests,1);assert.equal(await page.locator('#clear-history').isEnabled(),true);
  passed('Successful clear awaits refreshed UI and an older library response cannot restore deleted rows');
  const library=await api('library');assert.deepEqual(library.history,[]);assert.deepEqual(library.favorites,fixture.favorites);
  const saved=JSON.parse(fs.readFileSync(libraryPath,'utf8'));assert.deepEqual(saved.history,[]);assert.deepEqual(saved.favorites,fixture.favorites);assert.deepEqual(saved.bindings,fixture.bindings);assert.deepEqual(saved.settings,fixture.settings);
  await page.screenshot({path:path.join(out,'clear_history_empty.png')});
  passed('Real backend stores an empty history while preserving favorites, bindings and settings');
  await stop();await start();await openLibrary();await page.waitForFunction(()=>document.querySelector('#history').textContent==='暂无记录');
  assert.equal(await page.locator('#favorites .film-card').count(),1);assert.equal(await page.locator('#favorites .film-card h3').innerText(),'保留的收藏');assert.deepEqual((await api('library')).history,[]);
  passed('After full EXE restart, history remains empty and the favorite remains available');
  assert.deepEqual(report.page_errors,[]);assert.deepEqual(report.browser_dialog_events,[]);assert.deepEqual(report.external_requests,[]);await stop();
}
main().catch(e=>{report.failure=String(e.stack||e);console.error(report.failure);process.exitCode=1;}).finally(async()=>{
  if(proc&&proc.exitCode===null){try{if(session)await api('quit',{});await eventually(()=>proc.exitCode!==null,'cleanup quit',10000);}catch{proc.kill();await delay(500);}}
  if(path.dirname(temp)===os.tmpdir()&&path.basename(temp).startsWith('yingxia-clear-history-'))try{fs.rmSync(temp,{recursive:true,force:true,maxRetries:30,retryDelay:100});}catch(e){report.retained_test_directory=temp;report.cleanup_error=String(e);}
  report.check_count=report.checks.length;fs.mkdirSync(out,{recursive:true});fs.writeFileSync(path.join(out,'clear_history_tests.json'),JSON.stringify(report,null,2));
});
