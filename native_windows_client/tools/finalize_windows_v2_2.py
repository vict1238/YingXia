"""Publish the exact Windows 2.2 candidate after its saved validation passes."""
from __future__ import annotations
import argparse
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parents[2]
CLIENT = ROOT / 'native_windows_client'
EVIDENCE = CLIENT / 'evidence/v2_2'
RELEASES = ROOT / 'releases'
CANDIDATE = CLIENT / 'build/candidates/YingXia_v2_2_candidate2.exe'
SHA = '4e91a204a6bd868fd788938efd34c01bce263cd4c518f713a63fe605116203f6'
BACKEND_SHA = '014ead0e0ec4f7e4929d35361be07fbbaee8b8862a5e9a1dab9f1a63ceb20803'
BUNDLE_SHA = 'b33a6ae3bba3005dcf38c9bee700f87e2cb2d6afe7acf9ce67958fbfb5b1b5c3'
NAME = 'YingXia_v2.2.0_Native_Windows_x64.exe'
VALIDATION = 'YingXia_v2.2.0_Native_Windows_VALIDATION.json'
README = 'YingXia_v2.2.0_Native_Windows_README_zh.md'

def digest(p):
    with p.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()

def load(p):
    return json.loads(p.read_text(encoding='utf-8-sig'))

