// SPDX-License-Identifier: GPL-3.0-or-later
// Minimal shipping EXE verification. All input and pixels belong to this test's
// own PID/HWND; interaction stops immediately if another app is foreground.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const os = require('node:os');
const crypto = require('node:crypto');
const readline = require('node:readline');
const { spawn, execFile } = require('node:child_process');
const { promisify } = require('node:util');
const execFileAsync = promisify(execFile);
const root = path.resolve(__dirname, '../..');
const executable = process.env.YINGXIA_NATIVE_EXE;
assert.ok(executable && path.isAbsolute(executable), 'Provide the candidate EXE absolute path.');
const label = process.env.YINGXIA_QA_LABEL || 'candidate1';
const backgroundMsaa = process.env.YINGXIA_QA_BACKGROUND_MSAA === '1';
const sourceOnly = process.env.YINGXIA_QA_SOURCE_ONLY === '1';
const measurementOnly = process.env.YINGXIA_QA_MEASUREMENT_ONLY === '1';
assert.match(label, /^[a-z0-9_]+$/);
const output = path.join(root, 'native_windows_client/evidence/v2_2/window', label);
assert.equal(fs.existsSync(output), false, 'Preserve earlier evidence; choose a fresh label.');
fs.mkdirSync(output, { recursive: true });
const temporary = fs.mkdtempSync(path.join(os.tmpdir(), 'yingxia-v22-window-'));
const data = process.env.YINGXIA_QA_RESUME_DATA || path.join(temporary, 'isolated-data');
assert.ok(path.isAbsolute(data) && path.basename(data) === 'isolated-data' && /^yingxia-v22-window-[a-z0-9]+$/i.test(path.basename(path.dirname(data))), 'Only this harness isolated data can be resumed.');
const cache = path.join(temporary, 'isolated-cache');
fs.mkdirSync(data, { recursive: true });
const windowClass = 'YingXiaNativeWindow_' + crypto.createHash('sha256').update(data.toLowerCase()).digest('hex').slice(0, 16);
const report = {
  schema: 1, started_at: new Date().toISOString(), executable: path.basename(executable),
  sha256: crypto.createHash('sha256').update(fs.readFileSync(executable)).digest('hex'),
  shipping_sfx_tested: true, production_backend: true, isolated_data_and_cache: true,
  real_third_party_playback_tested: false, gpu_benchmark_repeated: false,
  input_scope: 'Owned PID/HWND only; no global input, activation attempts or external window capture.',
  checks: [], observations: [], limitations: []
};
if (!fs.existsSync(path.join(data, 'library.json'))) fs.writeFileSync(path.join(data, 'library.json'), JSON.stringify({ schema: 1,
  settings: { network_consent: true, auto_update_sources: false, save_history: true, auto_next: false },
  history: [], favorites: [], bindings: [] }));
