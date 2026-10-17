// Actual shipping Flutter/Go launcher test. All data/cache paths are isolated.
// Synthetic media comes from a separately tagged test backend, never the EXE.
const assert=require('node:assert/strict');
const fs=require('node:fs');const path=require('node:path');const os=require('node:os');const crypto=require('node:crypto');
const {spawn,execFile}=require('node:child_process');const {promisify}=require('node:util');const execFileAsync=promisify(execFile);
const root=path.resolve(__dirname,'../..');
const executable=process.env.YINGXIA_NATIVE_EXE||path.join(root,'releases/YingXia_v2.0.0_Native_Windows_x64.exe');
const fixtureBackend=process.env.YINGXIA_NATIVE_FIXTURE||path.join(root,'native_core/build/yingxia_backend_fixture_v2.exe');
const output=path.join(root,'native_client/evidence/windows');
const temporary=fs.mkdtempSync(path.join(os.tmpdir(),'yingxia-flutter-native-'));
const cache=path.join(temporary,'原生 运行缓存');const data=path.join(temporary,'生产库');const fixtureData=path.join(temporary,'视频测试库');
const report={platform:`${process.platform}/${process.arch}`,executable:path.basename(executable),sha256:crypto.createHash('sha256').update(fs.readFileSync(executable)).digest('hex'),
  shipping_gui_tested:true,production_backend_history_tested:true,media_backend:'separately compiled native_fixture Go child via explicit developer override',
  fixture_backend_sha256:crypto.createHash('sha256').update(fs.readFileSync(fixtureBackend)).digest('hex'),
  synthetic_media_only:true,real_third_party_playback_tested:false,checks:[],observations:[]};
