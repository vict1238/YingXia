"""Verify a candidate production backend using disposable data and private HTTP.

Usage: python verify_v2_3_transport.py --backend PATH [--online] [--output PATH]
Offline checks use seeded synthetic library records and public-shaped official
page identifiers; they never request those pages or any film media. --online
adds a bounded title-only iQiyi suggestion request. Credentials are never saved.
"""
from __future__ import annotations

import argparse
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import queue
import subprocess
import tempfile
import threading
import time
import urllib.error
import urllib.parse
import urllib.request

ROOT = Path(__file__).resolve().parents[2]


class Backend:
    def __init__(self, executable: Path, directory: Path):
        flags = subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0
        self.process = subprocess.Popen(
            [str(executable), '--data-dir', str(directory)],
            stdin=subprocess.PIPE, stdout=subprocess.PIPE,
            stderr=subprocess.DEVNULL, creationflags=flags,
        )
        self.descriptor = {}
        self.http = urllib.request.build_opener(urllib.request.ProxyHandler({}))
        startup = queue.Queue()
        threading.Thread(target=lambda: startup.put(self.process.stdout.readline()), daemon=True).start()
        try:
            raw = startup.get(timeout=20)
            self.descriptor = json.loads(raw)
            if self.descriptor.get('error'):
                raise RuntimeError(self.descriptor['error'])
            parsed = urllib.parse.urlsplit(self.descriptor['origin'])
            if parsed.scheme != 'http' or parsed.hostname != '127.0.0.1' or not parsed.port:
                raise RuntimeError('Unexpected private backend listener')
        except Exception:
            self.process.stdin.close()
            self.process.terminate()
            self.process.wait(timeout=10)
            raise

    def call(self, path, data=None, *, auth=True, csrf=True, headers=None):
        request_headers = {'Origin': self.descriptor['origin']}
        if auth:
            request_headers['Cookie'] = self.descriptor['cookie']
        if data is not None:
            request_headers['Content-Type'] = 'application/json'
            if csrf:
                request_headers['X-YingXia-CSRF'] = self.descriptor['csrf']
        request_headers.update(headers or {})
        request = urllib.request.Request(
            self.descriptor['origin'] + '/api/' + path,
            data=json.dumps(data).encode() if data is not None else None,
            headers=request_headers,
        )
        try:
            with self.http.open(request, timeout=20) as response:
                return response.status, json.load(response)
        except urllib.error.HTTPError as error:
            return error.code, json.load(error)

    def value(self, path, data=None):
        status, body = self.call(path, data)
        if status != 200 or not body.get('ok'):
            raise AssertionError(f'{path.split("?")[0]}: HTTP {status}, {body.get("error")}')
        return body['data']

    def close(self):
        if self.process.poll() is None:
            try:
                self.call('quit', {})
            finally:
                self.process.stdin.close()
                try:
                    self.process.wait(timeout=10)
                except subprocess.TimeoutExpired:
                    self.process.terminate()
                    self.process.wait(timeout=10)
        return self.process.returncode


def seed_library(directory: Path):
    """Seed only an explicitly created TemporaryDirectory, before backend start."""
    histories, favorites, bindings = [], [], []
    for ordinal, title in enumerate(('庆余年', '流浪地球')):
        link = f'https://v.qq.com/x/cover/yingxia_transport_{ordinal}/fixture.html'
        film_id = 'link-' + hashlib.sha256(link.encode()).hexdigest()[:20]
        episode = '当前播放页（集数未核验）'
        favorites.append({'id': film_id, 'title': title, 'source': '合成记录'})
        histories.append({
            'id': f'synthetic-history-{ordinal}', 'film_id': film_id, 'title': title,
            'episode': episode, 'group_key': 'official', 'episode_key': 'page',
            'source': '腾讯视频页面', 'mode': 'external', 'position': 65 if ordinal == 0 else 199,
            'duration': 200, 'completed': ordinal == 1, 'evidence': 'native_time_advanced',
            'created_at': '2026-01-01T00:00:00Z', 'updated_at': '2026-01-01T00:00:00Z',
        })
        bindings.append({'film_id': film_id, 'episode': episode, 'platform': 'qq', 'url': link})
    library = {'schema': 1, 'settings': {'network_consent': False, 'auto_update_sources': False},
               'favorites': favorites, 'history': histories, 'bindings': bindings}
    (directory / 'library.json').write_text(json.dumps(library, ensure_ascii=False), encoding='utf-8')
    return histories


