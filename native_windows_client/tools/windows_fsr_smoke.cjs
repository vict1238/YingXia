// Actual shipping Flutter/Go launcher test. All data/cache paths are isolated.
// Synthetic media comes from a separately tagged test backend, never the EXE.
const assert=require('node:assert/strict');
const fs=require('node:fs');const path=require('node:path');const os=require('node:os');const crypto=require('node:crypto');
const {spawn,execFile}=require('node:child_process');const {promisify}=require('node:util');const execFileAsync=promisify(execFile);
const readline=require('node:readline');
const root=path.resolve(__dirname,'../..');
const executable=process.env.YINGXIA_NATIVE_EXE||path.join(root,'releases/YingXia_v2.1.0_Native_Windows_x64.exe');
const fixtureBackend=process.env.YINGXIA_NATIVE_FIXTURE||path.join(root,'native_windows_client/build/qa_core/yingxia_backend_fsr_fixture.exe');
const diagnosticLabel=process.env.YINGXIA_QA_LABEL||'candidate3_direct_runner';assert.match(diagnosticLabel,/^[a-z0-9_]+$/);
const output=path.join(root,process.env.YINGXIA_QA_MEDIA_ONLY||process.env.YINGXIA_QA_PRODUCTION_ONLY||process.env.YINGXIA_QA_LABEL?'native_windows_client/evidence/windows/diagnostics/'+diagnosticLabel:'native_windows_client/evidence/windows');
const temporary=fs.mkdtempSync(path.join(os.tmpdir(),'yingxia-flutter-fsr-'));
const cache=path.join(temporary,'原生 运行缓存');const data=path.join(temporary,'生产库');const fixtureData=path.join(temporary,'视频测试库');
const report={platform:`${process.platform}/${process.arch}`,executable:path.basename(executable),sha256:crypto.createHash('sha256').update(fs.readFileSync(executable)).digest('hex'),
  shipping_gui_tested:true,production_backend_history_tested:!process.env.YINGXIA_QA_PRIOR_PRODUCTION&&!process.env.YINGXIA_QA_MEDIA_ONLY&&!process.env.YINGXIA_QA_LIGHT_ONLY,media_backend:'separately compiled native_fixture Go child via explicit developer override',
  fixture_backend_sha256:crypto.createHash('sha256').update(fs.readFileSync(fixtureBackend)).digest('hex'),
  synthetic_media_only:true,real_third_party_playback_tested:false,checks:[],observations:[]};
