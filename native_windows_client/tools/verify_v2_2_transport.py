"""Exercise the production Windows 2.2 backend with isolated persistent data.

Only three public catalog metadata endpoints are measured. No film media is
requested, and private session descriptors are never saved in the report.
"""
from __future__ import annotations
import hashlib
import json
import subprocess
import tempfile
import time
import urllib.error
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
EXE = ROOT / 'native_windows_core/build/yingxia_backend.exe'
OUT = ROOT / 'native_windows_client/evidence/v2_2/production_transport.json'
report = {'backend_sha256': hashlib.sha256(EXE.read_bytes()).hexdigest(),
          'real_third_party_media_playback_tested': False,
          'live_network_scope': 'Three public CMS metadata operations only',
          'isolated_data': True, 'checks': [], 'observations': []}

def check(name, condition):
    if not condition:
        raise AssertionError(name)
    report['checks'].append({'name': name, 'passed': True})

class Backend:
    def __init__(self, directory):
        self.process = subprocess.Popen([str(EXE), '--data-dir', str(directory)],
            stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL,
            creationflags=subprocess.CREATE_NO_WINDOW)
        self.descriptor = json.loads(self.process.stdout.readline())
        if self.descriptor.get('error'):
            raise RuntimeError(self.descriptor['error'])
        assert self.descriptor['origin'].startswith('http://127.0.0.1:')
    def call(self, path, data=None, auth=True, csrf=True):
        headers = {'Origin': self.descriptor['origin']}
        if auth:
            headers['Cookie'] = self.descriptor['cookie']
        if data is not None:
            headers['Content-Type'] = 'application/json'
            if csrf:
                headers['X-YingXia-CSRF'] = self.descriptor['csrf']
        request = urllib.request.Request(self.descriptor['origin'] + '/api/' + path,
            data=json.dumps(data).encode() if data is not None else None,
            headers=headers)
        try:
            with urllib.request.urlopen(request, timeout=20) as response:
                status, body = response.status, json.load(response)
        except urllib.error.HTTPError as error:
            status, body = error.code, json.load(error)
        return status, body
    def value(self, path, data=None):
        status, body = self.call(path, data)
        if status != 200 or not body.get('ok'):
            raise AssertionError(f'{path}: {status}, {body.get("message")}')
        return body['data']
    def close(self):
        if self.process.poll() is None:
            try:
                self.call('quit', {})
            finally:
                self.process.stdin.close()
                self.process.wait(timeout=10)
        return self.process.returncode

backend = None
try:
    with tempfile.TemporaryDirectory(prefix='yingxia-v2-2-transport-') as directory:
        backend = Backend(directory)
        sources = backend.value('sources')
        check('Production backend seeds the expanded registry', len(sources['providers']) >= 47)
        check('Private source GET and mutation endpoints retain session and CSRF checks',
              backend.call('sources', auth=False)[0] == 403 and
              backend.call('settings', {'network_consent': True}, csrf=False)[0] == 403)
        ids = [p['id'] for p in sources['providers']][:2]
        backend.value('settings', {'enabled_providers': {ids[0]: False, ids[1]: False}})
        backend.value('settings', {'enabled_providers': {ids[0]: True}})
        toggles = {p['id']: p['enabled'] for p in backend.value('sources')['providers']}
        check('Enabling one provider preserves a different disabled provider',
              toggles[ids[0]] is True and toggles[ids[1]] is False)
        sources = backend.value('sources/add', {'name': 'Transport fixture',
            'url': 'https://example.com/yingxia-test/api.php/provide/vod'})
        custom = next(p for p in sources['providers'] if p['name'] == 'Transport fixture')
        sources = backend.value('sources/edit', {'id': custom['id'], 'name': 'Edited fixture', 'url': ''})
        edited = next(p for p in sources['providers'] if p['id'] == custom['id'])
        check('Custom provider editing retains its URL when the URL field is left blank',
              edited['name'] == 'Edited fixture' and edited['endpoint'] == custom['endpoint'])
        rejected = backend.call('sources/add', {'name': 'Invalid private endpoint', 'url': 'http://127.0.0.1/api'})
        check('Private network source input remains rejected', rejected[0] >= 400)
        sources = backend.value('sources/remove', {'id': custom['id']})
        check('Custom provider removal updates the registry', all(p['id'] != custom['id'] for p in sources['providers']))
        curated = json.loads((ROOT / 'native_windows_client/evidence/v2_2/source_screening/accepted_providers.json').read_text(encoding='utf-8-sig'))
        if isinstance(curated, dict):
            curated = curated['providers']
        selected = [p['id'] for p in curated[:3]]
        backend.value('settings', {'auto_update_sources': False, 'network_consent': True})
        started = time.monotonic()
        job = backend.value('sources/speed/start', {'ids': selected})
        check('Live catalog speed measurement starts asynchronously',
              time.monotonic() - started < 2 and job['total'] == len(selected))
        while job['state'] == 'running' and time.monotonic() - started < 30:
            time.sleep(.2)
            job = backend.value('sources/speed/status?id=' + job['id'])
        check('Live speed measurement completes with exact progress',
              job['state'] == 'done' and job['completed'] == len(selected) and job['progress'] == 100)
        successful = [r for r in job['results'] if r['status'] == 'available']
        check('New providers return actual parseable catalog records in the production Go client',
              bool(successful) and all(r['catalog_count'] > 0 and not r['playback_verified'] for r in successful))
        report['observations'].append({'live_catalog_results': job['results']})
        check('First backend exits normally', backend.close() == 0)
        backend = Backend(directory)
        sources = backend.value('sources')
        after = {p['id']: p for p in sources['providers']}
        check('Provider overrides and completed speed results persist across production backend restart',
              after[ids[1]]['enabled'] is False and
              all(after[r['id']]['health']['checked_at'] == r['checked_at'] for r in job['results']))
        check('Restarted backend exits normally', backend.close() == 0)
except Exception as error:
    report['failure'] = str(error)
    raise
finally:
    if backend is not None and backend.process.poll() is None:
        backend.close()
    report['passed'] = sum(c['passed'] for c in report['checks'])
    OUT.parent.mkdir(parents=True, exist_ok=True)
    OUT.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': report['passed'], 'failure': report.get('failure'), 'report': str(OUT)}, ensure_ascii=False))
