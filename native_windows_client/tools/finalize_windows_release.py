"""Gate Windows 2.1 delivery on the selected candidate's full runtime evidence.

This script builds nothing. It copies the tested bytes and creates the release
README, checksum and validation report only after every preflight gate passes.
It does not package sources or update project state. Existing releases are
never replaced. Use --check-only to evaluate the gates without publishing.
"""
from __future__ import annotations

import argparse
from datetime import datetime, timezone
import hashlib
import json
import math
import os
from pathlib import Path, PurePosixPath
import re
import shutil
import statistics
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parents[2]
CLIENT = ROOT / "native_windows_client"
EVIDENCE = CLIENT / "evidence"
RELEASES = ROOT / "releases"
CANDIDATE_NAME = "YingXia_candidate6.exe"
CANDIDATE_SHA = "d826f45a89f9aa8bbed1ed578e666a724c6f984d15756bef0d90fd73af7bd106"
CANDIDATE_SIZE = 60_419_072
BUNDLE_SHA = "004d9a5fb484acc55709f103177a4184e211cb2f574c91917a23f2d9725b6006"
BUNDLE_FILE_COUNT = 244
SOURCE_FREEZE = EVIDENCE / "source_build_candidate6.json"
FINAL_NAME = "YingXia_v2.1.0_Native_Windows_x64.exe"
REPORT_NAME = "YingXia_v2.1.0_Native_Windows_VALIDATION.json"
README_NAME = "YingXia_v2.1.0_Native_Windows_README_zh.md"
FIXTURE_SHA = "0f1803480545b72b01353071ac8a543f41d4dabb6c8eea824728ef13486f4a46"
BASELINE_SHA = {
    "YingXia_v1.3.2_Windows_x64.exe": "63ea2d697008c28b9d423c0a803ecc420760fb8d2f6de39e39e03724573ab84e",
    "YingXia_v2.0.0_Native_Windows_x64.exe": "dd2062a005e12626033ab5f4281be7a540c4616a2f9d0edae16654cbb6889f3b",
    "YingXia_v2.0.0_Android.apk": "b9d470913a117005b89006e137b7517e6bafafd717ba96b91efe934677a4f9a6",
    "yingxia_backend.exe": "0e2febcd74110cda5547da784efaccfe7025e68ae65d6219173b2502acd24077",
}
PASS_NAMES = {
    "enhanced": {"YingXia FSR1 EASU", "YingXia FSR1 RCAS Enhanced"},
    "light": {"YingXia FSR1 RCAS Light"},
}


def require(condition: bool, explanation: str) -> None:
    if not condition:
        raise ValueError(explanation)


def digest(path: Path) -> str:
    with path.open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def load_json(path: Path):
    return json.loads(path.read_text(encoding="utf-8-sig"))


def relative(path: Path) -> str:
    return path.relative_to(ROOT).as_posix()


def safe_source_path(base: Path, name: str) -> Path:
    part = PurePosixPath(name)
    require(bool(name) and "\\" not in name and ":" not in name and not part.is_absolute() and ".." not in part.parts, f"Unsafe input manifest path: {name}")
    path = base.joinpath(*part.parts)
    require(path.resolve().is_relative_to(base.resolve()), f"Manifest path escapes source root: {name}")
    require(path.is_file() and not path.is_symlink(), f"Missing or linked input: {name}")
    return path


def evidence_record(path: Path, scope: str) -> dict:
    return {"path": relative(path), "size": path.stat().st_size, "sha256": digest(path), "scope": scope}