def run(args):
    executable = args.backend.resolve(strict=True)
    report = {
        'created_at': datetime.now(timezone.utc).isoformat(),
        'backend_sha256': hashlib.sha256(executable.read_bytes()).hexdigest(),
        'isolated_data': True, 'real_third_party_media_playback_tested': False,
        'live_network_scope': 'Public Chinese title suggestions only' if args.online else 'No outbound network requested',
        'checks': [], 'observations': [],
    }

    def check(name, condition):
        report['checks'].append({'name': name, 'passed': bool(condition)})
        if not condition:
            raise AssertionError(name)

    backend = None
    try:
        with tempfile.TemporaryDirectory(prefix='yingxia-v2-3-transport-') as raw_directory:
            directory = Path(raw_directory)
            histories = seed_library(directory)
            backend = Backend(executable, directory)
            state = backend.value('state')
            report['version'] = state['version']
            check('Candidate is the production Windows v2.3 backend', state['version'] == '2.3.0-native-windows')
            check('New playback switches default enabled', state['skip_marked_ads'] is True and state['smart_source_switch'] is True)
            check('Relay exposes numeric counters without URLs', isinstance(state.get('media_relay'), dict) and 'http' not in json.dumps(state['media_relay']).lower())
            private_paths = ('search/suggest?q=qyn&online=0', 'resume?film_id=missing', 'playback/alternatives/status?id=missing')
            check('New private GET routes require a session', all(backend.call(path, auth=False)[0] == 403 for path in private_paths))
            check('New private write route retains CSRF', backend.call('playback/alternatives', {}, csrf=False)[0] == 403)
            check('Origin and Host checks remain enforced', backend.call('state', headers={'Origin': 'https://example.com'})[0] == 403 and backend.call('state', headers={'Host': 'invalid.example'})[0] == 403)
            for query in ('qingyunian', 'qyn', 'qing yu nian'):
                suggestion = backend.value('search/suggest?q=' + urllib.parse.quote(query) + '&online=0')
                check('Known title resolves from local pinyin: ' + query, suggestion['resolved_query'] == '庆余年' and suggestion['resolved'] is True and suggestion['scope'] == 'local_metadata')
            suggestion = backend.value('search/suggest?q=' + urllib.parse.quote('庆于年') + '&online=0')
            check('Chinese typo offers the real cached title', any(item['title'] == '庆余年' for item in suggestion['items']))
            resume = backend.value('resume?film_id=' + histories[0]['film_id'])
            check('Resume returns the selected episode and saved position', resume['found'] is True and resume['group_key'] == 'official' and resume['episode_key'] == 'page' and resume['position'] == 65 and resume['policy'] == 'resume')
            generation = resume['history_generation']
            ended = backend.value('resume?film_id=' + histories[1]['film_id'])
            check('Completed episode explicitly restarts the same episode', ended['found'] is True and ended['position'] == 0 and ended['policy'] == 'restart' and ended['episode_key'] == 'page')
            backend.value('progress', {'history_id': histories[0]['id'], 'evidence': 'native_time_advanced', 'position': 72, 'duration': 200, 'completed': False})
            check('Native progress accepts duration and completed state', backend.value('resume?film_id=' + histories[0]['film_id'])['position'] == 72)
            check('Invalid progress is rejected', backend.call('progress', {'history_id': histories[0]['id'], 'evidence': 'native_time_advanced', 'position': 300, 'duration': 200})[0] == 400)
            backend.value('settings', {'skip_marked_ads': False, 'smart_source_switch': False})
            sources = backend.value('sources')
            check('Expanded source registry remains present', len(sources['providers']) >= 47)
            ids = [p['id'] for p in sources['providers']][:2]
            backend.value('settings', {'enabled_providers': {ids[0]: False, ids[1]: False}})
            backend.value('settings', {'enabled_providers': {ids[0]: True}})
            toggles = {p['id']: p['enabled'] for p in backend.value('sources')['providers']}
            check('Provider toggle patches retain unrelated switches', toggles[ids[0]] is True and toggles[ids[1]] is False)
            sources = backend.value('sources/add', {'name': 'Transport fixture', 'url': 'https://example.com/yingxia-transport/api.php/provide/vod'})
            custom = next(p for p in sources['providers'] if p['name'] == 'Transport fixture')
            sources = backend.value('sources/edit', {'id': custom['id'], 'name': 'Edited fixture', 'url': ''})
            edited = next(p for p in sources['providers'] if p['id'] == custom['id'])
            check('Custom source editing keeps an omitted endpoint', edited['name'] == 'Edited fixture' and edited['endpoint'] == custom['endpoint'])
            check('Private network source input remains rejected', backend.call('sources/add', {'name': 'Invalid private endpoint', 'url': 'http://127.0.0.1/api'})[0] >= 400)
            sources = backend.value('sources/remove', {'id': custom['id']})
            check('Custom source removal persists', all(p['id'] != custom['id'] for p in sources['providers']))
            # All providers disabled means this job cannot contact a catalog or
            # media origin. Its only available film is the seeded official link.
            backend.value('settings', {'enabled_providers': {p['id']: False for p in sources['providers']}, 'network_consent': True, 'auto_update_sources': False})
            check('Disabled source-switch setting is enforced', backend.call('playback/alternatives', {'film_id': histories[0]['film_id'], 'group_key': 'official', 'episode_key': 'page'})[0] == 400)
            backend.value('settings', {'smart_source_switch': True})
            job = backend.value('playback/alternatives', {'film_id': histories[0]['film_id'], 'group_key': 'official', 'episode_key': 'page', 'position': 72})
            deadline = time.monotonic() + 5
            while job['phase'] not in ('done', 'failed', 'cancelled') and time.monotonic() < deadline:
                time.sleep(.05)
                job = backend.value('playback/alternatives/status?id=' + job['id'])
            check('Bounded alternative lookup returns no mismatched episode', job['phase'] == 'done' and not job['candidates'])
            check('Alternative cancellation route remains available', backend.call('playback/alternatives/cancel', {'id': job['id']})[0] == 200)
            backend.value('settings', {'smart_source_switch': False})
            if args.online:
                live = backend.value('search/suggest?q=xiaozhupeiqi')
                report['observations'].append({'public_title_query': 'xiaozhupeiqi', 'scope': live['scope'], 'titles': [item['title'] for item in live['items']]})
                check('Production client obtains unseen Chinese titles from public suggestions', live['scope'] == 'local_and_public_suggestions' and any('小猪佩奇' in item['title'] for item in live['items']))
            backend.value('settings', {'network_consent': False})
            backend.value('clear-history', {})
            backend.value('progress', {'history_id': histories[0]['id'], 'evidence': 'native_time_advanced', 'position': 80, 'duration': 200, 'completed': False})
            cleared = backend.value('resume?film_id=' + histories[0]['film_id'])
            check('Clear increments generation and stale progress cannot restore resume', cleared['found'] is False and cleared['history_generation'] > generation and not backend.value('library')['history'])
            check('First candidate backend exits normally', backend.close() == 0)
            backend = Backend(executable, directory)
            state = backend.value('state')
            library = backend.value('library')
            check('Playback settings persist after a production restart', state['skip_marked_ads'] is False and state['smart_source_switch'] is False and state['network_consent'] is False)
            check('History stays cleared and favorites survive restart', not library['history'] and len(library['favorites']) == 2)
            check('Provider switches survive production restart', all(not p['enabled'] for p in backend.value('sources')['providers']))
            check('Restarted candidate backend exits normally', backend.close() == 0)
    except Exception as error:
        report['failure'] = f'{type(error).__name__}: {error}'
        raise
    finally:
        if backend is not None and backend.process.poll() is None:
            try:
                backend.close()
            except Exception as error:
                report['cleanup_failure'] = type(error).__name__
        report['passed'] = sum(bool(item['passed']) for item in report['checks'])
        report['failed'] = sum(not item['passed'] for item in report['checks'])
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
        print(json.dumps({'passed': report['passed'], 'failed': report['failed'], 'failure': report.get('failure'), 'report': str(args.output)}, ensure_ascii=False))


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--backend', type=Path, required=True)
    parser.add_argument('--output', type=Path, default=ROOT / 'native_windows_client/evidence/v2_3/production_transport.json')
    parser.add_argument('--online', action='store_true', help='Also request public title suggestions; never film media')
    run(parser.parse_args())
