"""Generate synthetic 720p media and a separately tagged, isolated Go QA backend.

This never modifies native_core or includes the fixture in the shipping bundle.
"""
from pathlib import Path
import hashlib
import json
import os
import shutil
import subprocess
import time

CLIENT = Path(__file__).resolve().parents[1]
ROOT = CLIENT.parent
BUILD = CLIENT / "build" / "qa_core"
EVIDENCE = CLIENT / "evidence" / "fixture"
FFMPEG_DIR = ROOT / ".tools/native/ffmpeg_tool/extracted/ffmpeg-9.0.2-essentials_build/bin"
GO = ROOT / ".tools/next/go/bin/go.exe"


def run(args, log_name, cwd=None):
    with (EVIDENCE / log_name).open("w", encoding="utf8") as log:
        result = subprocess.run([str(a) for a in args], cwd=cwd, stdout=log, stderr=subprocess.STDOUT, check=False)
    if result.returncode:
        raise RuntimeError(f"{log_name} failed with {result.returncode}")


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    EVIDENCE.mkdir(parents=True, exist_ok=True)
    BUILD.mkdir(parents=True, exist_ok=True)
    original = ROOT / "native_core"
    for name in ("go.mod", "go.sum"):
        shutil.copy2(original / name, BUILD / name)
    shutil.copytree(original / "vendor", BUILD / "vendor", dirs_exist_ok=True)
    engine = BUILD / "internal/engine"
    engine.mkdir(parents=True, exist_ok=True)
    for source in (original / "internal/engine").glob("*.go"):
        if not source.name.endswith("_test.go"):
            shutil.copy2(source, engine / source.name)
    for name in ("sources.json", "providers.json", "LICENSE.txt"):
        shutil.copy2(original / "internal/engine" / name, engine / name)
    shutil.copytree(original / "internal/engine/fixturedata", engine / "fixturedata", dirs_exist_ok=True)
    shutil.copytree(original / "cmd/native_fixture", BUILD / "cmd/native_fixture", dirs_exist_ok=True)
    media = engine / "fixturedata/fsr"
    media.mkdir(parents=True, exist_ok=True)
    samples = []
    for fps in (24, 30, 60):
        target = media / f"720p{fps}.mp4"
        if not target.exists():
            run([FFMPEG_DIR / "ffmpeg.exe", "-hide_banner", "-y", "-f", "lavfi", "-i", f"testsrc2=size=1280x720:rate={fps}",
                 "-t", "120", "-an", "-c:v", "libx264", "-preset", "veryfast", "-threads", "4", "-crf", "25",
                 "-pix_fmt", "yuv420p", "-g", str(fps * 2), "-movflags", "+faststart", target], f"encode_720p{fps}.log")
        probe = subprocess.check_output([str(FFMPEG_DIR / "ffprobe.exe"), "-v", "error", "-count_frames", "-show_streams", "-of", "json", str(target)], text=True, encoding="utf8")
        stream = json.loads(probe)["streams"][0]
        if (stream["width"], stream["height"], int(stream["nb_read_frames"])) != (1280, 720, fps * 120):
            raise RuntimeError(f"Wrong decoded source format: {stream}")
        samples.append({"file": target.name, "width": stream["width"], "height": stream["height"], "fps": fps,
                        "duration_seconds": 120, "decoded_frames": int(stream["nb_read_frames"]), "bytes": target.stat().st_size, "sha256": digest(target)})
    run([FFMPEG_DIR / "ffmpeg.exe", "-hide_banner", "-y", "-i", media / "720p30.mp4", "-c:v", "copy", "-an",
         "-hls_time", "4", "-hls_playlist_type", "vod", "-hls_segment_filename", media / "720hls%03d.ts", media / "720hls.m3u8"], "remux_720hls.log")
    fixture = (engine / "fixture.go").read_text(encoding="utf8")
    fixture = fixture.replace("//go:embed fixturedata/demo/* fixturedata/hls/*", "//go:embed fixturedata/demo/* fixturedata/hls/* fixturedata/fsr/*")
    fixture = fixture.replace('ct := "video/webm"', 'ct := "video/webm"\n\tif strings.HasSuffix(name, ".mp4") { ct = "video/mp4" }')
    insert = "\n".join(
        f'\t\t{{"vod_id": "91{fps:03d}", "vod_name": "FSR 720p {fps}fps", "vod_year": "2026", "type_name": "合成测试", "vod_play_from": "FSR 测试", "vod_play_url": "第01集$https://media.example.com/fsr/720p{fps}.mp4#第02集$https://media.example.com/fsr/720p{fps}.mp4"}},'
        for fps in (24, 30, 60))
    insert += '\n\t\t{"vod_id": "91999", "vod_name": "FSR 720p HLS", "vod_year": "2026", "type_name": "合成测试", "vod_play_from": "FSR HLS 测试", "vod_play_url": "第01集$https://media.example.com/fsr/720hls.m3u8#第02集$https://media.example.com/fsr/720hls.m3u8"},'
    fixture = fixture.replace("return []Obj{", "return []Obj{\n" + insert, 1)
    (engine / "fixture.go").write_text(fixture, encoding="utf8")
    executable = BUILD / "yingxia_backend_fsr_fixture.exe"
    run([GO, "build", "-mod=vendor", "-tags", "native_fixture", "-trimpath", "-ldflags=-s -w", "-o", executable, "./cmd/native_fixture"], "go_fixture_build.log", cwd=BUILD)
    report = {"synthetic_only": True, "generated_utc_epoch": time.time(), "source_original_untouched": True,
              "backend": str(executable.relative_to(CLIENT)), "backend_sha256": digest(executable), "backend_bytes": executable.stat().st_size,
              "source_fixture_sha256": digest(engine / "fixture.go"), "ffmpeg_version": subprocess.check_output([str(FFMPEG_DIR / "ffmpeg.exe"), "-version"], text=True).splitlines()[0],
              "samples": samples, "hls_assets": [{"file": p.name, "bytes": p.stat().st_size, "sha256": digest(p)} for p in sorted(media.glob("*.ts"))]}
    (EVIDENCE / "fixtures.json").write_text(json.dumps(report, indent=2), encoding="utf8")
    print(json.dumps({"backend": str(executable), "sha256": report["backend_sha256"], "samples": samples}, indent=2))


if __name__ == "__main__":
    main()
