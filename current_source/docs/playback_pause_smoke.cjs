// Developer-only v1.3.0 UI regression. Real browser decoding uses synthetic
// test clips and the Go test harness; no live catalogue/player is contacted.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const {spawn} = require('node:child_process');
const {chromium} = require(process.env.PLAYWRIGHT_MODULE || 'playwright');
const root = path.resolve(__dirname, '..');
const out = path.join(__dirname, 'validation_v1.3.0');
const binary = process.env.YINGXIA_TESTSERVER || path.join(root, 'build', process.platform === 'win32' ? 'yingxia_testserver.exe' : 'yingxia_testserver');
const temp = fs.mkdtempSync(path.join(os.tmpdir(), 'yingxia-pause-ui-'));
const instruction = '选集之后请点击下方播放所选集';
const report = {
  environment: `${process.platform}/${process.arch}, isolated headless browser, native local HTTP navigation`,
  catalogue: 'synthetic fixtures', media: 'test-only WebM and H264 HLS',
  real_third_party_playback_tested: false, checks: [], observations: [], page_errors: [], intercepted_external_requests: []
};
const delay = ms => new Promise(resolve => setTimeout(resolve, ms));
const passed = name => {report.checks.push({name, passed: true}); console.log('PASS:', name);};
let proc, browser, context, origin, csrf, page, resolveRequests = 0;
async function api(endpoint, data) {
  const response = await context.request[data === undefined ? 'get' : 'post'](`${origin}/api/${endpoint}`, data === undefined ? {} : {data, headers: {'X-YingXia-CSRF': csrf, Origin: origin}});
  const result = await response.json();
  assert.equal(result.ok, true, JSON.stringify(result));
  return result.data;
}
async function videoState() {
  return page.locator('#stage video').evaluate(v => ({time: v.currentTime, paused: v.paused, ended: v.ended, source: v.currentSrc, frames: v.getVideoPlaybackQuality().totalVideoFrames, width: v.videoWidth, marker: v.dataset.pauseTestMarker}));
}
async function openFavorite(title) {
  await page.locator('[data-view=library]').click();
  await page.locator('#favorites .film-card').filter({hasText: title}).click();
  await page.waitForFunction(text => document.querySelector('#film-title').textContent === text, title);
  await page.waitForFunction(() => !document.querySelector('#play').disabled);
}
async function checkPauseAndResume(view, label) {
  const marker = `pause-${label}-${view}`;
  await page.locator('#stage video').evaluate((v, value) => {v.dataset.pauseTestMarker = value;}, marker);
  const episode = await page.locator('#selected-episode').innerText();
  const film = await page.locator('#film-title').innerText();
  const source = await page.locator('#player-source').inputValue();
  const beforeCount = resolveRequests;
  const historyBefore = (await api('library')).history.map(h => h.id).sort();
  await page.locator(`[data-view=${view}]`).click();
  await page.waitForFunction(() => document.querySelector('#stage video')?.paused === true);
  const paused = await videoState();
  assert.equal(paused.marker, marker, `${label}: player must remain mounted`);
  assert.ok(!paused.ended, `${label}: pause must happen before the clip ends`);
  await page.waitForTimeout(650);
  const frozen = await videoState();
  assert.equal(frozen.paused, true);
  assert.ok(Math.abs(frozen.time - paused.time) < 0.08, `${label}: hidden video advanced by ${frozen.time - paused.time}s`);
  await page.locator('[data-view=workspace]').click();
  await page.waitForTimeout(180);
  const returned = await videoState();
  assert.equal(returned.paused, true, `${label}: returning must not resume automatically`);
  assert.equal(returned.marker, marker);
  assert.equal(returned.source, paused.source);
  assert.equal(await page.locator('#selected-episode').innerText(), episode);
  assert.equal(await page.locator('#film-title').innerText(), film);
  assert.equal(await page.locator('#player-source').inputValue(), source);
  assert.equal(resolveRequests, beforeCount, `${label}: navigation must not resolve again`);
  await page.locator('#play').click();
  await page.waitForFunction(time => {const v = document.querySelector('#stage video'); return v && !v.paused && v.currentTime > time + 0.15;}, paused.time);
  const resumed = await videoState();
  assert.equal(resumed.marker, marker, `${label}: Play should reuse the paused player`);
  assert.equal(resumed.source, paused.source);
  assert.equal(resolveRequests, beforeCount, `${label}: resuming must not create a new resolve/history entry`);
  assert.deepEqual((await api('library')).history.map(h => h.id).sort(), historyBefore);
  report.observations.push({label, view, paused, frozen, returned, resumed});
  passed(`${label}: leaving for ${view} pauses; returning stays paused; Play resumes the same position/player`);
}
async function delayedResolveTest() {
  await page.locator('#stop').click();
  let sawResolve;
  const requested = new Promise(resolve => {sawResolve = resolve;});
  const handler = async route => {
    const response = await route.fetch();
    sawResolve();
    await delay(800);
    await route.fulfill({response});
  };
  await page.route('**/api/resolve', handler);
  await page.locator('#play').click();
  await requested;
  await page.locator('[data-view=search]').click();
  await page.waitForTimeout(1150);
  assert.equal(await page.locator('#stage video, #stage iframe').count(), 0);
  await page.locator('[data-view=workspace]').click();
  await page.waitForTimeout(180);
  assert.equal(await page.locator('#stage video, #stage iframe').count(), 0);
  assert.equal(await page.locator('#play').isEnabled(), true);
  await page.unroute('**/api/resolve', handler);
  passed('A resolve response arriving after navigation cannot mount or autoplay a player');
}
async function main() {
  fs.mkdirSync(out, {recursive: true});
  proc = spawn(binary, ['-test.run=^TestUIServerHarness$', '-test.v'], {env: {...process.env, YINGXIA_UI_TEST_DIR: temp}, windowsHide: true});
  origin = await new Promise((resolve, reject) => {
    let output = '';
    const timer = setTimeout(() => reject(new Error('test server startup timed out')), 15000);
    proc.once('error', reject);
    proc.once('exit', code => {clearTimeout(timer); reject(new Error(`test server exited: ${code} ${output}`));});
    proc.stdout.on('data', chunk => {output += chunk; const match = output.match(/APP_URL=(http:\/\/127\.0\.0\.1:\d+)/); if(match) {clearTimeout(timer); resolve(match[1]);}});
    proc.stderr.on('data', chunk => {output += chunk;});
  });
  browser = await chromium.launch({headless: true, ...(process.env.CHROMIUM_PATH ? {executablePath: process.env.CHROMIUM_PATH} : {})});
  context = await browser.newContext({viewport: {width: 1340, height: 920}, locale: 'zh-CN'});
  // Delay only the resolution of play() when explicitly requested. Native play
  // still starts immediately, so this exercises the leave/pending-promise race.
  await context.addInitScript(() => {
    const original = HTMLMediaElement.prototype.play;
    window.__pauseTestPlayCalls = [];
    HTMLMediaElement.prototype.play = function(...args) {
      window.__pauseTestPlayCalls.push({workspace: document.querySelector('#view-workspace')?.classList.contains('active'), time: performance.now()});
      const result = original.apply(this, args);
      return window.__pauseTestDelayPlay ? Promise.all([result, new Promise(resolve => setTimeout(resolve, 800))]).then(() => undefined) : result;
    };
  });
  page = await context.newPage();
  page.setDefaultTimeout(12000);
  page.on('pageerror', error => report.page_errors.push(String(error)));
  page.on('request', request => {if(request.url() === `${origin}/api/resolve`) resolveRequests++;});
  await page.route('**/*', route => {
    if(new URL(route.request().url()).origin === origin) return route.continue();
    report.intercepted_external_requests.push(route.request().url().split('?')[0]);
    return route.fulfill({status: 200, contentType: 'text/html', body: '<!doctype html><title>Synthetic external player</title><body>Fixture page</body>'});
  });
  await page.goto(origin);
  await page.waitForFunction(() => document.querySelector('#player-source').options.length > 0);
  csrf = await page.evaluate(() => window.YX_BOOT.csrf);
  assert.equal(await page.locator('#import-open, #import-dialog, #import-form').count(), 0);
  await page.locator('[data-view=workspace]').click();
  assert.equal(await page.locator('#stage .stage-empty h3').innerText(), instruction);
  await page.screenshot({path: path.join(out, 'ui_workspace.png'), fullPage: true});
  passed('The paste action/dialog are removed and the initial workspace uses the requested instruction');
  await api('settings', {network_consent: true, auto_update_sources: false, auto_next: false});
  await api('detail?id=demo-orbit-1');
  await api('favorite', {film_id: 'demo-orbit-1'});
  await api('favorite', {film_id: 'test-hls'});
  await page.reload();
  await page.waitForFunction(() => document.querySelector('#player-source').options.length > 0);
  csrf = await page.evaluate(() => window.YX_BOOT.csrf);
  await openFavorite('轨道片段 第一季');
  await page.locator('#play').click();
  await page.waitForFunction(() => document.querySelector('#stage video')?.currentTime > 0.7);
  const webm = await videoState();
  assert.ok(webm.frames > 0 && webm.width > 0, 'WebM must decode actual frames');
  report.webm_initial = webm;
  for(const view of ['search', 'library', 'settings']) await checkPauseAndResume(view, 'WebM');
  // Browser lifecycle delivery is simulated here; navigation checks above use
  // real clicks and actual playback. Restoring visibility must not auto-resume.
  await page.evaluate(() => {
    Object.defineProperty(document, 'hidden', {value: true, configurable: true});
    document.dispatchEvent(new Event('visibilitychange'));
    delete document.hidden;
    document.dispatchEvent(new Event('visibilitychange'));
  });
  assert.equal((await videoState()).paused, true);
  await page.waitForTimeout(180);
  assert.equal((await videoState()).paused, true);
  await page.locator('#play').click();
  await page.waitForFunction(() => document.querySelector('#stage video')?.paused === false);
  passed('The simulated hidden-document visibility event pauses playback; visibility restoration does not resume it');
  await page.locator('#stop').click();
  assert.equal(await page.locator('#stage video, #stage iframe').count(), 0);
  assert.equal(await page.locator('#stage .stage-empty h3').innerText(), instruction);
  passed('Stop clears playback and restores the requested instruction');
  await page.evaluate(() => {window.__pauseTestDelayPlay = true;});
  await page.locator('#play').click();
  await page.waitForFunction(() => {const v = document.querySelector('#stage video'); return v && !v.paused;});
  await page.locator('[data-view=library]').click();
  await page.waitForTimeout(1100);
  assert.equal((await videoState()).paused, true);
  await page.locator('[data-view=workspace]').click();
  assert.equal((await videoState()).paused, true);
  await page.evaluate(() => {window.__pauseTestDelayPlay = false;});
  passed('A pending play() promise cannot resume playback after leaving the workspace');
  await delayedResolveTest();
  await openFavorite('HLS 合成测试');
  await page.locator('#player-source').selectOption('native');
  await page.locator('#play').click();
  await page.waitForFunction(() => document.querySelector('#stage video')?.currentTime > 0.7);
  const hls = await videoState();
  assert.ok(hls.frames > 0 && hls.width === 640, 'HLS must decode actual frames');
  assert.match(hls.source, /^blob:/);
  report.hls_initial = hls;
  await checkPauseAndResume('search', 'HLS');
  await page.locator('#stop').click();
  let sawManifest;
  const manifestRequested = new Promise(resolve => {sawManifest = resolve;});
  let delayed = false;
  const mediaHandler = async route => {
    if(delayed) return route.continue();
    delayed = true;
    const response = await route.fetch();
    sawManifest();
    await delay(850);
    await route.fulfill({response});
  };
  await page.route('**/api/media?*', mediaHandler);
  const playsBeforeManifest = await page.evaluate(() => window.__pauseTestPlayCalls.length);
  await page.locator('#play').click();
  await manifestRequested;
  await page.locator('[data-view=search]').click();
  await page.waitForTimeout(1450);
  assert.equal((await videoState()).paused, true);
  assert.equal(await page.evaluate(n => window.__pauseTestPlayCalls.slice(n).some(call => !call.workspace), playsBeforeManifest), false, 'A late HLS manifest must not call play() while away');
  await page.locator('[data-view=workspace]').click();
  await page.waitForTimeout(150);
  assert.equal((await videoState()).paused, true);
  await page.unroute('**/api/media?*', mediaHandler);
  passed('An HLS manifest arriving after navigation leaves the mounted player paused');
  await page.locator('#stop').click();
  await page.locator('[data-view=search]').click();
  await page.locator('#search-mode').selectOption('provider:wsyzy');
  await page.locator('#query').fill('Example Journey 第二季');
  await page.locator('#search-submit').click();
  await page.waitForFunction(() => document.querySelector('#search-progress-bar')?.value === 100, null, {timeout: 155000});
  await page.locator('#results .film-card').first().click();
  await page.waitForFunction(() => document.querySelectorAll('#episodes button').length === 2);
  await page.locator('#player-source').selectOption('catalog');
  await page.locator('#play').click();
  await page.locator('#stage iframe').waitFor();
  await page.waitForFunction(() => document.querySelector('#playback-status').textContent === '播放页已打开');
  const iframeEpisode = await page.locator('#selected-episode').innerText();
  const iframeFilm = await page.locator('#film-title').innerText();
  const iframeCount = resolveRequests;
  await page.locator('[data-view=library]').click();
  assert.equal(await page.locator('#stage iframe').count(), 0, 'Leaving must unload a cross-origin iframe');
  await page.locator('[data-view=workspace]').click();
  await page.waitForTimeout(180);
  assert.equal(await page.locator('#stage iframe').count(), 0);
  assert.equal(await page.locator('#selected-episode').innerText(), iframeEpisode);
  assert.equal(await page.locator('#film-title').innerText(), iframeFilm);
  assert.equal(await page.locator('#player-source').inputValue(), 'catalog');
  assert.equal(resolveRequests, iframeCount);
  await page.locator('#play').click();
  await page.locator('#stage iframe').waitFor();
  assert.equal(resolveRequests, iframeCount + 1, 'Explicit Play may reopen the unloaded external page');
  passed('Leaving unloads the external iframe; selection survives and only explicit Play reopens it');
  await page.locator('#stop').click();
  assert.equal(await page.locator('#stage .stage-empty h3').innerText(), instruction);
  assert.deepEqual(report.page_errors, []);
  passed('No uncaught JavaScript errors throughout playback/navigation regression');
  report.browser = await browser.version();
  report.resolve_requests = resolveRequests;
}
main().catch(error => {report.failure = String(error.stack || error); console.error(report.failure); process.exitCode = 1;}).finally(async () => {
  if(context && origin && csrf) {try {await api('quit', {});} catch {}}
  if(browser) await browser.close();
  if(proc && proc.exitCode === null) {await delay(300); if(proc.exitCode === null) proc.kill();}
  report.check_count = report.checks.length;
  fs.mkdirSync(out, {recursive: true});
  fs.writeFileSync(path.join(out, 'playback_pause_tests.json'), JSON.stringify(report, null, 2));
  if(path.dirname(temp) === os.tmpdir() && path.basename(temp).startsWith('yingxia-pause-ui-')) fs.rmSync(temp, {recursive: true, force: true});
});