def check_sample(sample: dict, mode: str) -> dict:
    require(sample.get("effective") == mode and sample.get("requested") == mode, f"Missing actual {mode} mode observation")
    require(sample.get("source") == {"width": 1280, "height": 720}, f"Wrong actual {mode} input dimensions")
    require(sample.get("output") == {"width": 1920, "height": 1080}, f"Wrong actual {mode} native output dimensions")
    require(sample.get("shader_verified") is True and sample.get("vo_passes_status") == "available", f"No executed {mode} shader evidence")
    require(sample.get("renderer") == "hardware", f"{mode} shader proof does not use hardware rendering")
    passes = sample.get("vo_passes", [])
    # libmpv decorates the shader's DESC with its pass type and texture format.
    # Compare the exact shader label while still requiring an executed sample.
    executed = {
        re.sub(r"^user shader: (.+) \(rgb\)$", r"\1", item.get("desc", ""))
        for item in passes if item.get("count", 0) > 0
    }
    require(PASS_NAMES[mode].issubset(executed), f"Expected {mode} FSR processing stages were not executed")
    require(mode != "light" or "YingXia FSR1 EASU" not in executed, "Light sample unexpectedly executes EASU")
    # Copy only explicit noncredential renderer measurements to the release
    # report. The complete original observation remains in its evidence file.
    return {key: sample[key] for key in (
        "time", "requested", "effective", "source", "output", "fps", "rate",
        "position", "vo_drops", "vo_drop_delta", "decoder_drops", "render_ms",
        "vo_passes", "vo_passes_status", "shader_verified", "hwdec", "renderer",
    ) if key in sample}


def check_staged_provenance(runtime: dict, checks: list[dict]) -> tuple[dict, list[dict]]:
    """Validate the raw same-byte runs that a final combined report cites.

    Optional ``staged_raw_reports`` entries contain project-relative ``path``,
    ``sha256``, ``scope`` and ``selected_check_names``. A partial raw run with a
    later failure additionally needs ``failure_scope_note`` acknowledging it.
    Only its actual passed checks can count; unpassed checks must be supplied
    by another raw run on the identical candidate bytes.
    """
    raw_records = runtime.get("staged_raw_reports")
    if raw_records is None:
        require(not runtime.get("merged_from") and not runtime.get("staged_runs"), "Merged runtime report must declare staged_raw_reports provenance")
        return {"execution_method": "One completed runtime report for the selected candidate"}, []
    require(isinstance(raw_records, list) and len(raw_records) >= 2, "Staged runtime provenance requires its raw reports")
    all_names = {item["name"] for item in checks}
    production_names = {item["name"] for item in checks[:9]}
    covered, production_covered = set(), set()
    sources, evidence, raw_observations = [], [], []
    for item in raw_records:
        path = safe_source_path(ROOT, item["path"])
        require(path.resolve().is_relative_to(EVIDENCE.resolve()), "Raw runtime report must remain under the evidence directory")
        require(path != EVIDENCE / "windows/native_windows_fsr_tests.json", "Merged runtime report cannot cite itself as raw evidence")
        require(digest(path) == item.get("sha256"), "Staged raw report bytes changed")
        raw = load_json(path)
        require(raw.get("executable") == CANDIDATE_NAME and raw.get("sha256") == CANDIDATE_SHA, "Staged raw report tested different candidate bytes")
        require(raw.get("shipping_gui_tested") is True and not raw.get("direct_runner_diagnostic_only"), "Staged raw evidence must test the shipping EXE")
        require(raw.get("synthetic_media_only") is True and raw.get("real_third_party_playback_tested") is False, "Staged raw media scope differs from the final report")
        require(raw.get("fixture_backend_sha256") == FIXTURE_SHA, "Staged raw fixture identity differs from the final report")
        raw_checks = raw.get("checks", [])
        require(raw.get("check_count") == len(raw_checks), "Staged raw check-count metadata differs from its actual checks")
        passed_names = {check["name"] for check in raw_checks if check.get("passed") is True}
        selected = item.get("selected_check_names")
        require(isinstance(selected, list) and selected and len(set(selected)) == len(selected), "Staged provenance must explicitly select its distinct passed checks")
        require(set(selected).issubset(passed_names) and set(selected).issubset(all_names), "Merged checks are absent or not passed in their raw report")
        require(not covered.intersection(selected), "A staged check must be counted only once")
        covered.update(selected)
        if raw.get("production_backend_history_tested") is True:
            production_covered.update(set(selected) & production_names)
        failures = {key: value for key, value in raw.items() if (key.startswith("failure") or key in {"cleanup_error", "retained_test_directory"}) and value}
        require(not failures or bool(item.get("failure_scope_note")), "A partial raw run's later failure must be acknowledged in staged provenance")
        require(bool(item.get("scope")), "Staged raw report must state its evidence scope")
        sources.append({"path": relative(path), "sha256": item["sha256"], "scope": item["scope"], "selected_check_names": selected, "raw_run_had_later_failure": bool(failures), "failure_scope_note": item.get("failure_scope_note", "")})
        evidence.append(evidence_record(path, "Staged actual runtime evidence: " + item["scope"]))
        raw_observations.extend(raw.get("observations", []))
    require(covered == all_names, "Full 22-check final coverage is not supported by the staged raw reports")
    require(production_covered == production_names, "The nine production checks must run on the exact final candidate SHA")
    critical = {"enhanced_720_to_1080", "light_720_to_1080", "local_rtx4060_enhanced_samples", "hls_playback", "fps_fixture"}
    for observation in runtime.get("observations", []):
        if critical.intersection(observation):
            require(observation in raw_observations, "Merged actual renderer observation does not match its raw same-SHA evidence")
    return {"execution_method": "Completed coverage merged from same-SHA staged runs; not one uninterrupted test run", "staged_raw_reports": sources}, evidence