def require(condition, message):
    if not condition:
        raise ValueError(message)

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--window-report', required=True, type=Path)
    parser.add_argument('--check-only', action='store_true')
    args = parser.parse_args()
    window_path = args.window_report.resolve()
    require(window_path.is_relative_to(EVIDENCE.resolve()), 'Window evidence must be isolated project evidence')
    window = load(window_path)
    require(window.get('sha256') == SHA and window.get('shipping_sfx_tested') is True, 'Window tests use different software')
    require(not window.get('failure') and not window.get('close_error'), 'Window report has an unresolved failure')
    checks = window.get('checks', [])
    require(len(checks) >= 4 and all(c.get('passed') is True for c in checks), 'Actual window coverage is incomplete')
    require(digest(CANDIDATE) == SHA and CANDIDATE.stat().st_size == 60509696, 'Candidate bytes changed')
    source = load(EVIDENCE / 'source_build.json')
    require(source.get('unchanged_during_build') is True, 'Build sources were not frozen')
    for entry in source['files']:
        require(digest(ROOT / entry['path']) == entry['sha256'], 'Source changed after build: ' + entry['path'])
    baseline = load(EVIDENCE / 'immutable_baseline.json')
    for entry in baseline:
        require(digest(RELEASES / entry['file']) == entry['sha256'], 'Older release changed')
    for entry in load(EVIDENCE / 'shared_core_baseline.json'):
        require(digest(ROOT / entry['path']) == entry['sha256'], 'Historical shared backend changed')
    dart = (CLIENT / 'evidence/controller/flutter_tests_2_2.log').read_text(encoding='utf-8-sig')
    require(re.search(r'\+30:\s+All tests passed!', dart), 'Dart tests are not complete')
    require('No issues found!' in (CLIENT / 'evidence/controller/analyze_2_2.log').read_text(encoding='utf-8-sig'), 'Flutter analysis failed')
    backend_report = load(ROOT / 'native_windows_core/evidence/backend_validation.json')
    require(backend_report['software']['sha256'] == BACKEND_SHA, 'Different tested backend')
    require(digest(ROOT / 'native_windows_core/build/yingxia_backend.exe') == BACKEND_SHA, 'Backend changed')
    for name in ['engine_tests.log', 'core_tests.log']:
        content = (ROOT / 'native_windows_core/evidence' / name).read_text(encoding='utf-8-sig')
        require(re.search(r'(?m)^PASS\s*$', content) and '--- FAIL:' not in content, 'Go regression failure')
    vet_log = (ROOT / 'native_windows_core/evidence/vet.log').read_text(encoding='utf-8-sig').strip()
    require(vet_log in {'', 'go vet ./...: PASS'}, 'Go vet failed')
    transport = load(EVIDENCE / 'production_transport.json')
    require(transport.get('passed') == 12 and not transport.get('failure') and transport['backend_sha256'] == BACKEND_SHA, 'Production API validation failed')
    screening = load(EVIDENCE / 'source_screening/screening_summary.json')
    require(screening['new_accepted_cms'] == 30 and screening['result_registered_sources'] == 47, 'Source screening count mismatch')
    require(digest(EVIDENCE / 'source_screening/accepted_providers.json') == screening['accepted_cms_sha256'], 'Reviewed sources changed')
    require(digest(ROOT / 'native_windows_core/internal/engine/reviewed_sources.json') == screening['accepted_cms_sha256'], 'Embedded sources differ from the reviewed list')
    manifest = load(CLIENT / 'launcher/payload/manifest.json')
    require(manifest['version'] == '2.2.0-native-windows' and manifest['bundle_sha256'] == BUNDLE_SHA, 'Bundle identity mismatch')
    archive = CLIENT / 'launcher/payload/bundle.zip'
    require(digest(archive) == BUNDLE_SHA and len(manifest['files']) == 243, 'Bundle bytes changed')
    by_name = {r['path']: r for r in manifest['files']}
    require(by_name['yingxia_backend.exe']['sha256'] == BACKEND_SHA, 'Wrong embedded backend')
    require(by_name['media_kit_video_plugin.dll']['sha256'] == 'd740f1a4c311827191e6a21957b41c7fb57d1778cf6a83037271a401b7ac8dc3', 'Renderer reuse identity mismatch')
    with zipfile.ZipFile(archive) as z:
        require(z.testzip() is None and set(z.namelist()) == set(by_name), 'Bundle CRC or entry-set failure')
        for name, entry in by_name.items():
            require(hashlib.sha256(z.read(name)).hexdigest() == entry['sha256'], 'Runtime entry changed: ' + name)
    report = {
        'version': '2.2.0-native-windows', 'generated_utc': datetime.now(timezone.utc).isoformat(),
        'software': [{'file': NAME, 'size': CANDIDATE.stat().st_size, 'sha256': SHA}],
        'tested_candidate': CANDIDATE.relative_to(ROOT).as_posix(),
        'delivery_method': 'Copy exact tested bytes; no rebuild after actual window validation',
        'source_registry': {'total': 47, 'new_cms': 30, 'kazumi': 16, 'legacy': 1, 'rule_api': 9},
        'source_screening': screening,
        'window_validation': {'path': window_path.relative_to(ROOT).as_posix(), 'sha256': digest(window_path), 'report': window},
        'production_transport': transport, 'backend': backend_report,
        'dart_validation': {'tests_passed': 30, 'analysis': 'No issues found'},
        'compiled_source_freeze': {'files': len(source['files']), 'unchanged_during_build': True, 'live_hashes_verified': True},
        'embedded_bundle': {'sha256': BUNDLE_SHA, 'files': len(by_name), 'crc_and_all_file_sha256_verified': True},
        'renderer_reuse': load(EVIDENCE / 'renderer_reuse.json'),
        'runtime_scope': {'film_media_playback_repeated_in_2_2': False, 'integrated_gpu_benchmarked': False, 'metadata_only_live_requests': True},
        'immutable_previous_releases': baseline,
    }
    outputs = [NAME, NAME + '.sha256.txt', VALIDATION, README]
    require(all(not (RELEASES / n).exists() for n in outputs), 'Preserve existing release files')
    if args.check_only:
        print(json.dumps({'all_delivery_gates_passed': True, 'actual_window_checks': len(checks), 'would_create': outputs}, indent=2))
        return
    with tempfile.TemporaryDirectory(prefix='.YingXia_v2.2_', dir=RELEASES) as directory:
        temp = Path(directory)
        shutil.copyfile(CANDIDATE, temp / NAME)
        require(digest(temp / NAME) == SHA, 'Delivery copy differs')
        (temp / (NAME + '.sha256.txt')).write_text(SHA + '  ' + NAME + '\n', encoding='utf-8')
        (temp / VALIDATION).write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
        shutil.copyfile(CLIENT / 'README_zh.md', temp / README)
        for name in outputs:
            os.link(temp / name, RELEASES / name)
    print(json.dumps({'software': report['software'], 'created': outputs, 'actual_window_checks': len(checks)}, indent=2))

if __name__ == '__main__':
    main()
