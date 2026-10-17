// SPDX-License-Identifier: GPL-3.0-or-later
// Optional real-network validation of the final EXE, with an isolated data directory.
// Run after building v1.2.1; this checks bounded media samples, never decoder playback.
'use strict';
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const {spawn} = require('node:child_process');
const crypto = require('node:crypto');
const root = path.resolve(__dirname, '..');
const out = path.join(__dirname, 'validation_v1.2.1');
const packaged = path.resolve(root, '..', 'YingXia_v1.2.1_Windows_x64.exe');
const executable = process.env.YINGXIA_EXE || (fs.existsSync(packaged) ? packaged : path.resolve(root, '..', 'releases', 'YingXia_v1.2.1_Windows_x64.exe'));
const query = process.env.YINGXIA_LIVE_QUERY || '葬送的芙莉莲';
const delay = ms => new Promise(resolve => setTimeout(resolve, ms));
const clean = value => String(value || '').replace(/https?:\/\/[^\s"'<>]+/gi, '[URL omitted]');
const tiers = {media_ok: 0, page_ok: 1, unchecked: 2, failed: 3};
const report = {
  started_at: new Date().toISOString(), executable: path.basename(executable), version: '1.2.1-exe',
  environment: `${process.platform}/${process.arch}`, query, mode: 'all',
  scope: 'Real source search, candidate details and bounded playback-page/media-sample access checks',
  startup: '--no-browser with a new temporary data directory', auto_update_sources: false,
  actual_decoder_verification: false, real_third_party_playback_verified: false,
  media_urls_recorded: false, private_request_headers_recorded: false,
  status: 'running', runs: [], errors: []
};
let proc, api, dir, activeJob;
function compact(snapshot) {
  const items = snapshot.items || [];
  const counts = {media_ok: 0, page_ok: 0, unchecked: 0, failed: 0, pending: 0};
  for(const item of items) counts[item.availability?.state || 'pending'] = (counts[item.availability?.state || 'pending'] || 0) + 1;
  return {
    phase: snapshot.phase, percent: snapshot.percent,
    source_completed: snapshot.source_completed, source_total: snapshot.source_total,
    checked_count: snapshot.checked_count, check_total: snapshot.check_total,
    available_count: snapshot.available_count, candidate_count: items.length,
    availability_counts: counts, message: clean(snapshot.message)
  };
}
function validateProgress(snapshot) {
  for(const key of ['percent', 'source_completed', 'source_total', 'checked_count', 'check_total', 'available_count']) {
    assert.ok(Number.isInteger(snapshot[key]) && snapshot[key] >= 0, `${key} must be a nonnegative integer`);
  }
  assert.ok(snapshot.percent <= 100, 'progress exceeds 100');
  assert.ok(snapshot.source_completed <= snapshot.source_total, 'source count exceeds total');
  assert.ok(snapshot.checked_count <= snapshot.check_total, 'checked count exceeds total');
  assert.ok(snapshot.available_count <= (snapshot.items || []).length, 'available count exceeds candidates');
}
function availability(value = {}) {
  const result = {state: value.state, label: clean(value.label), checked_at: value.checked_at || '', latency_ms: value.latency_ms};
  if(value.error) result.error = clean(value.error);
  return result;
}
function validateFinal(final, sourceCount) {
  validateProgress(final);
  assert.equal(final.phase, 'done', `job ended in ${clean(final.phase)}: ${clean(final.message)}`);
  assert.equal(final.percent, 100);
  assert.equal(final.source_completed, final.source_total);
  assert.equal(final.source_total, sourceCount);
  assert.equal(final.source_count, sourceCount);
  assert.ok(Array.isArray(final.source_results));
  assert.equal(final.source_results.length, sourceCount);
  assert.equal(final.successful_sources, final.source_results.filter(source => source.ok === true).length);
  assert.equal(final.check_total, final.items.length);
  const rawCandidateCount = final.source_results.reduce((sum, source) => {
    assert.ok(typeof source.name === 'string' && source.name.trim(), 'source needs a readable name');
    assert.ok(Number.isInteger(source.count) && source.count >= 0, 'source result count is invalid');
    return sum + source.count;
  }, 0);
  assert.equal(final.items.length, Math.min(400, rawCandidateCount), 'source and candidate counts disagree');
  let prior = -1;
  for(const item of final.items) {
    const a = item.availability;
    assert.ok(a && Object.hasOwn(tiers, a.state), 'finished candidate has an unknown or pending state');
    assert.ok(typeof item.title === 'string' && item.title.trim(), 'candidate needs a readable title');
    assert.ok(typeof item.source === 'string' && item.source.trim(), 'candidate needs a readable source');
    assert.ok(typeof a.label === 'string' && a.label.trim(), 'availability needs a readable label');
    assert.ok(tiers[a.state] >= prior, 'availability tiers are not sorted');
    prior = tiers[a.state];
  }
  assert.equal(final.checked_count, final.items.filter(item => item.availability.state !== 'unchecked').length);
  assert.equal(final.available_count, final.items.filter(item => ['media_ok', 'page_ok'].includes(item.availability.state)).length);
}
async function search(sourceCount, runNumber) {
  const run = {run: runNumber, started_at: new Date().toISOString(), status: 'running', snapshots: [], errors: []};
  report.runs.push(run);
  const started = Date.now();
  let snapshot = await api('search/start', {query, mode: 'all'});
  activeJob = snapshot.id;
  assert.ok(typeof activeJob === 'string' && activeJob, 'search did not return a job ID');
  const deadline = started + 165000;
  let previous = '';
  while(true) {
    validateProgress(snapshot);
    const entry = compact(snapshot), key = JSON.stringify(entry);
    run.progress = entry;
    run.elapsed_ms = Date.now() - started;
    if(key !== previous) {
      previous = key;
      run.snapshots.push({elapsed_ms: Date.now() - started, ...entry});
      console.log(`Run ${runNumber}: ${entry.phase} ${entry.percent}% | sources ${entry.source_completed}/${entry.source_total} | checked ${entry.checked_count}/${entry.check_total} | accessible ${entry.available_count}`);
    }
    if(['done', 'failed', 'cancelled'].includes(snapshot.phase)) break;
    if(Date.now() >= deadline) throw new Error('search did not finish within 165 seconds');
    await delay(750);
    if(Date.now() >= deadline) throw new Error('search did not finish within 165 seconds');
    snapshot = await api(`search/status?id=${encodeURIComponent(activeJob)}`, undefined, Math.min(10000, deadline - Date.now()));
  }
  run.elapsed_ms = Date.now() - started;
  run.progress = compact(snapshot);
  run.sources = (snapshot.source_results || []).map(source => ({name: clean(source.name), ok: source.ok, count: source.count, elapsed_ms: source.elapsed_ms, ...(source.error ? {error: clean(source.error)} : {})}));
  run.items = (snapshot.items || []).map(item => ({title: clean(item.title), source: clean(item.source), availability: availability(item.availability)}));
  for(const source of run.sources) if(source.error) run.errors.push({scope: 'source_search', source: source.name, error: source.error});
  for(const item of run.items) if(item.availability.error) run.errors.push({scope: 'candidate_check', title: item.title, source: item.source, state: item.availability.state, error: item.availability.error});
  validateFinal(snapshot, sourceCount);
  run.status = 'passed';
  activeJob = null;
  return run;
}
async function waitExit(child, milliseconds = 10000) {
  const deadline = Date.now() + milliseconds;
  while(child.exitCode === null && child.signalCode === null && Date.now() < deadline) await delay(100);
  return child.exitCode !== null || child.signalCode !== null;
}
async function main() {
  report.sha256 = crypto.createHash('sha256').update(fs.readFileSync(executable)).digest('hex');
  dir = fs.mkdtempSync(path.join(os.tmpdir(), 'yingxia-live-filter-'));
  proc = spawn(executable, ['--no-browser', '--data-dir', dir], {windowsHide: true, stdio: 'ignore'});
  let spawnError;
  proc.once('error', error => {spawnError = error;});
  const sessionPath = path.join(dir, 'session.json');
  for(let i = 0; i < 100 && !fs.existsSync(sessionPath); i++) {
    if(spawnError) throw spawnError;
    if(proc.exitCode !== null) throw new Error(`release process exited during startup: ${proc.exitCode}`);
    await delay(100);
  }
  assert.ok(fs.existsSync(sessionPath), 'release session file was not created');
  const {origin} = JSON.parse(fs.readFileSync(sessionPath, 'utf8'));
  assert.match(origin, /^http:\/\/127\.0\.0\.1:\d+$/);
  const home = await fetch(origin, {signal: AbortSignal.timeout(10000)});
  assert.equal(home.status, 200);
  const cookie = home.headers.get('set-cookie').split(';')[0];
  await home.text();
  const boot = await fetch(`${origin}/bootstrap.js`, {headers: {Cookie: cookie}, signal: AbortSignal.timeout(10000)});
  const bootText = await boot.text();
  const csrf = JSON.parse(bootText.slice(bootText.indexOf('=') + 1).trim().replace(/;$/, '')).csrf;
  api = async (endpoint, data, timeoutMS = 10000) => {
    const response = await fetch(`${origin}/api/${endpoint}`, {
      method: data === undefined ? 'GET' : 'POST', signal: AbortSignal.timeout(Math.max(1, timeoutMS)),
      headers: {Cookie: cookie, Origin: origin, 'Content-Type': 'application/json', 'X-YingXia-CSRF': csrf},
      ...(data === undefined ? {} : {body: JSON.stringify(data)})
    });
    const body = await response.json();
    if(!response.ok || !body.ok) throw new Error(`${endpoint.split('?')[0]}: ${clean(body.message || body.error || response.status)}`);
    return body.data;
  };
  const initial = await api('state');
  assert.equal(initial.version, '1.2.1-exe');
  await api('settings', {auto_update_sources: false, network_consent: true});
  const state = await api('state');
  assert.equal(state.network_consent, true);
  assert.equal(state.source_registry.auto_update, false);
  const sourceCount = state.source_registry.providers.filter(source => source.enabled).length;
  report.enabled_source_count = sourceCount;
  const first = await search(sourceCount, 1);
  if(process.env.YINGXIA_LIVE_REPEAT === '1') {
    const repeat = await search(sourceCount, 2);
    report.repeat_observation = {first_elapsed_ms: first.elapsed_ms, repeat_elapsed_ms: repeat.elapsed_ms, timing_proves_cache_hit: false, note: 'Same-query timing is observational; source search still runs and candidate sets may differ.'};
  }
  report.status = 'passed';
}
main().catch(error => {
  report.status = 'failed';
  if(report.runs.at(-1)?.status === 'running') report.runs.at(-1).status = 'failed';
  report.failure = clean(error.stack || error);
  report.errors.push({scope: 'script', error: report.failure});
  console.error(report.failure);
  process.exitCode = 1;
}).finally(async () => {
  if(activeJob && api) {try {await api('search/cancel', {id: activeJob});} catch(error) {report.errors.push({scope: 'cleanup_cancel', error: clean(error.message)});}}
  if(api) {try {await api('quit', {});} catch(error) {report.errors.push({scope: 'cleanup_quit', error: clean(error.message)});}}
  if(proc && !await waitExit(proc, 3000)) {proc.kill(); await waitExit(proc, 3000);}
  report.finished_at = new Date().toISOString();
  report.elapsed_ms = Date.parse(report.finished_at) - Date.parse(report.started_at);
  report.progress = report.runs.at(-1)?.progress || null;
  for(const run of report.runs) for(const error of run.errors) report.errors.push({run: run.run, ...error});
  fs.mkdirSync(out, {recursive: true});
  fs.writeFileSync(path.join(out, 'live_search_filter.json'), JSON.stringify(report, null, 2));
  // Only remove the absolute temp directory created by this run, after its process exits.
  if(dir && path.dirname(dir) === os.tmpdir() && path.basename(dir).startsWith('yingxia-live-filter-') && (!proc || proc.exitCode !== null || proc.signalCode !== null)) fs.rmSync(dir, {recursive: true, force: true});
});
