"""Record and compare the source bytes used for the Windows 2.2 build."""
import hashlib
import json
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
OUT = ROOT / 'native_windows_client/evidence/v2_2/source_build.json'

def snapshot():
    paths = set()
    client = ROOT / 'native_windows_client'
    for name in ['lib', 'assets', 'vendor', 'windows']:
        for p in (client / name).rglob('*'):
            if p.is_file() and not any(s in p.parts for s in ['ephemeral', '.plugin_symlinks', 'build', '.dart_tool', '__pycache__']):
                paths.add(p)
    for p in (client / 'launcher').iterdir():
        if p.is_file() and p.suffix in ['.go', '.rc', '.manifest', '.syso', '.mod', '.ps1']:
            paths.add(p)
    for name in ['pubspec.yaml', 'pubspec.lock', 'build_windows_native.ps1', 'THIRD_PARTY_NOTICES.md']:
        paths.add(client / name)
    core = ROOT / 'native_windows_core'
    for p in core.rglob('*'):
        if p.is_file() and not any(s in p.relative_to(core).parts for s in ['evidence', 'build', '__pycache__']) and (p.suffix in ['.go', '.mod', '.sum', '.json', '.c', '.h', '.s', '.syso'] or p.name == 'LICENSE.txt'):
            paths.add(p)
    return [{'path': p.relative_to(ROOT).as_posix(), 'sha256': hashlib.sha256(p.read_bytes()).hexdigest()} for p in sorted(paths)]

files = snapshot()
if '--finish' in sys.argv:
    data = json.loads(OUT.read_text(encoding='utf-8'))
    data['unchanged_during_build'] = data['files'] == files
    if not data['unchanged_during_build']:
        raise RuntimeError('Source bytes changed during the build')
else:
    if OUT.exists():
        raise FileExistsError('Preserve existing build snapshot')
    data = {'version': '2.2.0-native-windows', 'files': files, 'unchanged_during_build': None}
OUT.write_text(json.dumps(data, indent=2) + '\n', encoding='utf-8')
print(json.dumps({'files': len(files), 'unchanged_during_build': data['unchanged_during_build']}))
