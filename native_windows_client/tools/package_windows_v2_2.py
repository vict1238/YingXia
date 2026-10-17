"""Package frozen Windows 2.2 sources after the final EXE validation report exists.

Run from any directory. The report must contain a ``software`` list with the
exact final EXE's file, size and SHA-256. This helper never builds or publishes
software, never replaces an existing archive, and excludes user/runtime data.
"""
from __future__ import annotations

import argparse
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import re
import zipfile

ROOT = Path(__file__).resolve().parents[2]
CLIENT = ROOT / "native_windows_client"
RELEASES = ROOT / "releases"
EXE_NAME = "YingXia_v2.2.0_Native_Windows_x64.exe"
PREFIX = "YingXia_native_windows_2.2.0/"
FIXED_DATE = (2026, 10, 15, 0, 0, 0)
EXCLUDED_DIRS = {
    ".dart_tool", ".gradle", ".cxx", ".plugin_symlinks", ".git", ".idea",
    "build", "ephemeral", "node_modules", "__pycache__", "userdata",
    "user_data", "payload",
}
EXCLUDED_NAMES = {
    ".flutter-plugins-dependencies", "local.properties", "key.properties",
    "library.json", "settings.json", "video_quality.json", "bootstrap.json",
    "bundle.zip",
}
PRIVATE_SUFFIXES = {".jks", ".keystore", ".p12", ".pfx", ".pem", ".key"}
BINARY_SUFFIXES = {".exe", ".dll", ".apk", ".aar", ".so", ".pdb", ".pyc", ".pyo", ".o", ".obj", ".res", ".syso"}
TEXT_SUFFIXES = {".json", ".jsonl", ".md", ".log", ".txt", ".yaml", ".yml", ".ps1", ".py", ".cjs", ".dart", ".go", ".h", ".cc", ".glsl"}
SENSITIVE_KEYS = {"cookie", "csrf", "authorization", "session_cookie", "password", "client_secret", "private_key"}
UPSTREAM_SOURCE_ARCHIVES = {
    "libmpv-windows-build-f5a6f879.zip", "libmpv-android-build-fe8c3ac1.zip",
    "mpv-windows-652a1dd907.zip", "mpv-android-78d43740.zip", "ffmpeg-6.0.tar.xz",
}


def digest(path: Path) -> str:
    with path.open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def safe_archive_name(name: str) -> None:
    path = PurePosixPath(name)
    if "\\" in name or path.is_absolute() or ".." in path.parts or ":" in name or not name:
        raise ValueError(f"Unsafe archive path: {name}")


def is_link(path: Path) -> bool:
    return path.is_symlink() or (hasattr(path, "is_junction") and path.is_junction())


def walk_sources(base: Path, *, license_tree: bool = False, evidence_tree: bool = False):
    """Walk only explicitly allowlisted roots, never directory links or caches."""
    if not base.is_dir() or is_link(base):
        raise ValueError(f"Missing or linked source root: {base}")
    archive_records = {}
    if license_tree:
        manifest = json.loads((base / "native/upstream-source-manifest.json").read_text(encoding="utf-8-sig"))
        archive_records = {record["file"]: record for record in manifest if record["file"] in UPSTREAM_SOURCE_ARCHIVES}
        if set(archive_records) != UPSTREAM_SOURCE_ARCHIVES:
            raise ValueError("Pinned upstream source archive manifest is incomplete")
        for name, record in archive_records.items():
            path = base / "native" / name
            if record.get("status") != "saved" or path.stat().st_size != record["bytes"] or digest(path) != record["sha256"]:
                raise ValueError(f"Pinned upstream license source archive changed: {name}")
    for directory, names, files in os.walk(base, followlinks=False):
        parent = Path(directory)
        names[:] = sorted(name for name in names if name not in EXCLUDED_DIRS and not (evidence_tree and name in {"runtime", "cache", "http"}) and not is_link(parent / name))
        for name in sorted(files):
            path = parent / name
            # This full upstream response is already represented by the pinned
            # production rules and its hash/report; omit the duplicate snapshot.
            if evidence_tree and name == "kazumi_current_pinned_registry.json":
                continue
            if is_link(path):
                raise ValueError(f"Linked file cannot enter release: {path}")
            if name in EXCLUDED_NAMES or name.endswith(".iml") or path.suffix.lower() in PRIVATE_SUFFIXES:
                continue
            # License archives are pinned upstream source snapshots, not app
            # binaries or a generated runtime payload. Preserve them verbatim.
            if path.suffix.lower() in BINARY_SUFFIXES:
                if path != CLIENT / "launcher/resource_windows_amd64.syso":
                    continue
            if path.suffix.lower() in {".zip", ".7z", ".xz", ".gz"} and not license_tree:
                continue
            if path.suffix.lower() in {".zip", ".7z", ".xz", ".gz"} and license_tree:
                if path.parent != base / "native" or path.name not in archive_records:
                    raise ValueError(f"Unapproved archive in license tree: {path}")
            if evidence_tree and path.suffix.lower() not in {".json", ".jsonl", ".md", ".log", ".png", ".txt", ".glsl"}:
                continue
            if path.suffix.lower() == ".log" and not evidence_tree:
                continue
            if path.suffix.lower() == ".properties" and "key" in name.lower():
                continue
            yield path


