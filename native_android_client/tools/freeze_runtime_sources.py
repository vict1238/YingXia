"""Tie release build/QA to frozen Android runtime source hashes."""
from pathlib import Path
import hashlib
import json
import sys

CLIENT=Path(__file__).resolve().parents[1]
ROOT=CLIENT.parent
OUTPUT=CLIENT/'evidence/android/runtime_source_manifest.json'
SKIP={'build','.gradle','.cxx','.dart_tool','ephemeral','.git','evidence','__pycache__','.kotlin'}
def digest(path):
    with path.open('rb') as stream: return hashlib.file_digest(stream,'sha256').hexdigest()
def entries():
    paths=[]
    for directory in [CLIENT/'lib',CLIENT/'assets',CLIENT/'vendor/media_kit_video',CLIENT/'android',ROOT/'native_android_core']:
        for path in directory.rglob('*'):
            if not path.is_file(): continue
            parts=path.relative_to(directory).parts
            if any(part in SKIP for part in parts): continue
            if path.name in {'local.properties','key.properties'} or path.suffix in {'.iml','.log','.zip','.jar','.aar','.exe','.test','.jks','.keystore'}: continue
            paths.append(path)
    paths += [CLIENT/'pubspec.yaml',CLIENT/'pubspec.lock',CLIENT/'android/app/libs/yingxia_core.aar']
    return [{'path':p.relative_to(ROOT).as_posix(),'bytes':p.stat().st_size,'sha256':digest(p)} for p in sorted(set(paths))]
def main():
    current=entries()
    if '--check' in sys.argv:
        saved=json.loads(OUTPUT.read_text(encoding='utf-8'))
        assert saved['files']==current, 'Runtime source changed after freeze'
        print(f'Frozen runtime sources unchanged: {len(current)} files')
    else:
        OUTPUT.parent.mkdir(parents=True,exist_ok=True)
        OUTPUT.write_text(json.dumps({'schema':1,'scope':'Android production runtime; excludes tools, evidence and caches. Test fixture sources are retained for source correspondence.','files':current},ensure_ascii=False,indent=2),encoding='utf-8')
        print(f'Frozen runtime sources: {len(current)} files')
if __name__=='__main__': main()