const delay = ms => new Promise(resolve => setTimeout(resolve, ms));
const pass = name => { report.checks.push({ name, passed: true }); console.log('PASS:', name); };
async function eventually(test, message, timeout = 12000) {
  let last; const end = Date.now() + timeout;
  while (Date.now() < end) { try { const value = await test(); if (value) return value; } catch (e) { last = e; } await delay(180); }
  throw Error(message + (last ? ': ' + last.message : ''));
}
let launcher, runner, helper, helperReady, helperErrors = ''; const requests = [];
// Add an owned-view-only scroll command in a temporary copy. The preserved
// historical helper remains unchanged; no mouse pointer or foreground change.
const originalProbe = fs.readFileSync(path.join(__dirname, 'windows_ui_probe.ps1'), 'utf8');
let scrollProbe = originalProbe.replace("'Snapshot','Accessibility'", "'Scroll','Snapshot','Accessibility'").replace('switch($Action){', `switch($Action){
  'Scroll' {
    if(![YingXiaNativeProbe]::OwnsForeground($window)){throw 'External application is foreground; no scrolling input attempted.'}
    $delta=0;if(![int]::TryParse($Value,[ref]$delta) -or [Math]::Abs($delta) -gt 2400){throw 'Provide a bounded owned-view wheel delta.'}
    $view=[YingXiaNativeProbe]::FlutterView($window)
    $scrollRect=[YingXiaNativeProbe+Rect]::new();[void][YingXiaNativeProbe]::GetWindowRect($view,[ref]$scrollRect)
    $x=[int](($scrollRect.left+$scrollRect.right)/2);$y=[int](($scrollRect.top+$scrollRect.bottom)/2)
    [void][YingXiaNativeProbe]::SendMessage($view,0x020A,[IntPtr]($delta -shl 16),[IntPtr](($y -shl 16) -bor ($x -band 0xffff)))
  }
`);
assert.notEqual(scrollProbe, originalProbe);
if (backgroundMsaa) {
  assert.ok(scrollProbe.includes('  [YingXiaNativeProbe]::Activate($window)'));
  scrollProbe = scrollProbe.replace('  [YingXiaNativeProbe]::Activate($window)', '  # Owned MSAA / view messages only: never activate or focus an OS window.');
  scrollProbe = scrollProbe.replace('$_.role -eq 43 -or $_.action', '$_.role -eq 43 -or $_.role -eq 44 -or $_.action');
  scrollProbe = scrollProbe.replace("    if(![YingXiaNativeProbe]::OwnsForeground($window)){throw 'External application is foreground; no scrolling input attempted.'}", '    # Scroll is sent only to the previously verified owned Flutter HWND.');
  report.observations.push({ background_msaa_only: true, native_activation_calls_removed_for_all_used_actions: true });
}
const probePath = path.join(temporary, 'owned_v2_2_ui_probe.ps1');
fs.writeFileSync(probePath, scrollProbe);
async function ps(code) {
  const result = await execFileAsync('powershell.exe', ['-NoProfile', '-NonInteractive', '-EncodedCommand', Buffer.from(code, 'utf16le').toString('base64')], { windowsHide: true, timeout: 15000, maxBuffer: 2e6 });
  return JSON.parse(result.stdout.replace(/^\uFEFF/, ''));
}
async function family() {
  return ps(`$all=@(Get-CimInstance Win32_Process | Select-Object ProcessId,ParentProcessId,Name);$ids=[Collections.Generic.HashSet[int]]::new();[void]$ids.Add(${launcher.pid});do{$changed=$false;foreach($item in $all){if($ids.Contains([int]$item.ParentProcessId) -and $ids.Add([int]$item.ProcessId)){$changed=$true}}}while($changed);ConvertTo-Json -Depth 4 -Compress -InputObject @($all | Where-Object {$ids.Contains([int]$_.ProcessId)})`);
}
async function initializeHelper() {
  if (helperReady) return helperReady;
  helper = spawn('powershell.exe', ['-NoProfile', '-NonInteractive', '-ExecutionPolicy', 'Bypass', '-File', probePath, '-Server'], { windowsHide: true, stdio: ['pipe', 'pipe', 'pipe'] });
  helper.stderr.on('data', chunk => helperErrors += chunk);
  helperReady = new Promise((resolve, reject) => {
    readline.createInterface({ input: helper.stdout, crlfDelay: Infinity }).on('line', line => {
      try { const row = JSON.parse(line); if (row.server_ready) resolve(); else requests.shift()?.(row); } catch (e) { reject(e); }
    });
    helper.once('error', reject);
    helper.once('exit', code => { if (code) reject(Error('Owned helper exited: ' + helperErrors)); while (requests.length) requests.shift()({ ok: false, error: 'Owned helper exited.' }); });
  });
  return helperReady;
}
async function probe(action = 'Snapshot', name = '', value = '', image = '') {
  await initializeHelper();
  const request = { OwnerPid: runner, ClassName: windowClass, Action: action, ElementName: name, Value: value, OutputPath: image, RequireAlreadyForeground: '1' };
  const response = await new Promise((resolve, reject) => {
    const timer = setTimeout(() => reject(Error('Owned helper timeout.')), 10000);
    requests.push(row => { clearTimeout(timer); resolve(row); });
    helper.stdin.write(JSON.stringify(request) + '\n');
  });
  if (!response.ok) throw Error(response.error);
  return response.result;
}
async function nodes() { return (await probe('Accessibility')).elements; }
async function has(text) { return (await nodes()).some(row => row.name?.includes(text) || row.value?.includes(text)); }
async function invoke(name) {
  const row = await eventually(async () => {
    const choices = (await nodes()).filter(row => (row.role === 43 || row.role === 44 || row.action) && (row.name === name || row.name?.split('\n')[0] === name));
    return choices.length === 1 ? choices[0] : null;
  }, 'Unique action unavailable: ' + name, 5000);
  await probe('Invoke', row.name); await delay(180);
}
async function textField(prefix, value) {
  const rows = (await nodes()).filter(row => row.role === 42 && (row.name?.startsWith(prefix) || row.name?.includes(prefix)));
  assert.equal(rows.length, 1, 'Unique text field: ' + prefix);
  await probe('SetText', rows[0].name, value); await delay(220);
}
async function choose(current, wanted) {
  await invoke(current); await delay(250); await invoke(wanted); await delay(220);
}
async function capture(name) { await probe('Capture', '', '', path.join(output, name + '.png')); }
async function scroll(delta) { await probe('Scroll', '', String(delta)); await delay(250); }
async function scrollUntil(name, delta) {
  for (let attempt = 0; attempt < 7; attempt++) {
    const bounds = (await probe()).rect;
    if ((await nodes()).some(row => row.name?.includes(name) && row.rect?.[2] > 0 && row.rect?.[3] > 0 && row.rect[1] >= bounds[1] + 30 && row.rect[1] + row.rect[3] <= bounds[3])) return;
    await scroll(delta);
  }
  throw Error('Owned scroll did not reveal control: ' + name);
}
function registry() { return JSON.parse(fs.readFileSync(path.join(data, 'source_registry.json'), 'utf8')); }
function sourceEnabled(source) { const library = JSON.parse(fs.readFileSync(path.join(data, 'library.json'), 'utf8')); return library.settings?.enabled_providers?.[source.id] ?? source.enabled; }
function preference() { const name = path.join(data, 'video_quality.json'); return fs.existsSync(name) ? JSON.parse(fs.readFileSync(name, 'utf8')) : null; }
async function start() {
  const env = { ...process.env, YINGXIA_DATA_DIR: data, YINGXIA_NATIVE_CACHE_DIR: cache, YINGXIA_NATIVE_WINDOW_CLASS: windowClass };
  delete env.YINGXIA_NATIVE_BACKEND; delete env.YINGXIA_QA_REAL_FOCUS_INPUT;
  launcher = spawn(executable, ['--data-dir', data], { env, windowsHide: false, stdio: ['ignore', 'pipe', 'pipe'] });
  for (const [stream, name] of [[launcher.stdout, 'stdout'], [launcher.stderr, 'stderr']]) stream.on('data', chunk => fs.appendFileSync(path.join(output, 'launcher_' + name + '.log'), chunk.toString().replace(/[a-f0-9]{64}/gi, '[redacted]')));
  runner = await eventually(async () => (await family()).find(row => row.Name.toLowerCase() === 'yingxia_native.exe')?.ProcessId, 'Shipping native child did not start.', 45000);
  const state = await eventually(async () => { const state = await probe(); return state.visible ? state : null; }, 'Owned native window not visible.', 20000);
  assert.equal(state.title, '映匣 · YingXia'); await eventually(() => has('想看什么'), 'App not ready.');
  report.observations.push({ launch: state });
  if (!state.foreground && !backgroundMsaa) throw Error('External application is foreground; no activation or global input attempted.');
}
async function stop() {
  if (!launcher || launcher.exitCode !== null) return;
  await probe('Close'); await eventually(() => launcher.exitCode !== null, 'Launcher did not exit.', 15000);
  assert.equal(launcher.exitCode, 0); assert.deepEqual(await family(), []);
  report.observations.push({ normal_close_exit: launcher.exitCode, no_child_family_remaining: true });
}
async function main() {
  await start();
  if (!sourceOnly && !measurementOnly) {
  const processes = await family();
  assert.ok(processes.some(row => row.Name.toLowerCase() === 'yingxia_backend.exe'));
  assert.equal(processes.some(row => /chrome|msedge|webview/i.test(row.Name)), false);
  report.observations.push({ process_family: processes }); pass('Shipping SFX extracts and runs Flutter with its bundled production backend, without browser children');
  await invoke('播放工作台'); assert.ok(await has('关闭')); assert.ok(await has('选集之后请点击下方播放所选集')); await capture('playback_empty_off');
  report.observations.push({ empty_playback_elements: (await nodes()).map(({ name, role, state, rect }) => ({ name, role, state, rect })) });
  pass('Fresh startup has super resolution Off and the empty playback surface contains the text prompt');
  await invoke('来源与诊断'); assert.ok(await has('翻翻蓄小激光  cqh.vict1238@gmail.com')); assert.ok(await has('流风驻尘闲  yuchihaobo@gmail.com'));
  assert.ok(await has('默认超分')); await capture('sources_developers_default_off');
  await scrollUntil('来源管理', -360); assert.ok(await has('来源管理（47）')); assert.equal(registry().providers.length, 47); await capture('sources_47_header');
  await scrollUntil('默认超分', 360);
  pass('Production source panel contains 47 providers, developer credits and startup default control');
  await choose('关闭', '增强'); await eventually(() => preference()?.default_mode === 'enhanced', 'Explicit startup default not saved.');
  await invoke('播放工作台'); assert.ok(await has('关闭')); await choose('关闭', '轻量'); assert.equal(preference().default_mode, 'enhanced');
  await stop(); await start(); await invoke('播放工作台'); assert.ok(await has('增强')); await choose('增强', '轻量'); assert.equal(preference().default_mode, 'enhanced');
  await invoke('来源与诊断'); assert.ok(await has('增强')); await choose('增强', '关闭'); assert.equal(preference().default_mode, 'off');
  await stop(); await start(); await invoke('播放工作台'); assert.ok(await has('关闭'));
  pass('Explicit default survives full EXE restarts while temporary playback selections never change it');
  }
  await invoke('来源与诊断'); await capture('source_manager');
  if (!measurementOnly) {
  // The source table is bounded to 15 rows. Scroll within our own view to its
  // page controls, then return to the query and toolbar.
  // MSAA actions remain valid for offscreen descendants. Background tests must
  // not require a wheel to move the pointer/OS foreground before invoking them.
  await invoke('下一页'); assert.ok(await has('2 / 4 页')); await invoke('上一页');
  await textField('查找来源', 'QA 临时来源'); assert.ok(await has('没有匹配的来源'));
  await invoke('添加来源'); await textField('名称', 'QA 临时来源'); await textField('CMS 接口地址', 'https://example.com/api.php/provide/vod/'); await invoke('添加');
  await eventually(() => has('来源管理（48）'), 'Custom source did not appear.'); assert.ok(await has('QA 临时来源'));
  await invoke('管理来源'); await invoke('编辑'); await textField('名称', 'QA 临时来源 已修改'); await invoke('保存'); await eventually(() => has('QA 临时来源 已修改'), 'Custom edit not visible.');
  const customBefore = registry().providers.find(row => row.name === 'QA 临时来源 已修改');
  assert.ok(customBefore);
  const previousEnabled = sourceEnabled(customBefore);
  await invoke(previousEnabled ? '停用' : '启用'); await delay(350);
  const customAfter = await eventually(() => {
    const row = registry().providers.find(row => row.id === customBefore.id);
    return row && sourceEnabled(row) !== previousEnabled ? row : null;
  }, 'Source enabled toggle was not persisted.');
  report.observations.push({ source_registry_after_edit: { provider_count: registry().providers.length, custom_source: { id: customAfter.id, name: customAfter.name, enabled: sourceEnabled(customAfter) }, previous_enabled: previousEnabled } });
  pass('Source search, paging and custom add/edit/enabled controls work in the production window');
  await invoke('测速'); await eventually(async () => await has('测速中') || await has('测速完成'), 'Single-source measurement did not start.');
  await eventually(async () => await has('测速完成') || await has('测速失败'), 'Single-source measurement did not finish.', 20000); await capture('single_measurement');
  await textField('查找来源', '');
  }
  await invoke('全部测速'); await eventually(() => has('取消测速'), 'All-source measurement progress/cancel unavailable.'); await capture('all_measurement_progress');
  await invoke('取消测速'); await eventually(() => has('已取消'), 'All-source cancellation not visible.');
  pass(measurementOnly ? 'All-source measurement exposes progress and cancellation' : 'Single-source measurement completes and all-source measurement exposes progress and cancellation');
  await textField('查找来源', 'QA 临时来源'); await invoke('管理来源'); await invoke('删除'); await invoke('删除');
  await eventually(() => has('来源管理（47）'), 'Custom removal did not restore provider count.'); assert.ok(await has('没有匹配的来源')); await capture('source_removed');
  pass('Custom source removal restores 47 providers and the filtered list updates');
  await stop(); pass('Final native close exits normally with no owned orphan processes');
}
main().catch(async error => {
  report.failure = { message: error.message, stack: error.stack };
  if (/foreground|External application/.test(error.message)) report.limitations.push('External application was foreground; the test stopped interaction without activation attempts.');
  try { if (runner) { await capture('failure_owned_window'); report.failure_owned_elements = await nodes(); } } catch (_) {}
  try { await stop(); } catch (closeError) {
    report.close_error = closeError.message;
    if (launcher?.pid && launcher.exitCode === null) {
      try {
        report.owned_startup_dialog = await ps(`Add-Type -AssemblyName UIAutomationClient,UIAutomationTypes;$condition=[Windows.Automation.PropertyCondition]::new([Windows.Automation.AutomationElement]::ProcessIdProperty,${launcher.pid});$owned=[Windows.Automation.AutomationElement]::RootElement.FindAll([Windows.Automation.TreeScope]::Children,$condition);$result=@();foreach($item in $owned){if($item.Current.ClassName -eq '#32770'){$result+=@{class=$item.Current.ClassName;title=$item.Current.Name;owner_pid=${launcher.pid};text=@($item.FindAll([Windows.Automation.TreeScope]::Descendants,[Windows.Automation.Condition]::TrueCondition) | ForEach-Object {$_.Current.Name})};$pattern=$null;if($item.TryGetCurrentPattern([Windows.Automation.WindowPattern]::Pattern,[ref]$pattern)){$pattern.Close()}}};ConvertTo-Json -Depth 5 -Compress -InputObject $result`);
        await eventually(() => launcher.exitCode !== null, 'Owned startup dialog did not release launcher.', 3000);
      } catch (dialogError) { report.owned_dialog_close_error = dialogError.message; }
    }
  }
  process.exitCode = 1;
}).finally(async () => {
  report.finished_at = new Date().toISOString();
  fs.writeFileSync(path.join(output, 'native_windows_v2_2_tests.json'), JSON.stringify(report, null, 2) + '\n');
  if (helper) { helper.stdin.end(JSON.stringify({ Action: 'Exit' }) + '\n'); }
  console.log(JSON.stringify({ report: path.join(output, 'native_windows_v2_2_tests.json'), passed: report.checks.length, failure: report.failure?.message || null }));
});
