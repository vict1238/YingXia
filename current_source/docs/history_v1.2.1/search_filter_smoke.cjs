// SPDX-License-Identifier: GPL-3.0-or-later
// Developer-only regression: a local Go test server and synthetic source responses.
// This does not check third-party availability or claim that a film was decoded.
'use strict';
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const {spawn} = require('node:child_process');
const {chromium} = require(process.env.PLAYWRIGHT_MODULE || 'playwright');
const root = path.resolve(__dirname, '..');
const out = path.join(__dirname, 'validation_v1.2.1');
const binary = process.env.YINGXIA_TESTSERVER || path.join(root, 'build', process.platform === 'win32' ? 'yingxia_testserver.exe' : 'yingxia_testserver');
const temp = fs.mkdtempSync(path.join(os.tmpdir(), 'yingxia-filter-ui-'));
const report = {
  environment: `${process.platform}/${process.arch}, isolated headless browser, real loopback HTTP`,
  data: 'synthetic catalogue, intentionally failed metadata and accessible test media bytes',
  third_party_requests_allowed: false,
  real_third_party_playback_tested: false,
  checks: [], snapshots: [], page_errors: [], external_requests: []
};
const delay = ms => new Promise(resolve => setTimeout(resolve, ms));
const passed = name => {report.checks.push({name, passed: true}); console.log('PASS:', name);};
let proc, browser, context, page, origin, csrf;
const responseTasks = new Set();
async function api(endpoint, data) {
  const response = await context.request[data === undefined ? 'get' : 'post'](`${origin}/api/${endpoint}`, data === undefined ? {} : {data, headers: {'X-YingXia-CSRF': csrf, Origin: origin}});
  const result = await response.json();
  assert.equal(result.ok, true, `${endpoint}: ${JSON.stringify(result)}`);
  return result.data;
}
function remember(snapshot, endpoint) {
  if(!snapshot || !snapshot.id) return;
  const entry = {
    endpoint, id: snapshot.id, phase: snapshot.phase, percent: snapshot.percent,
    source_completed: snapshot.source_completed, source_total: snapshot.source_total,
    checked_count: snapshot.checked_count, check_total: snapshot.check_total,
    available_count: snapshot.available_count,
    items: (snapshot.items || []).map(f => ({id: f.id, title: f.title, availability: f.availability})),
    message: snapshot.message
  };
  report.snapshots.push(entry);
}
function validateSnapshot(snapshot) {
  for(const key of ['percent', 'source_completed', 'source_total', 'checked_count', 'check_total', 'available_count']) {
    assert.ok(Number.isFinite(snapshot[key]), `${key} must be a finite number: ${JSON.stringify(snapshot)}`);
    assert.ok(snapshot[key] >= 0, `${key} must not be negative`);
  }
  assert.ok(snapshot.percent <= 100);
  assert.ok(snapshot.source_completed <= snapshot.source_total);
  assert.ok(snapshot.checked_count <= snapshot.check_total);
  assert.ok(snapshot.available_count <= (snapshot.items || []).length);
}
async function waitDone(jobID, timeout = 20000) {
  const until = Date.now() + timeout;
  while(Date.now() < until) {
    const snapshot = await api(`search/status?id=${encodeURIComponent(jobID)}`);
    remember(snapshot, 'test-status');
    validateSnapshot(snapshot);
    if(snapshot.phase === 'done') return snapshot;
    assert.ok(!['failed', 'cancelled'].includes(snapshot.phase), `job unexpectedly ${snapshot.phase}: ${snapshot.message}`);
    await delay(100);
  }
  throw new Error(`search job ${jobID} did not finish`);
}
async function search(query) {
  await page.locator('#query').fill(query);
  const started = page.waitForResponse(r => r.url() === `${origin}/api/search/start` && r.request().method() === 'POST' && r.request().postDataJSON()?.query === query);
  await page.locator('#search-submit').click();
  const body = await (await started).json();
  assert.equal(body.ok, true, JSON.stringify(body));
  remember(body.data, 'test-start');
  return body.data;
}
async function main() {
  fs.mkdirSync(out, {recursive: true});
  proc = spawn(binary, ['-test.run=^TestUIServerHarness$', '-test.v'], {env: {...process.env, YINGXIA_UI_TEST_DIR: temp}, windowsHide: true});
  origin = await new Promise((resolve, reject) => {
    let output = '';
    const timer = setTimeout(() => reject(new Error('test server startup timed out')), 15000);
    proc.once('error', reject);
    proc.once('exit', code => {clearTimeout(timer); reject(new Error(`test server exited: ${code}\n${output}`));});
    proc.stdout.on('data', chunk => {
      output += chunk;
      const match = output.match(/APP_URL=(http:\/\/127\.0\.0\.1:\d+)/);
      if(match) {clearTimeout(timer); resolve(match[1]);}
    });
    proc.stderr.on('data', chunk => {output += chunk;});
  });
  browser = await chromium.launch({headless: true, ...(process.env.CHROMIUM_PATH ? {executablePath: process.env.CHROMIUM_PATH} : {})});
  context = await browser.newContext({viewport: {width: 1340, height: 920}, locale: 'zh-CN'});
  page = await context.newPage();
  page.setDefaultTimeout(15000);
  page.on('pageerror', error => report.page_errors.push(String(error)));
  await page.route('**/*', route => {
    const url = route.request().url();
    if(new URL(url).origin === origin) return route.continue();
    report.external_requests.push(url.split('?')[0]);
    return route.abort('blockedbyclient');
  });
  page.on('response', response => {
    if(!/\/api\/search\/(?:start|status)(?:\?|$)/.test(response.url())) return;
    const task = response.json().then(body => {if(body.ok) remember(body.data, new URL(response.url()).pathname);}).catch(() => {});
    responseTasks.add(task);
    task.finally(() => responseTasks.delete(task));
  });
  await page.goto(origin);
  await page.waitForFunction(() => document.querySelector('#search-mode')?.options.length > 2);
  csrf = await page.evaluate(() => window.YX_BOOT.csrf);
  const initial = await api('state');
  assert.equal(initial.version, '1.2.1-exe');
  assert.equal(await page.locator('#search-progress').isVisible(), false);
  assert.equal(await page.locator('#search-progress-bar').getAttribute('max'), '100');
  await api('settings', {network_consent: true, auto_update_sources: false});
  await page.reload();
  await page.waitForFunction(() => document.querySelector('#network-consent')?.checked === true);
  csrf = await page.evaluate(() => window.YX_BOOT.csrf);
  await page.locator('#search-mode').selectOption('provider:wsyzy');
  passed('Version and initially hidden determinate progress control load through the local server');

  const pendingVisible = page.locator('#results .availability-badge[data-state="pending"]').first().waitFor();
  const job = await search('筛选测试');
  validateSnapshot(job);
  await pendingVisible;
  const progress = await page.evaluate(() => {
    const bar = document.querySelector('#search-progress-bar');
    const pending = [...document.querySelectorAll('#results .film-card')].filter(c => c.querySelector('.availability-badge[data-state="pending"]'));
    return {percent: bar.value, maximum: bar.max, label: document.querySelector('#search-progress-label').textContent,
      pending_count: pending.length, pending_disabled: pending.every(c => c.disabled),
      progress_visible: !document.querySelector('#search-progress').classList.contains('hidden')};
  });
  assert.ok(progress.pending_count > 0);
  assert.equal(progress.pending_disabled, true);
  assert.equal(progress.progress_visible, true);
  assert.ok(progress.percent >= 0 && progress.percent < 100);
  assert.equal(progress.maximum, 100);
  assert.ok(progress.label.trim().length > 0);
  report.progress_observation = progress;
  await page.screenshot({path: path.join(out, 'ui_search_progress.png'), fullPage: true});
  passed('Checking displays real progress and disables untested result cards');

  const done = await waitDone(job.id);
  await page.waitForFunction(() => document.querySelectorAll('#results .availability-badge[data-state="pending"]').length === 0 && document.querySelectorAll('#results .film-card').length === 3);
  assert.equal(done.percent, 100);
  assert.equal(done.source_completed, done.source_total);
  assert.equal(done.checked_count, done.check_total);
  const byTitle = text => done.items.find(f => f.title.includes(text));
  const bad = byTitle('无法访问'), good = byTitle('可访问'), alternate = byTitle('备用线路');
  assert.ok(bad && good && alternate, 'the three availability fixtures must all remain in the result');
  assert.equal(bad.availability.state, 'failed');
  assert.equal(good.availability.state, 'media_ok');
  assert.equal(alternate.availability.state, 'media_ok');
  const sortedStates = done.items.map(f => f.availability.state);
  assert.deepEqual(sortedStates, ['media_ok', 'media_ok', 'failed']);
  assert.equal(done.available_count, 2);
  for(const f of done.items) assert.ok(f.availability.label, 'availability must have a readable label');
  const cards = await page.locator('#results .film-card').evaluateAll(nodes => nodes.map(n => ({title: n.querySelector('h3').textContent, state: n.querySelector('.availability-badge').dataset.state, label: n.querySelector('.availability-badge').textContent, disabled: n.disabled})));
  assert.deepEqual(cards.map(c => c.state), sortedStates);
  assert.ok(cards.slice(0, 2).every(c => !c.disabled));
  assert.ok(cards[2].title.includes('无法访问'));
  report.final_cards = cards;
  await page.screenshot({path: path.join(out, 'ui_search_filtered.png'), fullPage: true});
  passed('Actual media-byte probes move accessible results ahead of failed metadata while retaining every candidate');

  const observations = report.snapshots.filter(s => s.id === job.id);
  assert.ok(observations.some(s => s.phase === 'checking' && s.check_total > 0));
  for(const s of observations) validateSnapshot(s);
  for(let i = 1; i < observations.length; i++) {
    // Requests from the UI and this script can complete in different orders.
    if(observations[i - 1].endpoint === observations[i].endpoint && observations[i].phase !== 'done') {
      assert.ok(observations[i].percent >= observations[i - 1].percent, 'job progress regressed');
    }
  }
  passed('Status snapshots report bounded source and candidate counts and reach 100 percent only on completion');

  const detail = await api(`detail?id=${encodeURIComponent(alternate.id)}`);
  assert.notEqual(alternate.availability.group_key, detail.groups[0].key, 'alternate fixture must have a failed first group');
  const expectedGroup = detail.groups.find(g => g.key === alternate.availability.group_key);
  const expectedEpisode = expectedGroup?.episodes.find(e => e.key === alternate.availability.episode_key);
  assert.ok(expectedEpisode, 'successful sample must refer to an actual detail episode');
  await page.locator('#results .film-card').filter({hasText: '筛选测试 备用线路'}).click();
  await page.waitForFunction(title => document.querySelector('#film-title').textContent === title, alternate.title);
  assert.equal(await page.locator('#group-select').inputValue(), expectedGroup.key);
  assert.equal(await page.locator('#episodes .selected').innerText(), expectedEpisode.name);
  assert.equal(await page.locator('#favorites .availability-badge, #history .availability-badge').count(), 0);
  report.selected_sample = {group_key: expectedGroup.key, episode_key: expectedEpisode.key, episode_name: expectedEpisode.name};
  passed('Opening a result selects the group and episode whose availability check passed');

  await page.locator('[data-view=search]').click();
  let heldResolve, heldJobID, releaseHeld;
  const heldReady = new Promise(resolve => {heldResolve = resolve;});
  const release = new Promise(resolve => {releaseHeld = resolve;});
  let holdEnabled = true;
  await page.route(/\/api\/search\/status\?/, async route => {
    if(!holdEnabled) return route.continue();
    const response = await route.fetch();
    const body = await response.json();
    if(body.ok && body.data.phase === 'checking') {
      holdEnabled = false;
      heldJobID = body.data.id;
      heldResolve(body.data);
      await release;
    }
    await route.fulfill({response});
  });
  const cancelledStart = await search('取消测试');
  await Promise.race([heldReady, delay(12000).then(() => {throw new Error('could not hold an in-flight checking response');})]);
  assert.equal(heldJobID, cancelledStart.id);
  const cancelledResponse = page.waitForResponse(r => r.url() === `${origin}/api/search/cancel` && r.request().method() === 'POST');
  await page.locator('#search-cancel').click();
  const cancelledBody = await (await cancelledResponse).json();
  assert.equal(cancelledBody.ok, true, JSON.stringify(cancelledBody));
  assert.equal(cancelledBody.data.phase, 'cancelled');
  await page.waitForFunction(() => !document.querySelector('#search-submit').disabled);
  const cancelled = await api(`search/status?id=${encodeURIComponent(cancelledStart.id)}`);
  assert.equal(cancelled.phase, 'cancelled');
  remember(cancelled, 'cancelled-status');
  const replacement = await search('Example Journey 第二季');
  assert.notEqual(replacement.id, cancelledStart.id);
  const replacementDone = await waitDone(replacement.id);
  await page.waitForFunction(() => document.querySelectorAll('#results .film-card').length === 2 && !document.querySelector('#results').textContent.includes('取消测试'));
  releaseHeld();
  await delay(1600);
  assert.equal(await page.locator('#results .film-card').count(), 2);
  assert.doesNotMatch(await page.locator('#results').innerText(), /取消测试|筛选测试/);
  assert.ok(replacementDone.items.every(f => !/取消测试|筛选测试/.test(f.title)));
  await page.unroute(/\/api\/search\/status\?/);
  passed('Cancel aborts the old job; a delayed old checking response cannot overwrite a replacement query');

  let resolveStartSeen, resolveOverlapCancel, delayedStartID;
  const oldStartSeen = new Promise(resolve => {resolveStartSeen = resolve;});
  const oldCancelAcknowledged = new Promise(resolve => {resolveOverlapCancel = resolve;});
  const overlapOrder = [];
  let delayOldStart = true;
  await page.route(`${origin}/api/search/start`, async route => {
    const input = route.request().postDataJSON();
    if(input.query === '取消测试' && delayOldStart) {
      delayOldStart = false;
      resolveStartSeen();
      // Delay before the backend sees this request, so a premature replacement
      // would create its job first and then be superseded by this older start.
      await delay(700);
      const response = await route.fetch();
      const body = await response.json();
      delayedStartID = body.data?.id;
      overlapOrder.push('old_start_returned');
      return route.fulfill({response});
    }
    if(input.query === 'Example Journey 第二季') overlapOrder.push('new_start_sent');
    return route.continue();
  });
  await page.route(`${origin}/api/search/cancel`, async route => {
    if(route.request().postDataJSON().id !== delayedStartID) return route.continue();
    const response = await route.fetch();
    const body = await response.json();
    overlapOrder.push('old_cancel_acknowledged');
    resolveOverlapCancel(body);
    return route.fulfill({response});
  });
  const delayedResponse = page.waitForResponse(r => r.url() === `${origin}/api/search/start` && r.request().method() === 'POST' && r.request().postDataJSON()?.query === '取消测试');
  await page.locator('#query').fill('取消测试');
  await page.locator('#search-submit').click();
  await Promise.race([oldStartSeen, delay(12000).then(() => {throw new Error('delayed old start request was not observed');})]);
  assert.equal(await page.locator('#search-progress').isVisible(), true);
  await page.locator('#search-cancel').click();
  await page.waitForFunction(() => !document.querySelector('#search-submit').disabled);
  const overlapReplacement = await search('Example Journey 第二季');
  const oldStartBody = await (await delayedResponse).json();
  assert.equal(oldStartBody.ok, true, JSON.stringify(oldStartBody));
  const oldCancelBody = await Promise.race([oldCancelAcknowledged, delay(12000).then(() => {throw new Error('late old start was not cancelled');})]);
  assert.equal(oldCancelBody.ok, true, JSON.stringify(oldCancelBody));
  assert.equal(oldCancelBody.data.phase, 'cancelled');
  assert.deepEqual(overlapOrder, ['old_start_returned', 'old_cancel_acknowledged', 'new_start_sent'], 'replacement starts must wait for an in-flight old start and its cancellation');
  assert.notEqual(overlapReplacement.id, oldStartBody.data.id);
  const overlapDone = await waitDone(overlapReplacement.id);
  await page.waitForFunction(() => document.querySelectorAll('#results .film-card').length === 2 && !document.querySelector('#results').textContent.includes('取消测试'));
  await delay(900);
  const stillDone = await api(`search/status?id=${encodeURIComponent(overlapReplacement.id)}`);
  const oldStatusResponse = await context.request.get(`${origin}/api/search/status?id=${encodeURIComponent(oldStartBody.data.id)}`);
  const oldStatusBody = await oldStatusResponse.json();
  // The backend retains only a bounded number of jobs; cancellation was already
  // acknowledged before the replacement, so subsequent eviction is acceptable.
  if(oldStatusResponse.status() === 404) assert.equal(oldStatusBody.error, 'search_missing');
  else {assert.equal(oldStatusBody.ok, true, JSON.stringify(oldStatusBody)); assert.equal(oldStatusBody.data.phase, 'cancelled');}
  assert.equal(stillDone.phase, 'done');
  assert.equal(overlapDone.items.length, 2);
  assert.doesNotMatch(await page.locator('#results').innerText(), /取消测试|筛选测试/);
  report.start_overlap = {order: overlapOrder, old_id: oldStartBody.data.id, replacement_id: overlapReplacement.id, old_cancel_phase: oldCancelBody.data.phase, old_retained_phase: oldStatusBody.data?.phase || 'expired_after_cancel', replacement_phase: stillDone.phase};
  remember(oldStatusBody.data, 'overlap-old-status');
  remember(stillDone, 'overlap-new-status');
  await page.unroute(`${origin}/api/search/start`);
  await page.unroute(`${origin}/api/search/cancel`);
  passed('Cancelling before the old start returns serializes its cancellation before a replacement start and preserves the new result');

  const wrongCSRF = await context.request.post(`${origin}/api/search/cancel`, {data: {id: replacement.id}, headers: {'X-YingXia-CSRF': 'invalid', Origin: origin}});
  assert.equal(wrongCSRF.status(), 403);
  const anonymous = await browser.newContext();
  const noSession = await anonymous.request.get(`${origin}/api/search/status?id=${encodeURIComponent(replacement.id)}`);
  assert.equal(noSession.status(), 403);
  await anonymous.close();
  passed('Search status requires a session and cancellation retains CSRF protection');

  for(const width of [1340, 1024, 720, 390]) {
    await page.setViewportSize({width, height: 920});
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1), true, `overflow at ${width}`);
  }
  passed('Progress and result badges fit desktop and narrow windows');
  await Promise.all([...responseTasks]);
  assert.deepEqual(report.page_errors, []);
  assert.deepEqual(report.external_requests, []);
  passed('No uncaught JavaScript errors or third-party browser requests');
  report.browser = await browser.version();
}
main().catch(error => {report.failure = String(error.stack || error); console.error(report.failure); process.exitCode = 1;}).finally(async () => {
  if(context && origin && csrf) {try {await api('quit', {});} catch {}}
  if(browser) await browser.close();
  if(proc && proc.exitCode === null) {await delay(300); if(proc.exitCode === null) proc.kill();}
  report.check_count = report.checks.length;
  fs.mkdirSync(out, {recursive: true});
  fs.writeFileSync(path.join(out, 'search_filter_browser_tests.json'), JSON.stringify(report, null, 2));
  if(path.dirname(temp) === os.tmpdir() && path.basename(temp).startsWith('yingxia-filter-ui-')) fs.rmSync(temp, {recursive: true, force: true});
});
