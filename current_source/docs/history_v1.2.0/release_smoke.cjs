// Smoke-test the actual release binary with an isolated temporary data directory.
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const assert = require('node:assert/strict');
const {spawn} = require('node:child_process');
const {chromium} = require(process.env.PLAYWRIGHT_MODULE || 'playwright');
const root = path.resolve(__dirname, '..');
const packagedExecutable = path.resolve(root, '..', 'YingXia_v1.2.0_Windows_x64.exe');
const executable = process.env.YINGXIA_EXE || (fs.existsSync(packagedExecutable) ? packagedExecutable : path.resolve(root, '..', 'releases', 'YingXia_v1.2.0_Windows_x64.exe'));
const out = path.join(__dirname, 'validation_v1.2.0');
const temp = fs.mkdtempSync(path.join(os.tmpdir(), 'yingxia-release-'));
const dataDir = path.join(temp, '中文 空格目录');
const report = {executable: path.basename(executable), environment: `${process.platform}/${process.arch}`, startup: '--no-browser, isolated data directory', checks: [], page_errors: [], real_third_party_playback_tested: false};
report.sha256 = require('node:crypto').createHash('sha256').update(fs.readFileSync(executable)).digest('hex');
const passed = name => {report.checks.push({name, passed: true}); console.log('PASS:', name);};
const delay = ms => new Promise(resolve => setTimeout(resolve, ms));
let proc, browser, session, cookie, csrf;
function start() {const child = spawn(executable, ['--no-browser', '--data-dir', dataDir], {windowsHide: true, stdio: 'ignore'}); child.on('error', error => {report.process_error = String(error);}); return child;}
async function waitExit(child) {for(let i = 0; i < 100; i++) {if(child.exitCode !== null) return; await delay(100);} throw new Error('process did not exit');}
async function connect() {
  for(let i = 0; i < 100; i++) {
    if(fs.existsSync(path.join(dataDir, 'session.json'))) {session = JSON.parse(fs.readFileSync(path.join(dataDir, 'session.json'), 'utf8')); break;}
    await delay(100);
  }
  assert.ok(session?.origin, 'missing session file');
  const home = await fetch(session.origin);
  cookie = home.headers.get('set-cookie').split(';')[0];
  const html = await home.text();
  const boot = await fetch(`${session.origin}/bootstrap.js`, {headers: {Cookie: cookie}});
  csrf = JSON.parse((await boot.text()).split('=', 2)[1].replace(/;$/, '')).csrf;
  return html;
}
async function api(endpoint, data, expected = 200) {
  const response = await fetch(`${session.origin}/api/${endpoint}`, {method: data === undefined ? 'GET' : 'POST', headers: {Cookie: cookie, Origin: session.origin, 'Content-Type': 'application/json', 'X-YingXia-CSRF': csrf}, ...(data === undefined ? {} : {body: JSON.stringify(data)})});
  assert.equal(response.status, expected);
  return response.json();
}
async function main() {
  proc = start();
  const html = await connect();
  assert.match(html, /想看什么/);
  assert.doesNotMatch(html, /先试试离线演示|开始之前|退出软件|独立 EXE 版|免责|DRM/);
  const state = (await api('state')).data;
  assert.equal(state.version, '1.2.0-exe');
  assert.equal(state.source_registry.providers.length,17);
  assert.equal(state.source_registry.auto_update,true);
  assert.equal((await fetch(`${session.origin}/vendor/hls.min.js`,{headers:{Cookie:cookie}})).status,200);
  passed('Release embeds 17 sources, upstream tracking and the HLS library');
  assert.equal(state.network_consent, false);
  passed('Actual Windows GUI EXE starts from a Chinese/space data directory and serves the new embedded UI');
  const unauthorized = await fetch(`${session.origin}/api/state`);
  assert.equal(unauthorized.status, 403);
  passed('Local API still requires a session');
  const clip = await fetch(`${session.origin}/demo/clip1.webm`, {headers: {Cookie: cookie}});
  assert.equal(clip.status, 404);
  await api('search', {demo: true}, 400);
  passed('Release has no demo search endpoint or embedded demo video');
  const imported = (await api('import-platform', {title: '持久化测试', url: 'https://v.qq.com/x/cover/test/ep.html'})).data;
  await api('favorite', {film_id: imported.id});
  assert.equal((await api('library')).data.favorites.length, 1);
  const second = start();
  await waitExit(second);
  assert.equal(second.exitCode, 0);
  assert.equal(JSON.parse(fs.readFileSync(path.join(dataDir, 'session.json'), 'utf8')).pid, proc.pid);
  passed('Repeated launch reuses the running instance');
  browser = await chromium.launch({headless: true, executablePath: process.env.CHROMIUM_PATH || 'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe'});
  const page = await browser.newPage({viewport: {width: 1340, height: 920}, locale: 'zh-CN'});
  page.on('pageerror', error => report.page_errors.push(String(error)));
  await page.goto(session.origin);
  await page.waitForFunction(() => document.querySelector('#player-source').options.length > 0);
  await page.screenshot({path: path.join(out, 'release_home.png'), fullPage: true});
  for(const view of ['workspace', 'library', 'settings', 'search']) {
    await page.locator(`[data-view=${view}]`).click();
    assert.equal(await page.locator(`#view-${view}`).isVisible(), true);
  }
  assert.deepEqual(report.page_errors, []);
  report.browser = await browser.version();
  passed('Production embedded UI loads in isolated headless Edge and all pages open');
  await browser.close(); browser = null;
  await api('quit', {});
  await waitExit(proc);
  assert.equal(fs.existsSync(path.join(dataDir, 'session.json')), false);
  passed('Quit shuts down the backend and clears its session file');
  session = null;
  proc = start();
  await connect();
  assert.equal((await api('library')).data.favorites[0].title, '持久化测试');
  passed('Saved favorites persist after restarting the release EXE');
  await api('quit', {});
  await waitExit(proc);
}
main().catch(error => {report.failure = String(error.stack || error); console.error(report.failure); process.exitCode = 1;}).finally(async () => {
  if(browser) await browser.close();
  if(proc && proc.exitCode === null) {try {if(session) await api('quit', {}); await waitExit(proc);} catch {proc.kill();}}
  report.check_count = report.checks.length;
  fs.mkdirSync(out, {recursive: true});
  fs.writeFileSync(path.join(out, 'release_tests.json'), JSON.stringify(report, null, 2));
  if(path.dirname(temp) === os.tmpdir() && path.basename(temp).startsWith('yingxia-release-')) fs.rmSync(temp, {recursive: true, force: true});
});
