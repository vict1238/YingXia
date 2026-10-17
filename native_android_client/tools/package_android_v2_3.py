"""Package Android 2.3 sources after final production APK validation.

No build, signing, publication, deletion, or overwrite is performed. The only
application binary allowed in the source archive is the verified production Go
AAR required by Gradle. Source fixtures remain source; fixture APK/AAR files and
compiled tests are excluded. Use --audit-sources before final reports exist.
"""
from __future__ import annotations

import argparse
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import re
import zipfile
import zlib

ROOT = Path(__file__).resolve().parents[2]
CLIENT = ROOT / "native_android_client"
CORE = ROOT / "native_android_core"
RELEASES = ROOT / "releases"
APK_NAME = "YingXia_v2.3.0_Android.apk"
PREFIX = "YingXia_native_android_2.3.0/"
STAMP = (2026, 10, 16, 0, 0, 0)
AAR = CLIENT / "android/app/libs/yingxia_core.aar"
WRAPPER = CLIENT / "android/gradle/wrapper/gradle-wrapper.jar"
EXCLUDED_DIRS = {
    ".dart_tool", ".gradle", ".cxx", ".git", ".idea", ".plugin_symlinks",
    ".tools", "build", "ephemeral", "node_modules", "__pycache__", "http",
    "cache", "userdata", "user_data", "keys", "payload", ".kotlin",
    "aar_production", "aar_fixture",
}
EXCLUDED_NAMES = {
    ".flutter-plugins", ".flutter-plugins-dependencies", "local.properties",
    "key.properties", "library.json", "settings.json", "video_quality.json",
    "bootstrap.json", "kazumi_current_pinned_registry.json",
    "session.json", "title_index.json", "source_registry.json", "iterate_player_v23.py",
    "workbook_extracted.json", "subscription_probes.json",
    "debug_current.png", "runtime_overview.log",
}
PRIVATE_SUFFIXES = {".jks", ".keystore", ".p12", ".pfx", ".pem", ".key"}
BINARY_SUFFIXES = {
    ".exe", ".dll", ".apk", ".aar", ".so", ".pdb", ".pyc", ".pyo", ".o",
    ".obj", ".res", ".syso", ".class", ".jar", ".test", ".bin", ".dex", ".dill",
}
ARCHIVE_SUFFIXES = {".zip", ".7z", ".xz", ".gz", ".tar"}
PINNED_SOURCE_ARCHIVES = {
    "libmpv-windows-build-f5a6f879.zip", "libmpv-android-build-fe8c3ac1.zip",
    "mpv-windows-652a1dd907.zip", "mpv-android-78d43740.zip", "ffmpeg-6.0.tar.xz",
}
EVIDENCE_SUFFIXES = {".json", ".jsonl", ".md", ".log", ".txt", ".png", ".glsl", ".xml"}
SENSITIVE_KEYS = {"cookie", "csrf", "authorization", "session_cookie", "password", "client_secret", "private_key"}


def digest(path: Path) -> str:
    with path.open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def load(path: Path):
    return json.loads(path.read_text(encoding="utf-8-sig"))


def linked(path: Path) -> bool:
    return path.is_symlink() or (hasattr(path, "is_junction") and path.is_junction())


def safe_name(name: str) -> None:
    path = PurePosixPath(name)
    if not name or "\\" in name or ":" in name or path.is_absolute() or ".." in path.parts:
        raise ValueError(f"Unsafe archive path: {name}")