def check_runtime() -> tuple[dict, dict, Path, dict, list[dict]]:
    path = EVIDENCE / "windows/native_windows_fsr_tests.json"
    runtime = load_json(path)
    require(runtime.get("executable") == CANDIDATE_NAME and runtime.get("sha256") == CANDIDATE_SHA, "Runtime report tests different software bytes")
    require(runtime.get("shipping_gui_tested") is True and runtime.get("production_backend_history_tested") is True, "Shipping GUI and production history checks are required")
    require(runtime.get("synthetic_media_only") is True and runtime.get("real_third_party_playback_tested") is False, "Unexpected media-test scope")
    require(runtime.get("fixture_backend_sha256") == FIXTURE_SHA, "Runtime report uses a different synthetic fixture backend")
    require(not runtime.get("direct_runner_diagnostic_only"), "Direct-runner diagnostics cannot authorize delivery")
    require(not runtime.get("media_diagnostic_only") and not runtime.get("carried_forward_production_evidence"), "Partial media-only or older-candidate production evidence cannot authorize delivery")
    for key in runtime:
        if key.startswith("failure") or key in {"cleanup_error", "retained_test_directory"}:
            require(not runtime[key], f"Runtime report is not clean: {key}")
    checks = runtime.get("checks", [])
    require(runtime.get("check_count") == 22 and len(checks) == 22 and all(item.get("passed") is True for item in checks), "All 22 actual runtime checks must pass before delivery")
    require(len({item["name"] for item in checks}) == 22, "Runtime check names must be distinct")
    execution, staged_evidence = check_staged_provenance(runtime, checks)
    observations = runtime.get("observations", [])

    def observation(key):
        values = [item[key] for item in observations if key in item]
        require(len(values) == 1, f"Expected exactly one actual runtime observation: {key}")
        return values[0]

    enhanced = check_sample(observation("enhanced_720_to_1080"), "enhanced")
    light = check_sample(observation("light_720_to_1080"), "light")
    samples = observation("local_rtx4060_enhanced_samples")
    require(len(samples) >= 10, "The actual enhanced playback observation window is incomplete")
    for sample in samples:
        check_sample(sample, "enhanced")
    require(samples[-1].get("position", 0) - samples[0].get("position", 0) > 9, "Actual enhanced video clock did not progress through the observation window")
    require(all(item.get("integrated_gpu_benchmarked") is False for item in observations if "local_rtx4060_enhanced_samples" in item), "Observed GPU scope is inconsistent")
    fps_observations = [item for item in observations if "fps_fixture" in item]
    require(len(fps_observations) == 2 and {item["fps_fixture"] for item in fps_observations} == {24, 60}, "Actual 720p24 and 720p60 runtime observations are incomplete")
    for item in fps_observations:
        check_sample(item["sample"], "enhanced")
    hls_observation = observation("hls_playback")
    require(isinstance(hls_observation.get("position"), (int, float)) and hls_observation["position"] > 0.1, "Actual HLS video clock observation is incomplete")
    timings = [sample["render_ms"] for sample in samples if isinstance(sample.get("render_ms"), (int, float)) and math.isfinite(sample["render_ms"]) and sample["render_ms"] >= 0]
    measured = {
        "enhanced_720_to_1080": enhanced, "light_720_to_1080": light,
        "enhanced_observed_sample_count": len(samples),
        "enhanced_observed_video_clock_seconds": samples[-1]["position"] - samples[0]["position"],
        "enhanced_sample_measurements": [check_sample(sample, "enhanced") for sample in samples],
        "measurement_scope": "Actual libmpv vo-passes values during synthetic video on the local RTX 4060; not integrated-GPU throughput or end-to-end presentation latency",
    }
    if timings:
        measured["enhanced_observed_render_ms"] = {"count": len(timings), "minimum": min(timings), "median": statistics.median(timings), "maximum": max(timings)}
    return runtime, measured, path, execution, staged_evidence