def check_evidence_privacy(path: Path) -> None:
    if path.suffix.lower() not in TEXT_SUFFIXES:
        return
    content = path.read_text(encoding="utf-8-sig", errors="strict")
    if re.search(r"-----BEGIN (?:[A-Z ]+ )?PRIVATE KEY-----", content):
        raise ValueError(f"Private key in evidence: {path}")
    if re.search(r"(?im)^\s*(?:cookie|authorization|x-yingxia-csrf)\s*[:=]\s*\S", content):
        raise ValueError(f"Credential header in evidence: {path}")

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


def source_entries(validation_path: Path, readme_path: Path) -> list[tuple[Path, str]]:
    entries = []
    top_files = (
        ".metadata", "analysis_options.yaml", "build_windows_native.ps1",
        "pubspec.yaml", "pubspec.lock", "README_zh.md", "THIRD_PARTY_NOTICES.md",
    )
    for name in top_files:
        entries.append((CLIENT / name, PREFIX + "native_windows_client/" + name))
    for name in ("lib", "test", "assets", "windows", "vendor", "launcher", "tools", "licenses", "evidence"):
        for path in walk_sources(CLIENT / name, license_tree=name == "licenses", evidence_tree=name == "evidence"):
            if name == "evidence":
                check_evidence_privacy(path)
            entries.append((path, PREFIX + path.relative_to(ROOT).as_posix()))
    core = ROOT / "native_windows_core"
    for path in sorted(core.iterdir()):
        if path.is_file() and (path.suffix == ".go" or path.name in {"go.mod", "go.sum", "README_zh.md"}):
            entries.append((path, PREFIX + path.relative_to(ROOT).as_posix()))
    # Windows-specific production backend plus its source-level test fixtures; the old
    # Windows launcher, Android bridge, mobile toolchain and AAR are omitted.
    for name in ("cmd/native_backend", "cmd/native_fixture", "internal", "vendor", "evidence"):
        for path in walk_sources(core / name, evidence_tree=name == "evidence"):
            if name == "evidence":
                check_evidence_privacy(path)
            entries.append((path, PREFIX + path.relative_to(ROOT).as_posix()))
    for name in ("LICENSE.txt", "LICENSE_GO.txt", "THIRD_PARTY_NOTICES.md", "assets/windows/yingxia.ico"):
        entries.append((ROOT / "current_source" / name, PREFIX + "current_source/" + name))
    for name in ("LICENSE.txt", "LICENSE_GO.txt"):
        entries.append((ROOT / "current_source" / name, PREFIX + name))
    entries.extend([
        (readme_path, PREFIX + "RELEASE_README_zh.md"),
        (validation_path, PREFIX + "YingXia_v2.2.0_Native_Windows_VALIDATION.json"),
    ])
    seen = set()
    for path, name in entries:
        safe_archive_name(name)
        if name in seen or not path.is_file() or is_link(path):
            raise ValueError(f"Duplicate, missing or linked source entry: {name}")
        seen.add(name)
    return sorted(entries, key=lambda item: item[1])


def verified_software(validation_path: Path) -> dict:
    validation = json.loads(validation_path.read_text(encoding="utf-8-sig"))
    matches = [record for record in validation["software"] if record.get("file") == EXE_NAME]
    if len(matches) != 1:
        raise ValueError("Validation must contain exactly one final Windows 2.2 EXE entry")
    executable = RELEASES / EXE_NAME
    actual = {"file": EXE_NAME, "size": executable.stat().st_size, "sha256": digest(executable)}
    record = matches[0]
    if record.get("size") != actual["size"] or record.get("sha256", "").lower() != actual["sha256"]:
        raise ValueError("Final EXE bytes changed or differ from the validation report")
    return actual