def verify_aar() -> dict:
    record = load(CORE / "evidence/v2_3/android_core_build.json")
    actual = {"path": "native_android_client/android/app/libs/yingxia_core.aar",
              "size": AAR.stat().st_size, "sha256": digest(AAR)}
    if (record.get("native_fixture_tag") is not False or record.get("android_api") != 26
            or record.get("sha256", "").lower() != actual["sha256"]
            or record.get("bytes") != actual["size"]):
        raise ValueError("Client AAR is not the exact documented production API26 AAR")
    production = CORE / "build/yingxia_core.aar"
    if production.exists() and digest(production) != actual["sha256"]:
        raise ValueError("Core production AAR and client dependency differ")
    abis = {"jni/arm64-v8a/libgojni.so", "jni/armeabi-v7a/libgojni.so", "jni/x86_64/libgojni.so"}
    with zipfile.ZipFile(AAR) as archive:
        if archive.testzip() is not None:
            raise ValueError("Production AAR CRC failure")
        actual_abis = {name for name in archive.namelist() if name.startswith("jni/") and name.endswith(".so")}
        manifest = archive.read("AndroidManifest.xml").decode("utf-8")
        if actual_abis != abis or set(record.get("abis", [])) != abis or 'android:minSdkVersion="26"' not in manifest:
            raise ValueError("Production AAR ABI/manifest mismatch")
    return {**actual, "android_api": 26, "abis": sorted(abis), "native_fixture_tag": False}


def verify_license_archives() -> None:
    records = {row["file"]: row for row in load(CLIENT / "licenses/native/upstream-source-manifest.json")}
    for name in PINNED_SOURCE_ARCHIVES:
        record = records[name]
        path = CLIENT / "licenses/native" / name
        if record.get("status") != "saved" or path.stat().st_size != record["bytes"] or digest(path) != record["sha256"]:
            raise ValueError(f"Pinned upstream source archive changed: {name}")
    pub = load(CLIENT / "licenses/pub-license-manifest.json")
    if digest(CLIENT / "pubspec.lock") != pub["pubspec_lock_sha256"]:
        raise ValueError("Dart lock file differs from the license manifest")
    for row in pub["notices"]:
        if digest(CLIENT / row["file"]) != row["sha256"]:
            raise ValueError(f"Dart license changed: {row['file']}")
    for row in load(CLIENT / "licenses/fsr1/source-manifest.json")["files"]:
        path = CLIENT / row["path"]
        if path.stat().st_size != row["size"] or digest(path) != row["sha256"]:
            raise ValueError(f"FSR upstream source/license changed: {row['path']}")
    for row in load(CLIENT / "evidence/fsr_shader/shader_provenance.json")["execution_assets"]:
        if digest(CLIENT / "assets/shaders" / row["file"]) != row["sha256"]:
            raise ValueError(f"FSR execution shader changed: {row['file']}")
    for relative in ("licenses/go", "licenses/android"):
        base = CLIENT / relative
        name = "manifest.json" if relative.endswith("go") else "file_manifest.json"
        for row in load(base / name):
            if digest(base / row["path"]) != row["sha256"] or (base / row["path"]).stat().st_size != row["size"]:
                raise ValueError(f"License file manifest mismatch: {relative}/{row['path']}")
    module = "github.com/mozillazg/go-pinyin"
    if not re.search(r"(?m)^\s*github\.com/mozillazg/go-pinyin\s+v0\.21\.0\s*$", (CORE / "go.mod").read_text(encoding="utf-8")):
        raise ValueError("Pinyin module must remain pinned to v0.21.0")
    sums = (CORE / "go.sum").read_text(encoding="utf-8")
    for suffix in ("", "/go.mod"):
        if not re.search(r"(?m)^" + re.escape(module + " v0.21.0" + suffix) + r" h1:[A-Za-z0-9+/=]+$", sums):
            raise ValueError("Pinyin dependency checksum missing")
    go_licenses = CLIENT / "licenses/go"
    if (CORE / "vendor/modules.txt").read_bytes() != (go_licenses / "modules.txt").read_bytes():
        raise ValueError("Vendored modules and shipped Go license versions differ")
    if (CORE / "vendor" / module / "LICENSE").read_bytes() != (go_licenses / module / "LICENSE").read_bytes():
        raise ValueError("Pinned pinyin MIT license is missing or changed")