def check_source_freeze() -> tuple[dict, Path]:
    path = SOURCE_FREEZE
    snapshot = load_json(path)
    require(snapshot.get("unchanged_during_build") is True, "Compiled source changed during the selected candidate build")
    records = snapshot.get("files", [])
    require(records and len({item["path"] for item in records}) == len(records), "Frozen build source list is empty or has duplicates")
    covered = []
    for item in records:
        source = safe_source_path(CLIENT, item["path"])
        require(digest(source) == item["sha256"], f"Source changed after the selected candidate build: {item['path']}")
        covered.append({"path": item["path"], "size": source.stat().st_size, "sha256": item["sha256"]})
    names = {item["path"] for item in records}
    for required in ("lib/yingxia_app.dart", "lib/video_quality.dart", "lib/mpv_nodes.dart", "pubspec.lock", "pubspec.yaml", "assets/shaders/fsr1_enhanced.glsl", "assets/shaders/fsr1_light.glsl", "vendor/media_kit_video/windows/video_output.cc", "launcher/main.go", "launcher/resource_windows_amd64.syso"):
        require(required in names, f"Missing required build source coverage: {required}")
    return {"candidate": CANDIDATE_NAME, "unchanged_during_build": True, "live_hashes_verified": True, "frozen_file_count": len(covered), "files": covered, "coverage_scope": "Exact files recorded in the selected candidate's before/after build snapshot; release-only helpers and later QA evidence are outside compiled code"}, path


def check_baseline() -> tuple[list[dict], Path]:
    path = EVIDENCE / "immutable_baseline_sha256.json"
    records = load_json(path)
    require({item["file"]: item["sha256"] for item in records} == BASELINE_SHA, "Immutable older-edition/backend baseline record changed")
    result = []
    for name, expected in BASELINE_SHA.items():
        software = ROOT / "native_core/build/yingxia_backend.exe" if name == "yingxia_backend.exe" else RELEASES / name
        require(digest(software) == expected, f"Existing software or shared backend changed: {name}")
        result.append({"file": name, "path": relative(software), "size": software.stat().st_size, "sha256": expected, "unchanged": True})
    return result, path


