"""Inspect a built APK without rebuilding or exposing private signing material."""
from __future__ import annotations
import argparse
import hashlib
import json
import os
import re
import subprocess
import tempfile
import zipfile
from pathlib import Path

CLIENT = Path(__file__).resolve().parents[1]
ROOT = CLIENT.parent
CERTIFICATE = '6f3e4d81152657ba0b0fa9304763d0f47035e56a74161d8a9d27d3a12cc6085c'
ABIS = {'arm64-v8a', 'armeabi-v7a', 'x86_64'}

def sha(data): return hashlib.sha256(data).hexdigest()

def run(*args):
    return subprocess.run([str(x) for x in args], capture_output=True, check=True).stdout.decode('utf-8', 'replace')

def verify(apk: Path, fixture=False):
    sdk = Path(os.environ.get('ANDROID_HOME', ROOT / '.tools/native/android_sdk'))
    jdk = Path(os.environ.get('JAVA_HOME', ROOT / '.tools/native/jdk/jdk-17.0.20.1+1'))
    badging = run(sdk/'build-tools/36.0.0/aapt2.exe', 'dump', 'badging', apk)
    certs = run(jdk/'bin/java.exe', '-jar', sdk/'build-tools/36.0.0/lib/apksigner.jar', 'verify', '--verbose', '--print-certs', apk)
    certificate = re.search(r'certificate SHA-256 digest: ([a-f0-9]{64})', certs).group(1)
    assert certificate == CERTIFICATE, 'APK has a different signing certificate from Android 2.0'
    assert 'Verified using v2 scheme (APK Signature Scheme v2): true' in certs
    package = 'com.yingxia.yingxia_native' + ('.fixture' if fixture else '')
    assert f"name='{package}' versionCode='20300' versionName='2.3.0'" in badging
    assert "minSdkVersion:'26'" in badging and "targetSdkVersion:'36'" in badging
    assert 'application-debuggable' not in badging
    aar = ROOT/'native_android_core/build'/('yingxia_core_fixture.aar' if fixture else 'yingxia_core.aar')
    record = {'apk': apk.relative_to(ROOT).as_posix(), 'bytes': apk.stat().st_size, 'sha256': sha(apk.read_bytes()), 'package': package, 'version_name': '2.3.0', 'version_code': 20300, 'min_sdk': 26, 'target_sdk': 36, 'release_aot': True, 'debuggable': False, 'certificate_sha256': certificate, 'fixture': fixture, 'aar_sha256': sha(aar.read_bytes()), 'go_jni_matches_aar_after_ndk_strip': True, 'jni_normalization': 'NDK 28.2.13676358 llvm-strip --strip-unneeded', 'permissions': re.findall(r"uses-permission: name='([^']+)'", badging), 'libraries': []}
    with zipfile.ZipFile(apk) as archive, zipfile.ZipFile(aar) as backend:
        assert archive.testzip() is None
        assert {p.split('/')[1] for p in archive.namelist() if p.startswith('lib/')} == ABIS
        for abi in sorted(ABIS):
            go = archive.read(f'lib/{abi}/libgojni.so')
            original = backend.read(f'jni/{abi}/libgojni.so')
            # AGP strips even an already ldflags=-s Go ELF. Reproduce that
            # exact, pinned NDK transform rather than comparing unlike bytes.
            strip = sdk/'ndk/28.2.13676358/toolchains/llvm/prebuilt/windows-x86_64/bin/llvm-strip.exe'
            with tempfile.TemporaryDirectory(prefix='yingxia_jni_verify_') as temporary:
                source = Path(temporary)/'aar.so'
                normalized = Path(temporary)/'stripped.so'
                source.write_bytes(original)
                run(strip,'--strip-unneeded','-o',normalized,source)
                assert go == normalized.read_bytes(), 'APK Go JNI differs from the selected AAR after the AGP NDK transform'
            for name in ['libgojni.so', 'libapp.so', 'libflutter.so', 'libmpv.so']:
                data = archive.read(f'lib/{abi}/{name}')
                machine = int.from_bytes(data[18:20], 'little')
                assert data[:4] == b'\x7fELF' and machine == {'armeabi-v7a': 40, 'arm64-v8a': 183, 'x86_64': 62}[abi]
                record['libraries'].append({'abi': abi, 'name': name, 'bytes': len(data), 'sha256': sha(data), 'elf_machine': machine})
        for name in ['fsr1_light.glsl', 'fsr1_enhanced.glsl']:
            data = archive.read(f'assets/flutter_assets/assets/shaders/{name}')
            assert data == (CLIENT/'assets/shaders'/name).read_bytes()
        assert 'assets/flutter_assets/NOTICES.Z' in archive.namelist()
    return record

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('apk', type=Path)
    parser.add_argument('--fixture', action='store_true')
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    record = verify(args.apk.resolve(), args.fixture)
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(record, ensure_ascii=False, indent=2), encoding='utf-8')
    print(json.dumps({k: v for k, v in record.items() if k != 'libraries'}, ensure_ascii=False))

if __name__ == '__main__': main()