def evidence_privacy(path: Path) -> None:
    if path.suffix.lower() not in EVIDENCE_SUFFIXES - {".png"}:
        return
    content = path.read_text(encoding="utf-8-sig")
    if re.search(r"-----BEGIN (?:[A-Z ]+ )?PRIVATE KEY-----", content):
        raise ValueError(f"Private key in evidence: {path}")
    if re.search(r"(?im)^\s*(?:cookie|authorization|x-yingxia-csrf)\s*[:=]\s*\S", content):
        raise ValueError(f"Credential header in evidence: {path}")
    if re.search(r"yx_session_\d+=[a-f0-9]{64}|(?i:[?&](?:access_token|api_key|password|client_secret)=)(?!\*|%2A)[^\s&\"']+", content):
        raise ValueError(f"Session or URL credential in evidence: {path}")

    def inspect(value):
        if isinstance(value, dict):
            for key, item in value.items():
                if key.lower() in SENSITIVE_KEYS and item not in (None, "", False):
                    raise ValueError(f"Credential field in evidence: {path}: {key}")
                inspect(item)
        elif isinstance(value, list):
            for item in value:
                inspect(item)

    if path.suffix.lower() == ".json":
        inspect(json.loads(content))
    elif path.suffix.lower() == ".jsonl":
        for line in content.splitlines():
            if line.strip():
                inspect(json.loads(line))


def walk(base: Path, *, evidence=False, licenses=False):
    if not base.is_dir() or linked(base):
        raise ValueError(f"Missing or linked source directory: {base}")
    for directory, dirs, files in os.walk(base, followlinks=False):
        parent = Path(directory)
        dirs[:] = sorted(name for name in dirs if name not in EXCLUDED_DIRS
                         and not name.startswith("source_extract_check")
                         and parent / name != CLIENT / "evidence/android/runtime")
        for name in dirs:
            if linked(parent / name):
                raise ValueError(f"Linked source directory: {parent / name}")
        for name in sorted(files):
            path = parent / name
            suffix = path.suffix.lower()
            in_evidence = evidence or path.is_relative_to(CORE / "evidence")
            if name in EXCLUDED_NAMES or name.endswith(".iml") or suffix in PRIVATE_SUFFIXES:
                continue
            if suffix == ".properties" and ("key" in name.lower() or "signing" in name.lower()):
                continue
            if linked(path) or not path.resolve().is_relative_to(ROOT):
                raise ValueError(f"Linked or escaped source file: {path}")
            if suffix in BINARY_SUFFIXES and path not in {AAR, WRAPPER}:
                continue
            if suffix in ARCHIVE_SUFFIXES:
                if not (licenses and path.parent == CLIENT / "licenses/native" and name in PINNED_SOURCE_ARCHIVES):
                    continue
            if in_evidence and suffix not in EVIDENCE_SUFFIXES:
                continue
            if suffix == ".log" and not in_evidence:
                continue
            with path.open("rb") as stream:
                magic = stream.read(4)
            if magic[:2] == b"MZ" or magic == b"\x7fELF":
                raise ValueError(f"Compiled executable cannot enter source package: {path}")
            if in_evidence:
                evidence_privacy(path)
            yield path