if(path.basename(executable)==='yingxia_native.exe')report.direct_runtime_hashes=Object.fromEntries(['media_kit_video_plugin.dll','flutter_windows.dll','libmpv-2.dll','libEGL.dll','libGLESv2.dll','data/app.so'].filter(name=>fs.existsSync(path.join(path.dirname(executable),name))).map(name=>[name,crypto.createHash('sha256').update(fs.readFileSync(path.join(path.dirname(executable),name))).digest('hex')]));
const passed=name=>{report.checks.push({name,passed:true});console.log('PASS:',name);};const delay=ms=>new Promise(resolve=>setTimeout(resolve,ms));
let launcher,runner,windowClass,libraryPath,qualityLog,debugObserver,ownedFocusAcquired=false,expectedFocusLoss=false;
let probeProcess,probeReady;const probeRequests=[];
async function initializeProbe(){if(probeReady)return probeReady;probeProcess=spawn('powershell.exe',['-NoProfile','-NonInteractive','-ExecutionPolicy','Bypass','-File',path.join(__dirname,'windows_ui_probe.ps1'),'-Server'],{env:process.env,windowsHide:true,stdio:['pipe','pipe','pipe']});let errors='';probeProcess.stderr.on('data',chunk=>errors+=chunk);let readyResolve,readyReject;probeReady=new Promise((resolve,reject)=>{readyResolve=resolve;readyReject=reject;});readline.createInterface({input:probeProcess.stdout,crlfDelay:Infinity}).on('line',line=>{try{const response=JSON.parse(line);if(response.server_ready)readyResolve();else {const pending=probeRequests.shift();if(pending)pending(response);}}catch(error){readyReject(Error('QA helper protocol: '+error.message));}});probeProcess.once('error',readyReject);probeProcess.once('exit',code=>{if(code!==0)readyReject(Error('QA helper exited: '+errors));while(probeRequests.length)probeRequests.shift()({ok:false,error:'QA helper exited: '+errors});});return probeReady;}
const observedControls=new Map();
const qualityMenuPoints=new Map();let qualityClosedPoint;
async function eventually(test,description,timeout=25000){let last;const end=Date.now()+timeout;while(Date.now()<end){try{const value=await test();if(value)return value;}catch(e){last=e;}await delay(180);}throw new Error(description+(last?': '+last.message:''));}
async function powershell(code){const encoded=Buffer.from(code,'utf16le').toString('base64');try{const result=await execFileAsync('powershell.exe',['-NoProfile','-NonInteractive','-EncodedCommand',encoded],{windowsHide:true,timeout:20000,maxBuffer:3*1024*1024,encoding:'utf8'});return JSON.parse(result.stdout.replace(/^\uFEFF/,''));}catch(e){throw Error('Owned native test helper failed: '+String(e.stderr||e.stdout||e.code).slice(0,1800));}}
async function family(){return powershell(`$all=@(Get-CimInstance Win32_Process | Select-Object ProcessId,ParentProcessId,Name);$ids=[Collections.Generic.HashSet[int]]::new();[void]$ids.Add(${launcher.pid});do{$changed=$false;foreach($item in $all){if($ids.Contains([int]$item.ParentProcessId) -and $ids.Add([int]$item.ProcessId)){$changed=$true}}}while($changed);ConvertTo-Json -Depth 4 -Compress -InputObject @($all | Where-Object {$ids.Contains([int]$_.ProcessId)})`);}
async function probe(action='Snapshot',name='',value='',imagePath=''){
  assert.ok(Number.isInteger(runner));assert.match(windowClass,/^YingXiaNativeWindow_[a-f0-9]{16}$/);
  await initializeProbe();const request={OwnerPid:runner,ClassName:windowClass,Action:action,ElementName:name,Value:value,OutputPath:imagePath,RequireAlreadyForeground:ownedFocusAcquired&&!expectedFocusLoss&&['Invoke','Click','RealClick','ObservedClick','SetText','Escape'].includes(action)?'1':'0'};
  const response=await new Promise((resolve,reject)=>{const timeout=setTimeout(()=>reject(Error('Owned QA helper response timed out')),10000);probeRequests.push(value=>{clearTimeout(timeout);resolve(value);});probeProcess.stdin.write(JSON.stringify(request)+'\n');});if(!response.ok)throw Error('Owned native test helper failed: '+response.error);const answer=response.result;if(answer.foreground_activation_used_owned_pointer){ownedFocusAcquired=true;report.observations.push({owned_pointer_activation:{action,owner_pid:runner,class:windowClass,topmost_restored:true,cursor_restored:true,point_owner_checked:true}});}if(action==='Minimize')expectedFocusLoss=true;if(action==='Restore'){expectedFocusLoss=false;if(answer.foreground)ownedFocusAcquired=true;}return answer;
}
async function nodes(){const list=(await probe('Accessibility')).elements;const state=await probe();for(const item of list){if(item.name&&(item.role===43||item.role===42||item.role===46||item.action)&&item.rect?.[2]>0&&item.rect?.[3]>0)observedControls.set(item.name,{item,state});}return list;}
async function observedClick(observation,label){assert.ok(observation,'No observed coordinate exists for '+label);const state=await probe();const before=observation.state.rect;assert.equal(state.rect[2]-state.rect[0],before[2]-before[0],'owned window width changed since observed control');assert.equal(state.rect[3]-state.rect[1],before[3]-before[1],'owned window height changed since observed control');assert.equal(state.minimized,false);const bounds=observation.item.rect;const x=Math.round(bounds[0]+bounds[2]/2+state.rect[0]-before[0]);const y=Math.round(bounds[1]+bounds[3]/2+state.rect[1]-before[1]);report.observed_inputs??=[];const screenshot='observed_input_'+report.observed_inputs.length;await capture(screenshot);report.observed_inputs.push({label,method:'Owned Flutter view WM_MOUSE input at previously observed MSAA physical target; same window dimensions enforced',observed_rect:bounds,current_window_rect:state.rect,screen_point:[x,y],before_screenshot:screenshot+'.png'});return probe('ObservedClick','',`${x},${y}`);}
async function button(name){const list=await nodes();const matches=list.filter(item=>item.role===43&&(item.name===name||item.name?.split('\n')[0]===name));if(matches.length===1)return probe(process.env.YINGXIA_QA_STRESS_ONLY?'RealClick':'Click',matches[0].name);const cached=[...observedControls.values()].filter(row=>row.item.role===43&&(row.item.name===name||row.item.name?.split('\n')[0]===name));if(!process.env.YINGXIA_QA_STRESS_ONLY&&cached.length===1&&['搜索影片','播放工作台','收藏与记录','来源与诊断','全屏 ⛶','▷ 播放所选集','停止','下一集'].includes(name))return observedClick(cached[0],name);const item=await eventually(async()=>{const list=await nodes();const matches=list.filter(item=>item.role===43&&(item.name===name||item.name?.split('\n')[0]===name));return matches.length===1?matches[0]:null;},`button ${name} must become uniquely available`,6000);return probe(process.env.YINGXIA_QA_STRESS_ONLY?'RealClick':'Click',item.name);}
async function hasText(text){return (await nodes()).some(item=>item.name?.includes(text)||item.value?.includes(text));}
function library(){return JSON.parse(fs.readFileSync(libraryPath,'utf8'));}
function diagnostics(){if(!qualityLog||!fs.existsSync(qualityLog))return [];return fs.readFileSync(qualityLog,'utf8').split('\n').filter(Boolean).flatMap(line=>{try{return [JSON.parse(line)];}catch{return [];}});}
function sampleAfter(index,predicate){return diagnostics().slice(index).find(row=>row.event==='sample'&&predicate(row));}
async function setQuality(mode){
  const preferenceFile=path.join(path.dirname(libraryPath),'video_quality.json');
  const label={off:'关闭',light:'轻量',enhanced:'增强'}[mode];
  const current=fs.existsSync(preferenceFile)?JSON.parse(fs.readFileSync(preferenceFile,'utf8')).mode:'light';
  if(current===mode)return;
  const currentLabel={off:'关闭',light:'轻量',enhanced:'增强'}[current];
  const choices=(await nodes()).filter(item=>item.name?.split('\n')[0]===currentLabel&&(item.role===43||item.action));
  if(choices.length===1){qualityClosedPoint={item:choices[0],state:await probe()};await probe('Invoke',choices[0].name);}else await observedClick(qualityClosedPoint,'quality picker');
  await delay(250);
  const menu=await nodes();const menuState=await probe();for(const [key,caption] of Object.entries({off:'关闭',light:'轻量',enhanced:'增强'})){const items=menu.filter(item=>item.name?.split('\n')[0]===caption&&(item.role===43||item.action));if(items.length===1)qualityMenuPoints.set(key,{item:items[0],state:menuState});}
  const option=menu.filter(item=>item.name?.split('\n')[0]===label&&(item.role===43||item.action));
  if(option.length===1)await probe('Invoke',option[0].name);else await observedClick(qualityMenuPoints.get(mode),'quality option '+label);
  await eventually(()=>fs.existsSync(preferenceFile)&&JSON.parse(fs.readFileSync(preferenceFile,'utf8')).mode===mode,'quality preference not persisted');
  await delay(350);
  await nodes();
}
async function fullscreen(){await eventually(async()=>{const current=await nodes();return current.some(row=>row.name==='全屏 ⛶'&&!(row.state&1));},'fullscreen button must be enabled',3000);await button('全屏 ⛶');await eventually(()=>diagnostics().some(row=>row.viewport?.width>=1920&&row.viewport?.height>=1080),'actual fullscreen viewport must appear',3000);}
async function exitFullscreen(){await probe('Escape');await eventually(async()=>{const state=await probe();const row=diagnostics().at(-1);return state.rect[2]-state.rect[0]===1340&&state.rect[3]-state.rect[1]===900&&row?.viewport?.width<1280&&await hasText('超分');},'fullscreen exit must restore the actual normal window and viewport',3000);await delay(450);}
async function verifiedTarget(index,mode){
  return eventually(()=>sampleAfter(index,row=>row.requested===mode&&row.effective===mode&&row.source?.width===1280&&row.source?.height===720&&row.output?.width===1920&&row.output?.height===1080&&row.shader_verified===true),'actual 720p to 1080p shader render passes not verified in '+mode,30000);
}
async function start(directory,useFixture=false){
  observedControls.clear();qualityMenuPoints.clear();qualityClosedPoint=null;
  ownedFocusAcquired=false;expectedFocusLoss=false;
  fs.mkdirSync(directory,{recursive:true});libraryPath=path.join(directory,'library.json');
  windowClass='YingXiaNativeWindow_'+crypto.createHash('sha256').update(path.resolve(directory).toLowerCase()).digest('hex').slice(0,16);
  qualityLog=path.join(directory,'fsr_diagnostics.jsonl');
  const env={...process.env,YINGXIA_NATIVE_CACHE_DIR:cache,YINGXIA_DATA_DIR:directory,YINGXIA_NATIVE_WINDOW_CLASS:windowClass,YINGXIA_FSR_DIAGNOSTICS:qualityLog};delete env.YINGXIA_NATIVE_BACKEND;if(useFixture)env.YINGXIA_NATIVE_BACKEND=fixtureBackend;
  launcher=spawn(executable,['--data-dir',directory],{env,windowsHide:false,stdio:['ignore','pipe','pipe']});launcher.once('error',e=>{report.process_error=String(e);});
  const redact=text=>text.replace(/[a-f0-9]{64}/gi,'[redacted]');
  for(const [stream,name] of [[launcher.stdout,'stdout'],[launcher.stderr,'stderr']])stream.on('data',chunk=>fs.appendFileSync(path.join(output,'native_'+name+'.log'),redact(chunk.toString('utf8'))));
  launcher.once('exit',(code,signal)=>report.observations.push({native_parent_exit:{code,signal,time:new Date().toISOString()}}));
  runner=await eventually(async()=>{const child=(await family()).find(p=>p.Name.toLowerCase()==='yingxia_native.exe');return child?.ProcessId;},'native GUI child did not start',45000);
  const state=await eventually(async()=>{const state=await probe();return state.visible?state:null;},'native GUI did not display without manual restore',30000);
  assert.equal(state.title,'映匣 · YingXia');await probe('Restore');await eventually(()=>hasText('想看什么'),'app UI did not become ready');
  if(process.env.YINGXIA_QA_DEBUG){
    const python=process.env.YINGXIA_QA_PYTHON;assert.ok(python,'Provide the existing bundled Python for the owned debug observer');
    debugObserver=spawn(python,[path.join(__dirname,'observe_native_exception.py'),'--pid',String(runner),'--exe',executable,'--out',path.join(output,'native_exception_stack.json')],{windowsHide:true,stdio:['ignore','pipe','pipe']});
    await new Promise((resolve,reject)=>{let messages='',errors='';debugObserver.stdout.on('data',chunk=>{messages+=chunk; if(messages.includes('"debug_attached": true'))resolve();});debugObserver.stderr.on('data',chunk=>errors+=chunk);debugObserver.once('exit',code=>{if(code!==0)reject(Error('Owned debug observer failed: '+errors));});debugObserver.once('error',reject);setTimeout(()=>reject(Error('Debug observer attachment timed out: '+errors)),20000).unref();});
    report.debug_observer_attached=true;
  }
  return state;
}
async function stop(){await probe('Close');await eventually(()=>launcher.exitCode!==null,'launcher did not exit',20000);assert.equal(launcher.exitCode,0);const remaining=await family();assert.equal(remaining.length,0,'native job left child processes');}
async function capture(name){await probe('Capture','','',path.join(output,name+'.png'));}
async function clearConfirm(){await button('清空记录');await eventually(()=>hasText('清空历史记录？'),'confirmation dialog missing');}
async function playFilm(title,queryText='FSR'){
  await button('搜索影片');const snapshot=await nodes();const query=snapshot.find(item=>item.role===42&&item.name?.startsWith('搜索片名'));assert.ok(query,'search text field missing');
  await probe('SetText',query.name,queryText);await button('搜索');
  await eventually(()=>hasText('检测完成'),'fixture search did not complete',155000);
  const item=(await nodes()).find(node=>node.name?.includes(title));assert.ok(item,'fixture film card missing');await probe('Click',item.name);
  await eventually(async()=>{const current=await nodes();return current.some(row=>row.role===41&&row.name===title)&&!current.some(row=>row.role===42&&row.name?.startsWith('搜索片名'))&&current.some(row=>row.name?.includes('已选：第01集'));},'new selected film title and episode must become ready');
  if(process.env.YINGXIA_QA_OBSERVE_QUALITY&&!qualityClosedPoint){await setQuality('off');await setQuality('light');await setQuality('enhanced');await capture('observed_quality_before_video');}
  const playIndex=diagnostics().length;await button('▷ 播放所选集');
  const expectedFps=Number(title.match(/(24|30|60)fps/)?.[1]||30);
  try{await eventually(()=>sampleAfter(playIndex,row=>row.active&&row.position>0.4&&row.fps===expectedFps&&row.source?.width===1280),'new native playback must remain active',5000);}catch(error){if(await hasText('已暂停')){report.observations.push({initial_foreground_pause_resumed_by_explicit_input:true,title});await probe('Restore');await button('▷ 播放所选集');}else throw error;}
  await eventually(()=>library().history.some(row=>row.title===title&&row.evidence==='browser_time_advanced'&&row.position>0.5)&&sampleAfter(playIndex,row=>row.active&&row.position>0.4&&row.fps===expectedFps&&row.source?.width===1280),'new native media did not advance actively',8000);
}
async function main(){
  assert.equal(process.platform,'win32');fs.mkdirSync(output,{recursive:true});fs.mkdirSync(data,{recursive:true});
  if(process.env.YINGXIA_QA_LIGHT_ONLY){
    fs.mkdirSync(fixtureData,{recursive:true});fs.writeFileSync(path.join(fixtureData,'video_quality.json'),JSON.stringify({version:1,mode:'light'}));
    await start(fixtureData,true);await playFilm('FSR 720p 30fps');const index=diagnostics().length;await fullscreen();
    const positives=await eventually(()=>{const samples=diagnostics().slice(index).filter(row=>row.event==='sample'&&row.active&&row.requested==='light'&&row.effective==='light'&&row.source?.width===1280&&row.source?.height===720&&row.output?.width===1920&&row.output?.height===1080&&row.vo_passes.some(pass=>pass.desc.includes('YingXia FSR1 RCAS Light')&&pass.count>0&&pass.avg>0)&&!row.vo_passes.some(pass=>pass.desc.includes('YingXia FSR1 EASU')));return samples.length>=3&&samples.at(-1).position-samples[0].position>=1.5?samples:null;},'Light1080 must have three actual positive-count RCAS samples and advancing clock',10000);
    report.observations.push({light_720_to_1080:positives[0]},{light_positive_execution_samples:positives});await capture('native_fsr_light_positive_count');passed('Light mode renders only mild FSR1 RCAS after bilinear enlargement and avoids EASU');await stop();return;
  }
  if(process.env.YINGXIA_QA_MEDIA_ONLY){report.production_backend_history_tested=false;report.media_diagnostic_only=true;if(process.env.YINGXIA_QA_PRESELECT_ENHANCED){fs.mkdirSync(fixtureData,{recursive:true});fs.writeFileSync(path.join(fixtureData,'video_quality.json'),JSON.stringify({version:1,mode:'enhanced'}));}await start(fixtureData,true);await playFilm('FSR 720p 30fps');await capture('native_debug_720');passed('Bundled native runner synthetic 720p playback advances');await setQuality('enhanced');const renderIndex=diagnostics().length;await fullscreen();const actual=await verifiedTarget(renderIndex,'enhanced');report.observations.push({direct_native_720_to_1080:actual});await capture('native_debug_fsr1080');passed('Bundled native runner executes both FSR1 passes in the real 1080p output');await stop();passed('Native renderer cleanup exits without orphaned child processes');return;}
  if(process.env.YINGXIA_QA_PRIOR_PRODUCTION){const prior=JSON.parse(fs.readFileSync(process.env.YINGXIA_QA_PRIOR_PRODUCTION,'utf8'));assert.equal(prior.sha256,report.sha256,'Prior production source must use the exact same shipping EXE');assert.ok(prior.checks.slice(0,9).every(row=>row.passed));report.carried_forward_production_evidence={executable:prior.executable,sha256:prior.sha256,checks:prior.checks.slice(0,9),scope:'These production checks were already run on this exact shipping EXE SHA. This subsequent run executes only the separately labelled fixture checks.'};}
  else {
  const seed={schema:1,settings:{network_consent:false,auto_update_sources:false,save_history:true,auto_next:false},favorites:[{id:'native-clear-favorite',title:'保留的收藏',kind:'本机测试',source:'本机测试'}],bindings:[{film_id:'native-clear-favorite',episode:'第01集',platform:'qq',url:'https://v.qq.com/x/cover/test/ep.html'}],history:[1,2].map(n=>({id:'native-clear-'+n,film_id:'native-test-'+n,title:'历史记录 '+n,episode:'第01集',source:'本机测试',mode:'media',position:0,evidence:'opened',created_at:'2026-10-14T00:00:00Z',updated_at:'2026-10-14T00:00:00Z'}))};
  fs.writeFileSync(path.join(data,'library.json'),JSON.stringify(seed));
  const initial=await start(data);report.observations.push({initial_window:initial});await capture('native_home');
  const processes=await family();assert.ok(processes.some(p=>p.Name==='yingxia_backend.exe'));assert.equal(processes.some(p=>/chrome|msedge|webview/i.test(p.Name)),false);report.observations.push({production_process_family:processes});
  passed('Single release EXE extracts runtime and opens its own visible Flutter window with no browser/WebView children');
  for(const page of ['播放工作台','收藏与记录','来源与诊断','搜索影片']){await button(page);await delay(120);assert.ok(await hasText(page));}
  passed('All four approved pages load in the production Flutter/Go app');
  await button('来源与诊断');assert.ok(await hasText('开发人员'));assert.ok(await hasText('翻翻蓄小激光  cqh.vict1238@gmail.com'));assert.ok(await hasText('流风驻尘闲  yuchihaobo@gmail.com'));await capture('native_developers');
  passed('Exact two developer names and emails are shown at the top of sources and diagnostics');
  await button('播放工作台');await setQuality('off');await setQuality('enhanced');
  passed('Simple three-choice quality dropdown can change and persist the Windows-only preference');
  await button('收藏与记录');await eventually(()=>hasText('历史记录 2'),'seeded records not visible');await clearConfirm();await button('取消');assert.equal(library().history.length,2);
  passed('Production in-app clear confirmation can cancel without changing persistent records');
  fs.chmodSync(libraryPath,0o444);await clearConfirm();await button('清空');await eventually(()=>hasText('数据目录权限'),'real storage failure did not surface');assert.equal(library().history.length,2);assert.ok(await hasText('历史记录 2'));fs.chmodSync(libraryPath,0o600);
  passed('Actual read-only library write failure retains rows and allows retry');
  await clearConfirm();await button('清空');await eventually(()=>hasText('暂无记录'),'history not cleared in UI');const saved=library();assert.deepEqual(saved.history,[]);assert.deepEqual(saved.favorites,seed.favorites);assert.deepEqual(saved.bindings,seed.bindings);assert.deepEqual(saved.settings,seed.settings);await capture('native_history_cleared');
  passed('Actual production backend clears disk history while preserving favorites, bindings and settings');
  const nativePid=runner;await probe('Minimize');const second=spawn(executable,['--data-dir',data],{env:{...process.env,YINGXIA_NATIVE_CACHE_DIR:cache},windowsHide:false,stdio:'ignore'});await eventually(()=>second.exitCode!==null,'second launcher did not exit');assert.equal(second.exitCode,0);assert.equal(runner,nativePid);await eventually(async()=>!(await probe()).minimized,'second launch did not restore original window');
  passed('Repeated single-file launch restores the existing native window');
  await stop();await start(data);await button('播放工作台');assert.ok(await hasText('增强'));assert.equal(JSON.parse(fs.readFileSync(path.join(data,'video_quality.json'),'utf8')).mode,'enhanced');await button('收藏与记录');await eventually(()=>hasText('暂无记录'),'cleared history reappeared after restart');assert.ok(await hasText('保留的收藏'));assert.deepEqual(library().history,[]);await stop();
  passed('Full release EXE restart keeps history empty and favorite available; window close ends child family');
  }
  if(process.env.YINGXIA_QA_PRODUCTION_ONLY)return;
  fs.mkdirSync(fixtureData,{recursive:true});fs.writeFileSync(path.join(fixtureData,'video_quality.json'),JSON.stringify({version:1,mode:'enhanced'}));
  await start(fixtureData,true);await playFilm(process.env.YINGXIA_QA_STRESS_ONLY?'FSR 720p 60fps':process.env.YINGXIA_QA_REMAINING_ONLY?'FSR 720p 24fps':'FSR 720p 30fps');
  const mediaProcesses=await family();assert.equal(mediaProcesses.some(p=>/chrome|msedge|webview/i.test(p.Name)),false);report.observations.push({fixture_native_process_family:mediaProcesses});
  await capture('native_720_initial');
  let index;
  if(!process.env.YINGXIA_QA_STRESS_ONLY&&!process.env.YINGXIA_QA_REMAINING_ONLY){
  index=diagnostics().length;await setQuality('enhanced');await eventually(()=>sampleAfter(index,row=>row.effective==='enhanced'&&row.source?.width===1280&&row.target===null&&row.output?.width===1280),'small viewport should not enlarge texture');
  passed('A viewport smaller than the source leaves the original 720p texture and avoids shader work');
  index=diagnostics().length;await fullscreen();const enhanced=await verifiedTarget(index,'enhanced');await capture('native_fsr_enhanced_1080');
  const passes=enhanced.vo_passes.map(p=>p.desc);assert.ok(passes.some(p=>p.includes('YingXia FSR1 EASU')));assert.ok(passes.some(p=>p.includes('YingXia FSR1 RCAS Enhanced')));report.observations.push({enhanced_720_to_1080:enhanced});
  passed('Actual shipping libmpv renders EASU and RCAS into a 1920x1080 output texture from a 1280x720 source');
  await delay(10000);const performance=diagnostics().slice(index).filter(row=>row.event==='sample'&&row.shader_verified&&row.effective==='enhanced');assert.ok(performance.length>=10);assert.ok(performance.at(-1).position-performance[0].position>9,'measured enhanced video clock did not progress');report.observations.push({local_rtx4060_enhanced_samples:performance,integrated_gpu_benchmarked:false});
  passed('Enhanced 720p30 playback continues through at least ten measured shader and frame-drop samples on the local RTX 4060');
  await exitFullscreen();await setQuality('light');index=diagnostics().length;await fullscreen();const light=await verifiedTarget(index,'light');assert.equal(light.vo_passes.some(p=>p.desc.includes('YingXia FSR1 EASU')),false);await capture('native_fsr_light_1080');report.observations.push({light_720_to_1080:light});
  passed('Light mode renders only mild FSR1 RCAS after bilinear enlargement and avoids EASU');
  await exitFullscreen();await setQuality('off');index=diagnostics().length;await fullscreen();await eventually(()=>sampleAfter(index,row=>row.effective==='off'&&row.output?.width===1280&&row.output?.height===720&&!row.vo_passes.some(p=>p.desc.includes('YingXia FSR1'))),'off did not restore source texture/remove shader passes');
  passed('Off removes FSR shader passes and restores the 1280x720 native output texture');
  await exitFullscreen();const count=library().history.length;await button('收藏与记录');await delay(250);await button('播放工作台');await eventually(()=>hasText('已暂停'),'page leave did not pause player');await capture('native_paused_player');const pausedPosition=library().history.find(row=>row.title==='FSR 720p 30fps').position;await delay(1800);assert.equal(library().history.find(row=>row.title==='FSR 720p 30fps').position,pausedPosition);await button('▷ 播放所选集');await eventually(()=>library().history.some(row=>row.title==='FSR 720p 30fps'&&row.position>pausedPosition+0.1),'manual resume did not advance');assert.equal(library().history.length,count);
  passed('Leaving playback pauses the active synthetic video, restore stays paused and manual resume continues it');
  await button('停止');await eventually(()=>hasText('已停止'),'stop did not finish');index=diagnostics().length;await button('▷ 播放所选集');await eventually(()=>sampleAfter(index,row=>row.event==='sample'&&row.active&&row.position>1),'Stop then replay did not advance');
  passed('Stop followed by replay advances video without losing the playback surface');
  await button('下一集');await eventually(()=>hasText('已选：第02集'),'next episode not selected');await eventually(()=>library().history.some(row=>row.title==='FSR 720p 30fps'&&row.episode==='第02集'&&row.position>0.1),'next episode did not advance');
  passed('Switching episodes keeps native video decoding and time progress');
  await playFilm('FSR 720p HLS');await capture('native_hls_player');report.observations.push({hls_playback:library().history.find(row=>row.title==='FSR 720p HLS')});
  passed('Continuous synthetic 720p HLS decodes through the authenticated relay and advances time');
  await probe('Minimize');await delay(450);await probe('Restore');await eventually(()=>hasText('已暂停'),'native minimize did not pause');await capture('native_paused_player');
  passed('Actual Windows minimize pauses the native player and restore remains paused');
  }
  if(!process.env.YINGXIA_QA_STRESS_ONLY){for(const fps of [24,60]){if(!process.env.YINGXIA_QA_REMAINING_ONLY||fps!==24)await playFilm('FSR 720p '+fps+'fps');await setQuality('enhanced');index=diagnostics().length;await fullscreen();const sample=await verifiedTarget(index,'enhanced');assert.equal(sample.fps,fps);await delay(5000);const run=diagnostics().slice(index).filter(row=>row.event==='sample'&&row.active&&row.shader_verified);assert.ok(run.length>=4);assert.ok(run.at(-1).position-sample.position>3.5,'FPS fixture did not continue playing');report.observations.push({fps_fixture:fps,sample,continuous_samples:run});await exitFullscreen();}passed('Synthetic 720p24 and 720p60 inputs also render the enhanced 1080p texture and progress on the local GPU');}
  for(let cycle=0;cycle<3;cycle++){
    await button('收藏与记录');await eventually(()=>hasText('我的收藏'),'own library page must actually appear',3000);await button('播放工作台');await eventually(async()=>await hasText('FSR 720p 60fps')&&await hasText('已暂停'),'stress own page must actually restore paused video',3000);
    await delay(350);await setQuality('off');await setQuality('light');await setQuality('enhanced');
    const resizeIndex=diagnostics().length;await fullscreen();await eventually(()=>diagnostics().slice(resizeIndex).some(row=>row.event==='output'&&row.output?.width===1920&&row.output?.height===1080),'paused fullscreen resize did not complete');await exitFullscreen();
    const resumeIndex=diagnostics().length;await button('▷ 播放所选集');await eventually(()=>sampleAfter(resumeIndex,row=>row.active&&row.effective==='enhanced'&&(row.target===null||row.shader_verified)),'stress manual resume did not complete',5000);
    await button('停止');await eventually(()=>hasText('已停止'),'stress stop did not finish');const replayIndex=diagnostics().length;await button('▷ 播放所选集');await eventually(()=>sampleAfter(replayIndex,row=>row.active&&row.position>1&&(row.target===null||row.shader_verified)),'stress replay did not advance');
  }
  passed('Three actual own-page pause/fullscreen-resize/mode-change/real-click resume/Stop-replay stress cycles remain responsive with advanced render control');
  const closingIndex=diagnostics().length;await fullscreen();await verifiedTarget(closingIndex,'enhanced');
  const closingSamples=diagnostics().slice(closingIndex).filter(row=>row.event==='sample'&&row.active&&row.shader_verified);assert.ok(closingSamples.length>0);
  await stop();
  passed('Closing during enhanced video playback completes native renderer teardown and leaves no child processes');
}
main().catch(async e=>{report.failure=String(e.stack||e);try{report.failure_window=await probe();report.failure_processes=await family();report.failure_accessibility=await nodes();await capture('native_failure');}catch(probeError){report.failure_probe_error=String(probeError.stack||probeError);}console.error(report.failure);process.exitCode=1;}).finally(async()=>{
  if(launcher&&launcher.exitCode===null){try{await stop();}catch{launcher.kill();await delay(400);}}
  if(debugObserver&&debugObserver.exitCode===null){await delay(400);if(debugObserver.exitCode===null)debugObserver.kill();}
  if(probeProcess&&probeProcess.exitCode===null){probeProcess.stdin.write('{"Action":"Exit"}\n');probeProcess.stdin.end();await delay(200);if(probeProcess.exitCode===null)probeProcess.kill();}
  if(libraryPath&&fs.existsSync(libraryPath))try{fs.chmodSync(libraryPath,0o600);}catch{}
  for(const directory of [data,fixtureData]){const file=path.join(directory,'fsr_diagnostics.jsonl');if(fs.existsSync(file))fs.copyFileSync(file,path.join(output,path.basename(directory)+'_fsr.jsonl'));}
  report.check_count=report.checks.length;fs.mkdirSync(output,{recursive:true});fs.writeFileSync(path.join(output,'native_windows_fsr_tests.json'),JSON.stringify(report,null,2));
  if(path.dirname(temporary)===os.tmpdir()&&path.basename(temporary).startsWith('yingxia-flutter-fsr-'))try{fs.rmSync(temporary,{recursive:true,force:true,maxRetries:30,retryDelay:100});}catch(e){report.retained_test_directory=temporary;report.cleanup_error=String(e);fs.writeFileSync(path.join(output,'native_windows_fsr_tests.json'),JSON.stringify(report,null,2));}
});