def check_bundle() -> tuple[dict, Path]:
    path = CLIENT / "launcher/payload/manifest.json"
    manifest = load_json(path)
    records = manifest.get("files", [])
    require(manifest.get("version") == "2.1.0-native-windows" and manifest.get("bundle_sha256") == BUNDLE_SHA, "Selected candidate runtime bundle identity changed")
    require(len(records) == BUNDLE_FILE_COUNT and len({item["path"] for item in records}) == BUNDLE_FILE_COUNT, "Selected candidate runtime bundle file coverage changed")
    archive_path = CLIENT / "launcher/payload/bundle.zip"
    require(digest(archive_path) == BUNDLE_SHA, "Selected candidate runtime ZIP bytes changed")
    with zipfile.ZipFile(archive_path) as archive:
        require(archive.testzip() is None, "Selected candidate runtime ZIP failed CRC validation")
        require(set(archive.namelist()) == {item["path"] for item in records} and len(archive.infolist()) == BUNDLE_FILE_COUNT, "Runtime ZIP entry set differs from manifest")
        for item in records:
            name = item["path"]
            part = PurePosixPath(name)
            require(not part.is_absolute() and ".." not in part.parts and "\\" not in name and ":" not in name, f"Unsafe runtime ZIP entry: {name}")
            require(archive.getinfo(name).file_size == item["size"], f"Runtime ZIP entry size changed: {name}")
            with archive.open(name) as entry:
                require(hashlib.file_digest(entry, "sha256").hexdigest() == item["sha256"], f"Runtime ZIP entry bytes changed: {name}")
    by_name = {item["path"]: item for item in records}
    require(by_name.get("yingxia_backend.exe", {}).get("sha256") == BASELINE_SHA["yingxia_backend.exe"], "Runtime bundle does not contain the unchanged production backend")
    for name in ("THIRD_PARTY_NOTICES.md", "licenses/pub-license-manifest.json", "licenses/runtime/THIRD_PARTY_NOTICES.txt", "licenses/fsr1/LICENSE_AMD_FSR1.txt"):
        require(by_name.get(name, {}).get("sha256") == digest(CLIENT / name), f"Runtime licensing differs from the final source: {name}")
    return {"sha256": BUNDLE_SHA, "file_count": BUNDLE_FILE_COUNT, "crc_verified": True, "all_decompressed_file_sha256_verified": True}, path