def source_entries(validation: Path | None = None, readme: Path | None = None):
    entries = []
    for name in ("analysis_options.yaml", "pubspec.yaml", "pubspec.lock", "README_zh.md", "THIRD_PARTY_NOTICES.md"):
        entries.append((CLIENT / name, PREFIX + "native_android_client/" + name))
    if (CLIENT / ".metadata").is_file():
        entries.append((CLIENT / ".metadata", PREFIX + "native_android_client/.metadata"))
    for name in ("lib", "test", "assets", "android", "vendor", "tools", "tool", "licenses", "evidence"):
        entries.extend((path, PREFIX + path.relative_to(ROOT).as_posix())
                       for path in walk(CLIENT / name, evidence=name == "evidence", licenses=name == "licenses"))
    for path in sorted(CORE.iterdir()):
        if path.is_file() and (path.suffix == ".go" or path.name in {"go.mod", "go.sum", "README_zh.md", "LICENSE.txt", "build_android_aar.ps1", "verify_android_tests.ps1"}):
            entries.append((path, PREFIX + path.relative_to(ROOT).as_posix()))
    for name in ("cmd", "internal", "vendor", "evidence"):
        entries.extend((path, PREFIX + path.relative_to(ROOT).as_posix())
                       for path in walk(CORE / name, evidence=name == "evidence"))
    for name in ("LICENSE.txt", "LICENSE_GO.txt", "THIRD_PARTY_NOTICES.md"):
        entries.append((ROOT / "current_source" / name, PREFIX + "current_source/" + name))
    for name in ("LICENSE.txt", "LICENSE_GO.txt"):
        entries.append((ROOT / "current_source" / name, PREFIX + name))
    if validation is not None:
        evidence_privacy(validation)
        entries.append((validation, PREFIX + "YingXia_v2.3.0_Android_VALIDATION.json"))
    if readme is not None:
        entries.append((readme, PREFIX + "RELEASE_README_zh.md"))
    seen = set()
    for path, name in entries:
        safe_name(name)
        if name in seen or not path.is_file() or linked(path):
            raise ValueError(f"Duplicate, missing or linked input: {name}")
        seen.add(name)
    if AAR not in {path for path, _ in entries} or WRAPPER not in {path for path, _ in entries}:
        raise ValueError("Source package is missing its production AAR or Gradle wrapper")
    return sorted(entries, key=lambda item: item[1])


def verified_software(validation: Path) -> dict:
    matches = [row for row in load(validation)["software"] if row.get("file") == APK_NAME]
    if len(matches) != 1:
        raise ValueError("Validation must identify exactly one final production Android 2.3 APK")
    apk = RELEASES / APK_NAME
    actual = {"file": APK_NAME, "size": apk.stat().st_size, "sha256": digest(apk)}
    if matches[0].get("size", matches[0].get("bytes")) != actual["size"] or matches[0].get("sha256", "").lower() != actual["sha256"]:
        raise ValueError("Final APK differs from its validation report")
    runtime = load(CLIENT / "evidence/v2_3/android_runtime/production_checks.json")
    if runtime.get("apk_sha256", "").lower() != actual["sha256"]:
        raise ValueError("Final production runtime checks do not identify the final APK SHA-256")
    if not runtime.get("checks") or any(row.get("status") != "passed" for row in runtime["checks"]):
        raise ValueError("Final production runtime checks are incomplete or failed")
    return actual


def file_record(path: Path, name: str) -> dict:
    hasher, crc, size = hashlib.sha256(), 0, 0
    with path.open("rb") as stream:
        while block := stream.read(1024 * 1024):
            hasher.update(block)
            crc = zlib.crc32(block, crc)
            size += len(block)
    return {"path": name, "size": size, "sha256": hasher.hexdigest(), "crc32": f"{crc:08x}"}