const passed=name=>{report.checks.push({name,passed:true});console.log('PASS:',name);};const delay=ms=>new Promise(resolve=>setTimeout(resolve,ms));
let launcher,runner,windowClass,libraryPath;
async function eventually(test,description,timeout=25000){let last;const end=Date.now()+timeout;while(Date.now()<end){try{const value=await test();if(value)return value;}catch(e){last=e;}await delay(180);}throw new Error(description+(last?': '+last.message:''));}
async function powershell(code){const encoded=Buffer.from(code,'utf16le').toString('base64');try{const result=await execFileAsync('powershell.exe',['-NoProfile','-NonInteractive','-EncodedCommand',encoded],{windowsHide:true,timeout:20000,maxBuffer:3*1024*1024,encoding:'utf8'});return JSON.parse(result.stdout.replace(/^\uFEFF/,''));}catch(e){throw Error('Owned native test helper failed: '+String(e.stderr||e.stdout||e.code).slice(0,1800));}}
async function family(){return powershell(`$all=@(Get-CimInstance Win32_Process | Select-Object ProcessId,ParentProcessId,Name);$ids=[Collections.Generic.HashSet[int]]::new();[void]$ids.Add(${launcher.pid});do{$changed=$false;foreach($item in $all){if($ids.Contains([int]$item.ParentProcessId) -and $ids.Add([int]$item.ProcessId)){$changed=$true}}}while($changed);ConvertTo-Json -Depth 4 -Compress -InputObject @($all | Where-Object {$ids.Contains([int]$_.ProcessId)})`);}
async function probe(action='Snapshot',name='',value='',imagePath=''){
  assert.ok(Number.isInteger(runner));assert.match(windowClass,/^YingXiaNativeWindow_[a-f0-9]{16}$/);
  const quote=v=>"'"+v.replace(/'/g,"''")+"'";
  const body=fs.readFileSync(path.join(__dirname,'windows_ui_probe.ps1'),'utf8');
  const command=`& {\n${body}\n} -OwnerPid ${runner} -ClassName ${quote(windowClass)} -Action ${quote(action)}${name?' -ElementName '+quote(name):''}${value?' -Value '+quote(value):''}${imagePath?' -OutputPath '+quote(imagePath):''}`;
  return powershell(command);
}
async function nodes(){return (await probe('Accessibility')).elements;}
async function button(name){const list=await nodes();const matches=list.filter(item=>item.role===43&&(item.name===name||item.name?.split('\n')[0]===name));assert.equal(matches.length,1,`button ${name} must be unique`);return probe('Invoke',matches[0].name);}
async function hasText(text){return (await nodes()).some(item=>item.name?.includes(text));}
function library(){return JSON.parse(fs.readFileSync(libraryPath,'utf8'));}
async function start(directory,useFixture=false){
  fs.mkdirSync(directory,{recursive:true});libraryPath=path.join(directory,'library.json');
  windowClass='YingXiaNativeWindow_'+crypto.createHash('sha256').update(path.resolve(directory).toLowerCase()).digest('hex').slice(0,16);
  const env={...process.env,YINGXIA_NATIVE_CACHE_DIR:cache,YINGXIA_DATA_DIR:directory};delete env.YINGXIA_NATIVE_BACKEND;if(useFixture)env.YINGXIA_NATIVE_BACKEND=fixtureBackend;
  launcher=spawn(executable,['--data-dir',directory],{env,windowsHide:false,stdio:'ignore'});launcher.once('error',e=>{report.process_error=String(e);});
  runner=await eventually(async()=>{const child=(await family()).find(p=>p.Name.toLowerCase()==='yingxia_native.exe');return child?.ProcessId;},'native GUI child did not start',45000);
  const state=await eventually(async()=>{const state=await probe();return state.visible?state:null;},'native GUI did not display without manual restore',30000);
  assert.equal(state.title,'映匣 · YingXia');await probe('Restore');await eventually(()=>hasText('想看什么'),'app UI did not become ready');
  return state;
}
async function stop(){await probe('Close');await eventually(()=>launcher.exitCode!==null,'launcher did not exit',20000);assert.equal(launcher.exitCode,0);const remaining=await family();assert.equal(remaining.length,0,'native job left child processes');}
async function capture(name){await probe('Capture','','',path.join(output,name+'.png'));}
async function clearConfirm(){await button('清空记录');await eventually(()=>hasText('清空历史记录？'),'confirmation dialog missing');}
async function playFilm(title){
  await button('搜索影片');const snapshot=await nodes();const query=snapshot.find(item=>item.role===42&&item.name?.startsWith('搜索片名'));assert.ok(query,'search text field missing');
  await probe('SetText',query.name,'原生播放测试');await button('搜索');
  await eventually(()=>hasText('检测完成'),'fixture search did not complete',155000);
  const item=(await nodes()).find(node=>node.name?.includes(title));assert.ok(item,'fixture film card missing');await probe('Click',item.name);
  await eventually(()=>hasText('已选：第01集'),'episode selection missing');await button('▷ 播放所选集');
  await eventually(()=>library().history.some(row=>row.title===title&&row.evidence==='browser_time_advanced'&&row.position>0.1),'native media did not advance',20000);
}
async function main(){
  assert.equal(process.platform,'win32');fs.mkdirSync(output,{recursive:true});fs.mkdirSync(data,{recursive:true});
  const seed={schema:1,settings:{network_consent:false,auto_update_sources:false,save_history:true,auto_next:false},favorites:[{id:'native-clear-favorite',title:'保留的收藏',kind:'本机测试',source:'本机测试'}],bindings:[{film_id:'native-clear-favorite',episode:'第01集',platform:'qq',url:'https://v.qq.com/x/cover/test/ep.html'}],history:[1,2].map(n=>({id:'native-clear-'+n,film_id:'native-test-'+n,title:'历史记录 '+n,episode:'第01集',source:'本机测试',mode:'media',position:0,evidence:'opened',created_at:'2026-10-14T00:00:00Z',updated_at:'2026-10-14T00:00:00Z'}))};
  fs.writeFileSync(path.join(data,'library.json'),JSON.stringify(seed));
  const initial=await start(data);report.observations.push({initial_window:initial});await capture('native_home');
  const processes=await family();assert.ok(processes.some(p=>p.Name==='yingxia_backend.exe'));assert.equal(processes.some(p=>/chrome|msedge|webview/i.test(p.Name)),false);report.observations.push({production_process_family:processes});
  passed('Single release EXE extracts runtime and opens its own visible Flutter window with no browser/WebView children');
  for(const page of ['播放工作台','收藏与记录','来源与诊断','搜索影片']){await button(page);await delay(120);assert.ok(await hasText(page));}
  passed('All four approved pages load in the production Flutter/Go app');
  await button('收藏与记录');await eventually(()=>hasText('历史记录 2'),'seeded records not visible');await clearConfirm();await button('取消');assert.equal(library().history.length,2);
  passed('Production in-app clear confirmation can cancel without changing persistent records');
  fs.chmodSync(libraryPath,0o444);await clearConfirm();await button('清空');await eventually(()=>hasText('数据目录权限'),'real storage failure did not surface');assert.equal(library().history.length,2);assert.ok(await hasText('历史记录 2'));fs.chmodSync(libraryPath,0o600);
  passed('Actual read-only library write failure retains rows and allows retry');
  await clearConfirm();await button('清空');await eventually(()=>hasText('暂无记录'),'history not cleared in UI');const saved=library();assert.deepEqual(saved.history,[]);assert.deepEqual(saved.favorites,seed.favorites);assert.deepEqual(saved.bindings,seed.bindings);assert.deepEqual(saved.settings,seed.settings);await capture('native_history_cleared');
  passed('Actual production backend clears disk history while preserving favorites, bindings and settings');
  const nativePid=runner;await probe('Minimize');const second=spawn(executable,['--data-dir',data],{env:{...process.env,YINGXIA_NATIVE_CACHE_DIR:cache},windowsHide:false,stdio:'ignore'});await eventually(()=>second.exitCode!==null,'second launcher did not exit');assert.equal(second.exitCode,0);assert.equal(runner,nativePid);await eventually(async()=>!(await probe()).minimized,'second launch did not restore original window');
  passed('Repeated single-file launch restores the existing native window');
  await stop();await start(data);await button('收藏与记录');await eventually(()=>hasText('暂无记录'),'cleared history reappeared after restart');assert.ok(await hasText('保留的收藏'));assert.deepEqual(library().history,[]);await stop();
  passed('Full release EXE restart keeps history empty and favorite available; window close ends child family');
  await start(fixtureData,true);await playFilm('原生播放测试 WebM');
  const count=library().history.length;await button('收藏与记录');await delay(250);await button('播放工作台');await eventually(()=>hasText('已暂停'),'page leave did not pause player');await capture('native_webm_player');await button('＋ 收藏');const pausedPosition=library().history.find(row=>row.title==='原生播放测试 WebM').position;await button('▷ 播放所选集');await eventually(()=>library().history.some(row=>row.title==='原生播放测试 WebM'&&row.position>pausedPosition+0.1),'manual resume did not advance');assert.equal(library().history.length,count);
  passed('Native libmpv WebM playback advances; leaving workspace pauses and manual resume reuses playback');
  await playFilm('原生播放测试 HLS');await capture('native_hls_player');report.observations.push({hls_playback:library().history.find(row=>row.title==='原生播放测试 HLS')});
  passed('Native libmpv HLS playback decodes the synthetic authenticated relay and advances time');
  await probe('Minimize');await delay(450);await probe('Restore');await eventually(()=>hasText('已暂停'),'native minimize did not pause');await capture('native_paused_player');
  passed('Actual Windows minimize pauses the native player and restore remains paused');
  await stop();
}
main().catch(async e=>{report.failure=String(e.stack||e);try{report.failure_accessibility=await nodes();await capture('native_failure');}catch{}console.error(report.failure);process.exitCode=1;}).finally(async()=>{
  if(launcher&&launcher.exitCode===null){try{await stop();}catch{launcher.kill();await delay(400);}}
  if(libraryPath&&fs.existsSync(libraryPath))try{fs.chmodSync(libraryPath,0o600);}catch{}
  report.check_count=report.checks.length;fs.mkdirSync(output,{recursive:true});fs.writeFileSync(path.join(output,'native_windows_tests.json'),JSON.stringify(report,null,2));
  if(path.dirname(temporary)===os.tmpdir()&&path.basename(temporary).startsWith('yingxia-flutter-native-'))try{fs.rmSync(temporary,{recursive:true,force:true,maxRetries:30,retryDelay:100});}catch(e){report.retained_test_directory=temporary;report.cleanup_error=String(e);fs.writeFileSync(path.join(output,'native_windows_tests.json'),JSON.stringify(report,null,2));}
});