def check_other_evidence(bundle_path: Path) -> tuple[dict, list[dict]]:
    shader_path = EVIDENCE / "fsr_shader/angle_rgb_validation.json"
    shader = load_json(shader_path)
    require(shader.get("passed") is True and shader.get("upstream_commit") == "a21ffb8f6c13233ba336352bdff293894c706575", "Shader RGB validation did not pass for the pinned AMD source")
    require(len(shader.get("contexts", [])) == 2 and {item.get("requested_es_version") for item in shader["contexts"]} == {2, 3}, "Missing both shader language validation paths")
    for name, expected in shader.get("assets", {}).items():
        require(digest(safe_source_path(CLIENT / "assets/shaders", name)) == expected, f"Validated shader changed: {name}")
    require(set(shader.get("assets", {})) == {"fsr1_light.glsl", "fsr1_enhanced.glsl"}, "Shader validation coverage changed")
    bundle = load_json(bundle_path)
    by_name = {item["path"]: item for item in bundle["files"]}
    for name, expected in shader.get("angle_libraries", {}).items():
        require(by_name.get(name, {}).get("sha256") == expected, f"Actual validated ANGLE library differs from the selected candidate: {name}")
    for context in shader["contexts"]:
        require("RTX 4060" in context.get("renderer", ""), "Shader test GPU scope changed")
        require(context.get("enhanced_differs_from_bilinear") is True and context.get("flat_black_white_gray_regions_stable") is True, "Shader output/flat-region validation is incomplete")
        passes = context.get("passes", [])
        require(len(passes) == 3 and {item.get("description") for item in passes} == PASS_NAMES["enhanced"] | PASS_NAMES["light"], "Shader stage coverage changed")
        for stage in passes:
            require(stage.get("compiled_and_linked") is True and stage.get("rgb", {}).get("passed") is True and stage["rgb"].get("sampled_pixels") == 8210, "Shader compile/link or sampled RGB comparison is incomplete")
    analyze_path = EVIDENCE / "controller/analyze.log"
    tests_path = EVIDENCE / "controller/flutter_tests.log"
    analyze = analyze_path.read_text(encoding="utf-8-sig")
    tests = tests_path.read_text(encoding="utf-8-sig")
    require("No issues found!" in analyze, "Controller analysis is not clean")
    require(re.search(r"\+27:\s+All tests passed!", tests) is not None and "Some tests failed" not in tests, "All 27 controller/widget tests must pass")
    launcher_path = EVIDENCE / "launcher_tests.log"
    vet_path = EVIDENCE / "launcher_vet.log"
    launcher = launcher_path.read_text(encoding="utf-8-sig")
    passes = len(re.findall(r"(?m)^\s*--- PASS:", launcher))
    skips = len(re.findall(r"(?m)^\s*--- SKIP:", launcher))
    require(passes == 10 and skips == 1 and re.search(r"(?m)^PASS\s*$", launcher) is not None and "--- FAIL:" not in launcher, "Launcher test evidence differs from 10 PASS records / 1 harness skip")
    require(vet_path.read_text(encoding="utf-8-sig").strip() == "", "Launcher vet evidence is not clean")
    fixture_path = EVIDENCE / "fixture/transport.json"
    fixture = load_json(fixture_path)
    require(fixture.get("passed") == 6 and len(fixture.get("checks", [])) == 6 and fixture.get("backend_only") is True and fixture.get("synthetic_media_only") is True, "Six isolated fixture transport checks are required")
    inventory_path = EVIDENCE / "gpu_inventory.json"
    inventory = load_json(inventory_path)
    require(any("RTX 4060" in item.get("Name", "") for item in inventory), "Recorded local GPU inventory changed")
    checks = {
        "controller_and_widget": {"passed": 27, "analyze_no_issues": True, "automatic_downgrade_scope": "Deterministic controller tests with controlled drop/GPU samples; actual integrated-GPU frame drops were not forced"},
        "launcher": {"pass_records": passes, "harness_skips": skips, "vet_clean": True},
        "fixture_transport": {"passed": 6, "synthetic_media_only": True},
        "shader_rgb": {"passed": True, "glsl_versions": [100, 300], "stages_per_language": 3, "sampled_pixels_per_stage": 8210, "actual_gl_versions": [item["actual_gl_version"] for item in shader["contexts"]], "scope": shader["scope"]},
    }
    evidence = [
        evidence_record(shader_path, "Independent shader compilation and sampled RGB algorithm checks"),
        evidence_record(analyze_path, "Flutter analysis"), evidence_record(tests_path, "27 unit/controller/widget checks"),
        evidence_record(launcher_path, "10 Go PASS records and one explicit child-harness skip"), evidence_record(vet_path, "Go launcher vet"),
        evidence_record(fixture_path, "Six private synthetic-media transport checks"), evidence_record(inventory_path, "Read-only local adapter inventory"),
    ]
    return checks, evidence


def release_readme(execution: dict) -> str:
    execution_note = "检查由相同 SHA-256 的软件分批完成，原始报告及合并依据保存在验证记录中。" if execution.get("staged_raw_reports") else ""
    return f"""# 映匣 Windows 原生版 2.1.0

运行 `{FINAL_NAME}`，无需安装开发工具。启动前关闭旧版映匣。

播放工作台增加“超分”选择，默认轻量：

| 档位 | 处理方式 |
|---|---|
| 关闭 | 播放器正常缩放 |
| 轻量 | 双线性放大与低强度 FSR 1 RCAS 锐化 |
| 增强 | FSR 1 EASU 边缘重建与 RCAS 锐化 |

只在播放区域大于视频尺寸时处理，按实际显示尺寸和比例输出，上限 1920×1080。播放 720p 视频后点击全屏，可查看 1080p 输出。持续渲染掉帧或 GPU 耗时超出预算时，按“增强→轻量→关闭”自动降档；加载、缓冲、暂停和切换尺寸会重新观察。自动降档时界面显示实际档位。影片帧率保持原样。

搜索、播放、收藏与记录、来源与诊断继续采用简洁布局。“来源与诊断”顶部新增开发人员：

- 翻翻蓄小激光 · cqh.vict1238@gmail.com
- 流风驻尘闲 · yuchihaobo@gmail.com

收藏、记录和来源设置沿用 `%LOCALAPPDATA%\\YingXia\\standalone`；超分选择单独保存在该目录的 `video_quality.json`。

22 项实际窗口检查通过，其中 9 项使用生产后台，13 项使用隔离的合成视频。{execution_note}实播覆盖 720p24/30/60、MP4/HLS、三档处理、实际 1080p 纹理、切页 / 最小化暂停及恢复、换集、三轮尺寸与播放操作和退出清理。另有 27 项控制器 / 界面测试、启动器检查及着色器 RGB 校验。

实际显卡为 RTX 4060 Laptop GPU。核显吞吐、较旧 GLES 2 上下文回退和 HDR 运行尚未实测；自动降档通过受控性能数据的控制器测试验证。具体样本、软件和源码哈希见 `{REPORT_NAME}`。

本版仅更新 Windows 原生软件。FSR 1 使用 AMD MIT 源码，许可证和原始播放器源码随软件及对应源码包提供。
"""