def write_zip(destination: Path, entries: list[tuple[Path, str]]) -> dict:
    checksum_path = destination.with_suffix(destination.suffix + ".sha256.txt")
    for path in (destination, checksum_path):
        if path.exists():
            raise FileExistsError(f"Preserve published artifact: {path.name}")
    records = [{"path": name, "size": path.stat().st_size, "sha256": digest(path)} for path, name in entries]
    with zipfile.ZipFile(destination, "x", compression=zipfile.ZIP_DEFLATED, compresslevel=6, allowZip64=True) as archive:
        for (path, name), record in zip(entries, records, strict=True):
            info = zipfile.ZipInfo(name, FIXED_DATE)
            info.compress_type = zipfile.ZIP_DEFLATED
            written = hashlib.sha256()
            size = 0
            with path.open("rb") as source, archive.open(info, "w") as target:
                while block := source.read(1024 * 1024):
                    written.update(block)
                    size += len(block)
                    target.write(block)
            if size != record["size"] or written.hexdigest() != record["sha256"]:
                raise ValueError(f"Source changed during packaging: {name}")
        # Keep the empty generated-payload directory required by the launcher
        # recipe without including any candidate/runtime ZIP or manifest.
        directory = zipfile.ZipInfo(PREFIX + "native_windows_client/launcher/payload/", FIXED_DATE)
        directory.external_attr = 0x10
        archive.writestr(directory, b"")
        manifest = {
            "version": "2.2.0-native-windows",
            "files": records,
            "manifest_scope": "Every source/report file except this manifest itself; generated empty payload directory is not a file",
        }
        archive.writestr(zipfile.ZipInfo(PREFIX + "MANIFEST.json", FIXED_DATE), json.dumps(manifest, ensure_ascii=False, indent=2).encode("utf-8"))
    with zipfile.ZipFile(destination) as archive:
        if bad := archive.testzip():
            raise ValueError(f"Archive CRC failure: {bad}")
        for record in records:
            hasher = hashlib.sha256()
            with archive.open(record["path"]) as source:
                while block := source.read(1024 * 1024):
                    hasher.update(block)
            if hasher.hexdigest() != record["sha256"] or archive.getinfo(record["path"]).file_size != record["size"]:
                raise ValueError(f"Decompressed archive SHA/size mismatch: {record['path']}")
    checksum = digest(destination)
    checksum_path.write_text(f"{checksum}  {destination.name}\n", encoding="utf-8")
    return {"file": destination.name, "size": destination.stat().st_size, "sha256": checksum, "verified_files": len(records), "crc_verified": True, "all_decompressed_file_sha256_verified": True}


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--validation", type=Path, default=RELEASES / "YingXia_v2.2.0_Native_Windows_VALIDATION.json")
    parser.add_argument("--readme", type=Path, default=RELEASES / "YingXia_v2.2.0_Native_Windows_README_zh.md")
    parser.add_argument("--source-zip", type=Path, default=RELEASES / "YingXia_v2.2.0_Native_Windows_Source.zip")
    parser.add_argument("--report", type=Path, default=RELEASES / "YingXia_v2.2.0_Native_Windows_PACKAGING.json")
    parser.add_argument("--dry-run", action="store_true", help="Verify input software and list source files without writing an archive")
    args = parser.parse_args()
    software = verified_software(args.validation)
    entries = source_entries(args.validation, args.readme)
    if args.dry_run:
        print(json.dumps({"software": software, "source_files": len(entries), "archive_paths": [name for _, name in entries]}, ensure_ascii=False, indent=2))
        return
    if args.report.exists():
        raise FileExistsError(f"Preserve published packaging report: {args.report.name}")
    source_archive = write_zip(args.source_zip, entries)
    # Recheck the final software after compression as well as before it.
    if verified_software(args.validation) != software:
        raise ValueError("Final EXE changed during source packaging")
    report = {
        "software": [software], "source_archive": source_archive,
        "validation_report": args.validation.name,
        "validation_report_sha256": digest(args.validation),
        "private_runtime_data_included": False, "private_signing_material_included": False,
        "old_windows_launcher_and_android_bridge_included": False,
        "generated_payload_included": False,
        "resource_syso_included_with_source_recipe": True,
    }
    with args.report.open("x", encoding="utf-8") as stream:
        json.dump(report, stream, ensure_ascii=False, indent=2)
        stream.write("\n")
    print(json.dumps(report, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
