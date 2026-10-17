// Developer-only UI regression. Uses a test binary, synthetic catalogue and test clips.
// No third-party requests are allowed during this run.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const {spawn} = require('node:child_process');
const {chromium} = require(process.env.PLAYWRIGHT_MODULE || 'playwright');
const root = path.resolve(__dirname, '..');
const out = path.join(__dirname, 'validation_v1.0.2');
const binary = process.env.YINGXIA_TESTSERVER || path.join(root, 'build', process.platform === 'win32' ? 'yingxia_testserver.exe' : 'yingxia_testserver');
const temp = fs.mkdtempSync(path.join(os.tmpdir(), 'yingxia-ui-'));
const report = {environment: `${process.platform}/${process.arch}, native local HTTP navigation, isolated headless browser`, catalogue: 'synthetic fixtures', media: 'test-only WebM', real_third_party_playback_tested: false, checks: [], page_errors: [], external_requests: []};
const passed = name => { report.checks.push({name, passed: true}); console.log('PASS:', name); };
const delay = ms => new Promise(resolve => setTimeout(resolve, ms));
let proc, browser, context, origin, csrf;
async function api(endpoint, data) {
  const response = await context.request[ data === undefined ? 'get' : 'post' ](`${origin}/api/${endpoint}`, data === undefined ? {} : {data, headers: {'X-YingXia-CSRF': csrf, Origin: origin}});
  const result = await response.json();
  assert.equal(result.ok, true, JSON.stringify(result));
  return result.data;
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
  const page = await context.newPage();
  page.setDefaultTimeout(10000);
  page.on('pageerror', error => report.page_errors.push(String(error)));
  await page.route('**/*', route => {
    if(new URL(route.request().url()).origin === origin) return route.continue();
    report.external_requests.push(route.request().url().split('?')[0]);
    return route.fulfill({status: 200, contentType: 'text/html', body: '<!doctype html><title>Test player</title>'});
  });
  await page.goto(origin);
  await page.waitForFunction(() => document.querySelector('#player-source').options.length > 0);
  csrf = await page.evaluate(() => window.YX_BOOT.csrf);
  assert.equal(await page.locator('#network-consent').isChecked(), false);
  assert.equal(await page.locator('#demo-btn, #search-welcome, #quit, .side-bottom, .footnote, .eyebrow, .muted-chip').count(), 0);
  assert.equal(await page.locator('#results-header').isVisible(), false);
  await page.screenshot({path: path.join(out, 'ui_home.png'), fullPage: true});
  passed('Minimal home loads without removed sections or JavaScript errors');
  for(const view of ['workspace', 'library', 'settings', 'search']) {
    await page.locator(`[data-view=${view}]`).click();
    assert.equal(await page.locator(`#view-${view}`).isVisible(), true);
  }
  passed('All four navigation pages open');
  await page.locator('#query').fill('Example Journey 第二季');
  await page.locator('#search-submit').click();
  await page.locator('#consent-dialog').waitFor({state: 'visible'});
  assert.equal(await page.locator('#results .film-card').count(), 0);
  await page.locator('#consent-cancel').click();
  assert.equal(await page.locator('#network-consent').isChecked(), false);
  await page.locator('#search-submit').click();
  await page.locator('#consent-accept').click();
  await page.waitForFunction(() => document.querySelectorAll('#results .film-card').length === 2);
  assert.match(await page.locator('#results .film-card h3').first().innerText(), /第二季/);
  assert.equal(await page.locator('#search-message').isVisible(), false);
  await page.screenshot({path: path.join(out, 'ui_results.png'), fullPage: true});
  passed('Network consent still gates queries; synthetic search and season ordering work');
  await page.locator('#results .film-card').first().click();
  await page.waitForFunction(() => document.querySelectorAll('#episodes button').length === 2);
  assert.equal(await page.locator('#group-select option').count(), 2);
  await page.locator('#play').click();
  await page.locator('#stage iframe').waitFor();
  await page.waitForFunction(() => document.querySelector('#playback-status').textContent === '播放页已打开');
  assert.notEqual(await page.locator('#playback-dot').getAttribute('class'), 'good');
  assert.equal((await api('library')).history[0].evidence, 'opened');
  passed('Iframe page load stays distinct from observed video playback');
  await page.locator('#stop').click();
  await page.route('**/api/resolve', async route => {const response = await route.fetch(); await delay(800); await route.fulfill({response});});
  await page.locator('#play').click();
  await page.locator('#stop').click();
  await page.waitForTimeout(1200);
  assert.equal(await page.locator('#stage iframe, #stage video').count(), 0);
  await page.unroute('**/api/resolve');
  passed('Stop discards delayed playback responses');
  const fixture = await api('detail?id=demo-orbit-1');
  await api('favorite', {film_id: fixture.id});
  await page.locator('[data-view=library]').click();
  await page.locator('#favorites .film-card').first().click();
  await page.waitForFunction(() => document.querySelector('#film-title').textContent === '轨道片段 第一季');
  await page.locator('#play').click();
  await page.waitForFunction(() => document.querySelector('#stage video')?.currentTime > 1.2);
  const video = await page.locator('#stage video').evaluate(v => ({time: v.currentTime, frames: v.getVideoPlaybackQuality().totalVideoFrames, width: v.videoWidth}));
  assert.ok(video.frames > 0 && video.width > 0);
  report.test_media_observation = video;
  passed('Test-only media decodes and advances time through the real player UI');
  await page.locator('#next').click();
  await page.waitForFunction(() => document.querySelector('#stage video')?.currentSrc.endsWith('/demo/clip2.webm'));
  await page.waitForFunction(() => document.querySelector('#stage video')?.currentTime > 0.5);
  assert.match(await page.locator('#selected-episode').innerText(), /第02集/);
  await page.screenshot({path: path.join(out, 'ui_player.png'), fullPage: true});
  passed('Next episode selects and plays the correct test clip');
  await page.locator('#group-select').selectOption('g1');
  await page.waitForFunction(() => document.querySelector('#workspace-message').textContent.includes('缺少对应集数'));
  assert.equal(await page.locator('#play').isDisabled(), true);
  assert.equal(await page.locator('#stage video').count(), 0);
  passed('A source missing the selected episode does not play another episode');
  await page.locator('[data-view=library]').click();
  await page.waitForFunction(() => document.querySelector('#history').textContent.includes('已播放'));
  await page.screenshot({path: path.join(out, 'ui_library.png'), fullPage: true});
  await page.reload();
  await page.waitForFunction(() => document.querySelector('#player-source').options.length > 0);
  csrf = await page.evaluate(() => window.YX_BOOT.csrf);
  await page.locator('[data-view=library]').click();
  await page.locator('#favorites .film-card').waitFor();
  passed('Favorites and playback history survive page reload');
  await page.locator('#import-open').click();
  await page.locator('#import-title').fill('测试播放页');
  await page.locator('#import-url').fill('https://v.qq.com/x/cover/test/ep.html?token=secret&vid=abc');
  await page.locator('#import-form button[type=submit]').click();
  await page.waitForFunction(() => document.querySelector('#film-title').textContent === '测试播放页');
  assert.equal(await page.locator('#episodes button').count(), 1);
  assert.equal((await page.locator('#binding-url').inputValue()).includes('secret'), false);
  await page.locator('#player-source').selectOption('qilin');
  await page.locator('#bind-save').click();
  await page.locator('#play').click();
  await page.locator('#stage iframe').waitFor();
  passed('Import, per-episode binding and source selection still work');
  await page.locator('#stop').click();
  await page.locator('[data-view=settings]').click();
  await page.locator('#diagnose').click();
  await page.locator('#diagnostics table').waitFor();
  assert.match(await page.locator('#diagnostics').innerText(), /fixture HTTP only/);
  await page.evaluate(() => {window.scrollTo(0, 0); document.querySelector('#toast').classList.add('hidden');});
  await page.screenshot({path: path.join(out, 'ui_settings.png'), fullPage: true});
  passed('Source settings and HTTP diagnostics work');
  for(const width of [1340, 1024, 720, 390]) {
    await page.setViewportSize({width, height: 920});
    await page.locator('[data-view=search]').click();
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1), true, `overflow at ${width}`);
  }
  passed('Search layout fits desktop and narrow window widths');
  assert.deepEqual(report.page_errors, []);
  passed('No uncaught JavaScript errors throughout the run');
  report.browser = await browser.version();
}
main().catch(error => {report.failure = String(error.stack || error); console.error(report.failure); process.exitCode = 1;}).finally(async () => {
  if(context && origin && csrf) {try {await api('quit', {});} catch {}}
  if(browser) await browser.close();
  if(proc && proc.exitCode === null) {await delay(300); if(proc.exitCode === null) proc.kill();}
  report.check_count = report.checks.length;
  fs.writeFileSync(path.join(out, 'browser_tests.json'), JSON.stringify(report, null, 2));
  // temp is always an absolute directory created by this script in the system temp folder.
  if(path.dirname(temp) === os.tmpdir() && path.basename(temp).startsWith('yingxia-ui-')) fs.rmSync(temp, {recursive: true, force: true});
});