def main() -> None:
    global CANDIDATE_NAME, CANDIDATE_SHA, CANDIDATE_SIZE, BUNDLE_SHA, BUNDLE_FILE_COUNT, SOURCE_FREEZE
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--candidate", type=Path, default=CLIENT / "build/candidates" / CANDIDATE_NAME, help="Candidate EXE basename or absolute path under this client's build/candidates")
    parser.add_argument("--expected-sha", default=CANDIDATE_SHA)
    parser.add_argument("--expected-size", type=int, default=CANDIDATE_SIZE)
    parser.add_argument("--expected-bundle-sha", default=BUNDLE_SHA)
    parser.add_argument("--expected-bundle-files", type=int, default=BUNDLE_FILE_COUNT)
    parser.add_argument("--source-freeze", type=Path, default=SOURCE_FREEZE)
    parser.add_argument("--check-only", action="store_true", help="Evaluate every gate and print the plan without creating release files")
    args = parser.parse_args()
    candidate = args.candidate
    if not candidate.is_absolute() and len(candidate.parts) == 1:
        candidate = CLIENT / "build/candidates" / candidate
    candidate = candidate.resolve()
    require(candidate.is_relative_to((CLIENT / "build/candidates").resolve()) and candidate.suffix.lower() == ".exe", "Candidate must be an EXE within the isolated candidate directory")
    require(re.fullmatch(r"[0-9a-fA-F]{64}", args.expected_sha) is not None and re.fullmatch(r"[0-9a-fA-F]{64}", args.expected_bundle_sha) is not None, "Expected software and bundle hashes must be SHA-256 values")
    require(args.expected_size > 0 and args.expected_bundle_files > 0, "Expected candidate size and bundle file count must be positive")
    SOURCE_FREEZE = args.source_freeze.resolve()
    require(SOURCE_FREEZE.is_relative_to(EVIDENCE.resolve()) and SOURCE_FREEZE.is_file(), "Source freeze must be a saved report under the isolated evidence directory")
    CANDIDATE_NAME, CANDIDATE_SHA, CANDIDATE_SIZE = candidate.name, args.expected_sha.lower(), args.expected_size
    BUNDLE_SHA, BUNDLE_FILE_COUNT = args.expected_bundle_sha.lower(), args.expected_bundle_files
    destinations = [RELEASES / FINAL_NAME, RELEASES / (FINAL_NAME + ".sha256.txt"), RELEASES / REPORT_NAME, RELEASES / README_NAME]
    for path in destinations:
        require(not path.exists(), f"Preserve published release: {path.name}")
    require(candidate.stat().st_size == CANDIDATE_SIZE and digest(candidate) == CANDIDATE_SHA, "Tested candidate bytes changed")
    runtime, observed, runtime_path, execution, staged_evidence = check_runtime()
    source_coverage, source_path = check_source_freeze()
    baseline, baseline_path = check_baseline()
    bundle, bundle_path = check_bundle()
    other_checks, evidence = check_other_evidence(bundle_path)
    evidence.extend(staged_evidence)
    evidence.extend([
        evidence_record(runtime_path, "22 actual selected-candidate GUI/runtime checks: nine production backend and thirteen isolated synthetic-media checks"),
        evidence_record(source_path, "Selected candidate build source before/after freeze and current hash correspondence"),
        evidence_record(baseline_path, "Older published editions and unchanged shared production backend"),
        evidence_record(bundle_path, f"{BUNDLE_FILE_COUNT}-file embedded runtime manifest"),
    ])
    report = {
        "version": "2.1.0-native-windows", "generated_utc": datetime.now(timezone.utc).isoformat(),
        "software": [{"file": FINAL_NAME, "size": CANDIDATE_SIZE, "sha256": CANDIDATE_SHA}],
        "tested_candidate_file": relative(candidate), "tested_candidate_sha256": CANDIDATE_SHA,
        "delivery_method": "Exact tested candidate bytes copied; no rebuild after runtime QA",
        "runtime": {"passed": 22, "production_backend_checks": 9, "synthetic_media_checks": 13, "shipping_gui_tested": True, "production_backend_history_tested": True, "synthetic_media_only": True, "real_third_party_playback_tested": False, "fixture_backend_sha256": FIXTURE_SHA, "checks": runtime["checks"], **execution},
        "actual_renderer_observations": observed, "embedded_runtime_bundle": bundle,
        "other_validation": other_checks, "source_byte_coverage": source_coverage,
        "immutable_older_software_and_backend": baseline, "evidence": evidence,
        "hardware_scope": {"actual_gpu": "NVIDIA GeForce RTX 4060 Laptop GPU", "integrated_gpu_benchmarked": False, "older_gles2_context_fallback_runtime_tested": False, "hdr_runtime_tested": False, "automatic_downgrade": "Validated in deterministic controller tests; no forced actual integrated-GPU drop experiment"},
    }
    if args.check_only:
        print(json.dumps({"all_delivery_gates_passed": True, "software": report["software"], "runtime_passed": 22, "frozen_source_files_verified": source_coverage["frozen_file_count"], "would_create": [relative(path) for path in destinations], "created_files": []}, ensure_ascii=False, indent=2))
        return
    # Stage everything before exposing the first final filename. Hard-link
    # creation is atomic and fails if another process created that destination;
    # each link points to our independent byte copy, never to the candidate.
    with tempfile.TemporaryDirectory(prefix=".YingXia_v2.1.0_", dir=RELEASES) as staging:
        stage = Path(staging)
        staged_exe = stage / FINAL_NAME
        shutil.copyfile(candidate, staged_exe)
        require(staged_exe.stat().st_size == CANDIDATE_SIZE and digest(staged_exe) == CANDIDATE_SHA, "Staged delivery copy differs from tested candidate")
        (stage / (FINAL_NAME + ".sha256.txt")).write_text(f"{CANDIDATE_SHA}  {FINAL_NAME}\n", encoding="utf-8")
        (stage / REPORT_NAME).write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
        (stage / README_NAME).write_text(release_readme(execution), encoding="utf-8")
        # Verify immutable inputs again immediately before publication.
        check_source_freeze()
        check_baseline()
        require(digest(candidate) == CANDIDATE_SHA and digest(CLIENT / "launcher/payload/bundle.zip") == BUNDLE_SHA, "Candidate or embedded bundle changed during release staging")
        for item in evidence:
            require(digest(safe_source_path(ROOT, item["path"])) == item["sha256"], f"Validated evidence changed during release staging: {item['path']}")
        for destination in destinations:
            os.link(stage / destination.name, destination)
    require(digest(RELEASES / FINAL_NAME) == CANDIDATE_SHA, "Published copy verification failed")
    print(json.dumps({"software": report["software"], "runtime_passed": 22, "frozen_source_files_verified": source_coverage["frozen_file_count"], "created_files": [relative(path) for path in destinations], "source_archive_created": False, "project_state_updated": False}, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
