"""Package the already validated three YingXia editions without private tools/data."""
from __future__ import annotations

import hashlib
import json
from pathlib import Path
import zipfile

ROOT = Path(__file__).resolve().parents[2]
RELEASES = ROOT / "releases"
FIXED_DATE = (2026, 10, 14, 0, 0, 0)


def digest(path: Path) -> str:
    with path.open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def write_zip(destination: Path, entries: list[tuple[Path, str]], *, stored=False):
    if destination.exists():
        raise FileExistsError(f"Preserve published archive: {destination.name}")
    seen = set()
    manifest = []
    with zipfile.ZipFile(destination, "x", compression=zipfile.ZIP_STORED if stored else zipfile.ZIP_DEFLATED, compresslevel=None if stored else 6) as archive:
        for path, name in sorted(entries, key=lambda item: item[1]):
            parts = Path(name).parts
            if name.startswith(("/", "\\")) or ".." in parts or ":" in name or name in seen:
                raise ValueError(f"Unsafe or duplicate archive path: {name}")
            seen.add(name)
            record = {"path": name, "size": path.stat().st_size, "sha256": digest(path)}
            manifest.append(record)
            info = zipfile.ZipInfo(name, FIXED_DATE)
            info.compress_type = archive.compression
            with path.open("rb") as source, archive.open(info, "w") as target:
                while block := source.read(1024 * 1024):
                    target.write(block)
        archive.writestr("MANIFEST.json", json.dumps(manifest, ensure_ascii=False, indent=2).encode())
    with zipfile.ZipFile(destination) as archive:
        bad = archive.testzip()
        if bad:
            raise ValueError(f"Archive CRC failure: {bad}")
        for record in manifest:
            if hashlib.sha256(archive.read(record["path"])).hexdigest() != record["sha256"]:
                raise ValueError(f"Archive SHA mismatch: {record['path']}")
    checksum = digest(destination)
    destination.with_suffix(destination.suffix + ".sha256.txt").write_text(f"{checksum}  {destination.name}\n", encoding="utf-8")
    return {"file": destination.name, "size": destination.stat().st_size, "sha256": checksum, "verified_entries": len(manifest)}


def source_entries():
    entries = []
    for base in (ROOT / "native_client", ROOT / "native_core"):
        for path in base.rglob("*"):
            if not path.is_file():
                continue
            relative = path.relative_to(ROOT)
            parts = relative.parts
            if any(part in {".dart_tool", "build", ".gradle", ".cxx", "ephemeral", ".plugin_symlinks", "node_modules", ".git", "__pycache__"} for part in parts):
                continue
            if path.name in {"local.properties", "key.properties", ".flutter-plugins-dependencies", "bundle.zip"}:
                continue
            if relative.as_posix() == "native_core/windows_launcher/payload/manifest.json":
                continue
            if path.suffix.lower() in {".jks", ".keystore", ".p12", ".pem"} or (path.suffix.lower() == ".log" and "evidence" not in parts):
                continue
            if path.suffix.lower() in {".exe", ".dll", ".apk"}:
                continue
            if path.suffix.lower() in {".pyc", ".pyo"}:
                continue
            if path.suffix.lower() == ".properties" and "key" in path.name.lower():
                continue
            if path.name.endswith(".iml"):
                continue
            entries.append((path, "YingXia_native_2.0.0/" + relative.as_posix()))
    for name in ("LICENSE.txt", "LICENSE_GO.txt"):
        entries.append((ROOT / "current_source" / name, "YingXia_native_2.0.0/" + name))
    for name in ("LICENSE.txt", "assets/windows/yingxia.ico"):
        entries.append((ROOT / "current_source" / name, "YingXia_native_2.0.0/current_source/" + name))
    entries.append((RELEASES / "YingXia_Three_Editions_README_zh.md", "YingXia_native_2.0.0/RELEASE_README_zh.md"))
    entries.append((RELEASES / "YingXia_Three_Editions_VALIDATION.json", "YingXia_native_2.0.0/YingXia_Three_Editions_VALIDATION.json"))
    return entries


def main():
    names = ("YingXia_v1.3.2_Windows_x64.exe", "YingXia_v2.0.0_Native_Windows_x64.exe", "YingXia_v2.0.0_Android.apk")
    readme = RELEASES / "YingXia_Three_Editions_README_zh.md"
    validation_path = RELEASES / "YingXia_Three_Editions_VALIDATION.json"
    for name in (*names, "YingXia_v1.3.2_Windows_x64.zip", validation_path.name):
        if not (RELEASES / name).is_file():
            raise FileNotFoundError(name)
    if not readme.is_file():
        raise FileNotFoundError(readme)
    software = [{"file": name, "size": (RELEASES / name).stat().st_size, "sha256": digest(RELEASES / name)} for name in names]
    validation = json.loads(validation_path.read_text(encoding="utf-8"))
    verified = {record["file"]: record["sha256"] for record in validation["software"]}
    for record in software:
        if verified.get(record["file"]) != record["sha256"]:
            raise ValueError(f"Software changed since validation: {record['file']}")
    core_aar = ROOT / "native_client/android/app/libs/yingxia_core.aar"
    if digest(core_aar) != validation["production_android_aar_sha256"]:
        raise ValueError("Source staging contains a changed or fixture Go AAR")
    for record in software:
        (RELEASES / (record["file"] + ".sha256.txt")).write_text(f"{record['sha256']}  {record['file']}\n", encoding="utf-8")
    source = write_zip(RELEASES / "YingXia_v2.0.0_Native_Source.zip", source_entries())
    client = ROOT / "native_client"
    entries = [(RELEASES / name, "YingXia_Three_Editions/" + name) for name in names]
    entries += [(readme, "YingXia_Three_Editions/README_zh.md"), (client / "THIRD_PARTY_NOTICES.md", "YingXia_Three_Editions/THIRD_PARTY_NOTICES_native.md")]
    entries += [(RELEASES / source["file"], "YingXia_Three_Editions/sources/" + source["file"]), (RELEASES / "YingXia_v1.3.2_Windows_x64.zip", "YingXia_Three_Editions/sources/YingXia_v1.3.2_Windows_x64.zip")]
    entries.append((validation_path, "YingXia_Three_Editions/VALIDATION.json"))
    for path in (client / "licenses").rglob("*"):
        if path.is_file():
            entries.append((path, "YingXia_Three_Editions/licenses/" + path.relative_to(client / "licenses").as_posix()))
    for name in ("LICENSE.txt", "LICENSE_GO.txt", "THIRD_PARTY_NOTICES.md"):
        entries.append((ROOT / "current_source" / name, "YingXia_Three_Editions/licenses/webview/" + name))
    combined = write_zip(RELEASES / "YingXia_Three_Editions_20261014.zip", entries, stored=True)
    report = {"software": software, "native_source_archive": source, "combined_archive": combined, "private_signing_material_included": False}
    (RELEASES / "YingXia_Three_Editions_PACKAGING.json").write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")
    print(json.dumps(report, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