def write_zip(destination: Path, entries, aar: dict) -> dict:
    checksum = destination.with_suffix(destination.suffix + ".sha256.txt")
    for path in (destination, checksum):
        if path.exists():
            raise FileExistsError(f"Preserve existing release artifact: {path}")
    records = [file_record(path, name) for path, name in entries]
    aar_name = PREFIX + aar["path"]
    packaged_aar = next((row for row in records if row["path"] == aar_name), None)
    if (packaged_aar is None or packaged_aar["size"] != aar["size"]
            or packaged_aar["sha256"] != aar["sha256"]):
        raise ValueError("AAR changed after validation; refusing a fixture or unverified AAR snapshot")
    with zipfile.ZipFile(destination, "x", compression=zipfile.ZIP_DEFLATED, compresslevel=6, allowZip64=True) as archive:
        for (path, name), record in zip(entries, records, strict=True):
            data = path.read_bytes()
            if len(data) != record["size"] or hashlib.sha256(data).hexdigest() != record["sha256"]:
                raise ValueError(f"Source changed while packaging: {name}")
            info = zipfile.ZipInfo(name, STAMP)
            info.compress_type = zipfile.ZIP_DEFLATED
            archive.writestr(info, data)
        manifest = {"schema": 1, "version": "2.3.0-native-android", "production_aar": aar,
                    "files": records, "manifest_scope": "Every file entry except MANIFEST.json itself"}
        archive.writestr(zipfile.ZipInfo(PREFIX + "MANIFEST.json", STAMP), json.dumps(manifest, ensure_ascii=False, indent=2).encode("utf-8"))
    with zipfile.ZipFile(destination) as archive:
        if archive.testzip() is not None or len(archive.namelist()) != len(records) + 1:
            raise ValueError("Source ZIP CRC or entry-count failure")
        if set(archive.namelist()) != {row["path"] for row in records} | {PREFIX + "MANIFEST.json"}:
            raise ValueError("Source ZIP has unmanifested entries")
        for row in records:
            info = archive.getinfo(row["path"])
            data = archive.read(row["path"])
            if (hashlib.sha256(data).hexdigest() != row["sha256"] or info.file_size != row["size"]
                    or f"{info.CRC:08x}" != row["crc32"]):
                raise ValueError(f"Decompressed SHA/size/CRC mismatch: {row['path']}")
        archived_aar = archive.read(aar_name)
        if len(archived_aar) != aar["size"] or hashlib.sha256(archived_aar).hexdigest() != aar["sha256"]:
            raise ValueError("Archived AAR differs from the verified production AAR")
    sha = digest(destination)
    with checksum.open("x", encoding="utf-8") as stream:
        stream.write(f"{sha}  {destination.name}\n")
    return {"file": destination.name, "size": destination.stat().st_size, "sha256": sha,
            "verified_files": len(records), "crc_verified": True,
            "all_decompressed_file_sha256_verified": True, "manifest_crc32_verified": True}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--validation", type=Path, default=RELEASES / "YingXia_v2.3.0_Android_VALIDATION.json")
    parser.add_argument("--readme", type=Path, default=RELEASES / "YingXia_v2.3.0_Android_README_zh.md")
    parser.add_argument("--source-zip", type=Path, default=RELEASES / "YingXia_v2.3.0_Android_Source.zip")
    parser.add_argument("--report", type=Path, default=RELEASES / "YingXia_v2.3.0_Android_PACKAGING.json")
    parser.add_argument("--dry-run", action="store_true", help="Verify final software and source inputs, without creating an archive")
    parser.add_argument("--audit-sources", action="store_true", help="Check source/AAR/license/privacy inputs before final release reports exist; write nothing")
    args = parser.parse_args()
    aar = verify_aar()
    verify_license_archives()
    if args.audit_sources:
        entries = source_entries()
        print(json.dumps({"production_aar": aar, "source_files": len(entries),
                          "source_bytes": sum(path.stat().st_size for path, _ in entries), "archive_written": False}, indent=2))
        return
    software = verified_software(args.validation)
    entries = source_entries(args.validation, args.readme)
    if args.dry_run:
        print(json.dumps({"software": software, "production_aar": aar, "source_files": len(entries),
                          "archive_paths": [name for _, name in entries], "archive_written": False}, indent=2))
        return
    if args.report.exists():
        raise FileExistsError(f"Preserve existing packaging report: {args.report}")
    archive = write_zip(args.source_zip, entries, aar)
    if verified_software(args.validation) != software or verify_aar() != aar:
        raise ValueError("Production APK/AAR changed during packaging")
    report = {"software": [software], "production_aar": aar, "source_archive": archive,
              "scope": ["native_android_client", "native_android_core", "required current_source license texts", "final release readme and validation"],
              "excluded": sorted(EXCLUDED_DIRS | EXCLUDED_NAMES | PRIVATE_SUFFIXES | (BINARY_SUFFIXES - {".aar", ".jar"})),
              "generated_evidence_excluded": ["source_extract_check*", "aar_production", "aar_fixture", "native_android_client/evidence/android/runtime"],
              "binary_allowlist": ["verified production client AAR", "Gradle wrapper JAR", "pinned upstream source archives"]}
    with args.report.open("x", encoding="utf-8") as stream:
        json.dump(report, stream, ensure_ascii=False, indent=2)
        stream.write("\n")
    print(json.dumps(report, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
