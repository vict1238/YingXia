# SPDX-License-Identifier: GPL-3.0-or-later
"""Verify shipping Go JNI and release fixture renderer correspondence."""
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import sys
import zipfile

ROOT = Path(__file__).resolve().parents[2]
EVIDENCE = ROOT / "native_client/evidence/android"
BUILD = ROOT / "native_core/build/evidence_jni"
APK = ROOT / "releases/YingXia_v2.0.0_Android.apk"
AAR = ROOT / "native_core/build/yingxia_core.aar"
FIXTURE = ROOT / "native_client/build/test-fixture/YingXia_fixture_Android.apk"
STRIP = ROOT / ".tools/native/android_sdk/ndk/28.2.13676358/toolchains/llvm/prebuilt/windows-x86_64/bin/llvm-strip.exe"
GO = ROOT / ".tools/next/go/bin/go.exe"
ABIS = ("arm64-v8a", "armeabi-v7a", "x86_64")


def digest(data):
    return hashlib.sha256(data).hexdigest()


def save(name, value):
    (EVIDENCE / name).write_text(json.dumps(value, indent=2) + "\n", encoding="utf-8")


EVIDENCE.mkdir(parents=True, exist_ok=True)
BUILD.mkdir(parents=True, exist_ok=True)
rows = []
with zipfile.ZipFile(APK) as apk, zipfile.ZipFile(AAR) as aar:
    for abi in ABIS:
        original = aar.read(f"jni/{abi}/libgojni.so")
        packaged = apk.read(f"lib/{abi}/libgojni.so")
        directory = BUILD / abi
        directory.mkdir(parents=True, exist_ok=True)
        source = directory / "production_aar.so"
        normalized = directory / "production_stripped.so"
        source.write_bytes(original)
        shutil.copyfile(source, normalized)
        subprocess.run([str(STRIP), "--strip-unneeded", str(normalized)], check=True)
        normalized_hash = digest(normalized.read_bytes())
        assert normalized_hash == digest(packaged), f"JNI differs from production AAR: {abi}"
        info = subprocess.run([str(GO), "version", "-m", str(normalized)], check=True, capture_output=True, text=True).stdout
        assert "GOOS=android" in info and "native_fixture" not in info, f"Unexpected native build: {abi}"
        (EVIDENCE / f"production_go_build_info_{abi}.txt").write_text(info, encoding="utf-8")
        rows.append({"abi": abi, "apk_bytes": len(packaged), "aar_bytes": len(original),
                     "apk_jni_sha256": digest(packaged), "aar_jni_sha256": digest(original),
                     "same_bytes": original == packaged, "normalized_aar_jni_sha256": normalized_hash,
                     "matches_after_gradle_strip": True, "fixture_build_tag_absent": True})
save("production_go_jni_correspondence.json", {
    "apk_sha256": digest(APK.read_bytes()), "production_aar_sha256": digest(AAR.read_bytes()),
    "jni_correspondence": rows, "fixture_build_tag": False,
    "release_source_exclusions": "native_core/evidence/production_exclusions.json"})

if FIXTURE.exists() and "--production-only" not in sys.argv:
    comparison = []
    with zipfile.ZipFile(APK) as apk, zipfile.ZipFile(FIXTURE) as fixture:
        for abi in ABIS:
            for library in ("libapp.so", "libflutter.so", "libmediakitandroidhelper.so", "libmpv.so"):
                entry = f"lib/{abi}/{library}"
                production_hash, fixture_hash = digest(apk.read(entry)), digest(fixture.read(entry))
                assert production_hash == fixture_hash, f"Release renderer/AOT mismatch: {entry}"
                comparison.append({"entry": entry, "production_sha256": production_hash,
                                   "fixture_sha256": fixture_hash, "identical": True})
    production_manifest = json.loads((EVIDENCE / "apk_package.json").read_text(encoding="utf-8-sig"))
    fixture_manifest = json.loads((EVIDENCE / "fixture_apk_package.json").read_text(encoding="utf-8-sig"))
    assert not production_manifest["debuggable"] and not fixture_manifest["debuggable"]
    save("fixture_release_correspondence.json", {
        "production_apk_sha256": digest(APK.read_bytes()), "fixture_apk_sha256": digest(FIXTURE.read_bytes()),
        "both_nondebuggable_release_aot": True, "native_code_comparison": comparison})
print("Production JNI verified for 3 ABIs; requested release correspondence checks passed.")
