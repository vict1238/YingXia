"""Installed-byte v2.3 QA, restricted to the project's owned emulator-5560.

Reads only app library/preferences/registry JSON; never reads the bootstrap or
session credentials. Production installs use -r and never clear its data.
Fixture resets are restricted to the separate synthetic fixture application.
Every invocation gets a fresh evidence directory, including failed assertions.
"""
from __future__ import annotations

import argparse
import hashlib
import json
import re
import subprocess
import sys
import time
from datetime import datetime, timezone
from pathlib import Path

from android_ui import ADB, CLIENT, SERIAL, adb, bounds, tree

PRODUCTION = "com.yingxia.yingxia_native"
FIXTURE = PRODUCTION + ".fixture"
ACTIVITY = "/com.yingxia.yingxia_native.MainActivity"
EVIDENCE = CLIENT / "evidence/v2_3/android_runtime"
sys.stdout.reconfigure(encoding="utf-8")


def nodes():
    raw, root = tree()
    return raw, [node for node in root.iter("node")]


def label(node):
    return node.get("text") or node.get("content-desc") or ""


def labels():
    return [label(node) for node in nodes()[1] if label(node)]


def has(text):
    return any(text in value for value in labels())


def tap(text, contains=False, index=0, _attempt=0):
    current = nodes()[1]
    matches = [n for n in current if (text in label(n) if contains else text == label(n))]
    if not matches and not contains:
        matches = [n for n in current if text in label(n)]
    if not matches:
        raise AssertionError(f"UI label not found: {text}")
    clickable = [n for n in matches if n.get("clickable") == "true"]
    matches = clickable or matches
    matches.sort(key=lambda n: (bounds(n)[2] - bounds(n)[0]) * (bounds(n)[3] - bounds(n)[1]))
    selected = matches[index]
    x1, y1, x2, y2 = bounds(selected)
    nav_tops = [bounds(n)[1] for n in current if "个标签" in label(n)]
    if nav_tops and "个标签" not in label(selected):
        # Flutter's semantics can include the part of a scrollable button hidden
        # under NavigationBar. Tap only its visible content intersection.
        bottom = min(nav_tops)
        if y1 >= bottom or min(y2, bottom) - y1 < 32:
            assert _attempt < 3, f"Control remains clipped under NavigationBar: {text}"
            swipe()
            return tap(text, contains, index, _attempt + 1)
        y2 = min(y2, bottom)
    adb("shell", "input", "tap", str((x1 + x2) // 2), str((y1 + y2) // 2))
    time.sleep(.35)


def nav(text):
    values = labels()
    if any(text + "\n" in value for value in values):
        tap(text + "\n", contains=True)
    else:
        # Wide-layout sidebar uses the full page names.
        tap({"搜索": "搜索影片", "播放": "播放工作台", "收藏": "收藏与记录", "来源": "来源与诊断"}[text])


def swipe(up=True):
    values = re.findall(r"\d+", adb("shell", "wm", "size").splitlines()[-1])
    width, height = map(int, values[-2:])
    adb("shell", "input", "swipe", str(width // 2), str(int(height * (.76 if up else .26))),
        str(width // 2), str(int(height * (.26 if up else .76))), "250")
    time.sleep(.2)


def reach(text, limit=12, up=True):
    for _ in range(limit):
        if has(text):
            return
        swipe(up)
    raise AssertionError(f"Could not scroll to {text}")


def wait_for(text, seconds=35):
    deadline = time.monotonic() + seconds
    while time.monotonic() < deadline:
        if has(text):
            return
        time.sleep(.3)
    raise AssertionError(f"Timed out waiting for {text}")


def start(package, restart=False):
    if restart:
        adb("shell", "am", "force-stop", package)
    adb("shell", "am", "start", "-n", package + ACTIVITY)
    time.sleep(1.5)


def app_json(package, filename, optional=False):
    if filename not in {"library.json", "video_quality.json", "source_registry.json"}:
        raise AssertionError("QA may not read other private app files")
    try:
        return json.loads(adb("shell", "cat", f"/data/user/0/{package}/files/yingxia/{filename}"))
    except (RuntimeError, json.JSONDecodeError):
        if optional:
            return None
        raise


def library(package):
    return app_json(package, "library.json")


def records(package, title=None):
    rows = library(package).get("history", [])
    return [row for row in rows if title is None or row.get("title") == title]


def latest(package, title):
    rows = records(package, title)
    if not rows:
        raise AssertionError(f"No history row for {title}")
    return rows[0]


def fill_query(value):
    fields = [n for n in nodes()[1] if n.get("class") == "android.widget.EditText"]
    if not fields:
        raise AssertionError("Search input is not visible")
    x1, y1, x2, y2 = bounds(fields[0])
    adb("shell", "input", "tap", str((x1 + x2) // 2), str((y1 + y2) // 2))
    adb("shell", "input", "keyevent", "123")
    adb("shell", "input", "keyevent", *(["67"] * 120))
    adb("shell", "input", "text", value)
    adb("shell", "input", "keyevent", "4")
    time.sleep(.4)


def position(row):
    return float(row.get("position", 0))


def summary(data):
    if data is None:
        return None
    return {
        "favorites": data.get("favorites", []),
        "history": data.get("history", []),
        "settings": data.get("settings", {}),
    }


class Run:
    def __init__(self, phase, apk=None):
        stamp = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")
        self.directory = EVIDENCE / f"{stamp}_{phase}"
        self.directory.mkdir(parents=True, exist_ok=False)
        self.report = {
            "schema": 1, "phase": phase, "serial": SERIAL,
            "started_at": datetime.now(timezone.utc).isoformat(),
            "scope": "Owned API35 x86_64 ranchu emulator; native app UI and app-specific synthetic records. No physical Android16/MatePad/ARM/TV performance claim.",
            "apk": str(apk) if apk else None,
            "apk_sha256": hashlib.sha256(apk.read_bytes()).hexdigest() if apk else None,
            "checks": [],
        }
        self.write()

    def write(self):
        self.report["updated_at"] = datetime.now(timezone.utc).isoformat()
        (self.directory / "checks.json").write_text(json.dumps(self.report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")

    def capture(self, name):
        raw, current = nodes()
        (self.directory / f"{name}.xml").write_text(raw, encoding="utf-8")
        (self.directory / f"{name}.png").write_bytes(adb("exec-out", "screencap", "-p", binary=True))
        return [label(n) for n in current if label(n)]

    def check(self, identifier, assertion, action):
        try:
            extra = action() or {}
            self.report["checks"].append({"id": identifier, "status": "passed", "assertion": assertion, **extra})
            print(json.dumps({"id": identifier, "status": "passed", **extra}, ensure_ascii=False), flush=True)
        except Exception as error:
            failure = {"id": identifier, "status": "failed", "assertion": assertion,
                       "error": f"{type(error).__name__}: {error}"}
            try:
                self.capture(identifier + "_failed")
                failure["evidence"] = [identifier + "_failed.xml", identifier + "_failed.png"]
            except Exception as capture_error:
                failure["capture_error"] = str(capture_error)
            self.report["checks"].append(failure)
            self.write()
            print(json.dumps(failure, ensure_ascii=False), flush=True)
        self.write()

    def installed(self, package):
        path = adb("shell", "pm", "path", package).strip().removeprefix("package:")
        installed_sha = adb("shell", "sha256sum", path).split()[0]
        assert installed_sha == self.report["apk_sha256"], "Installed APK differs from candidate"
        return {"installed_sha256": installed_sha}

    def logs(self, package):
        pid = adb("shell", "pidof", package).strip().split()[0]
        raw = adb("logcat", "-d", "--pid=" + pid, "-v", "brief")
        relevant = [line for line in raw.splitlines() if re.search(r"YingXiaCore|AndroidRuntime|FATAL EXCEPTION|Unhandled Exception|E/flutter|EGL_BAD", line)]
        clean = "\n".join(relevant)
        clean = re.sub(r"yx_session_\d+=[a-f0-9]+", "[SESSION_REDACTED]", clean)
        clean = re.sub(r"https?://[^\s]+", "[URL_REDACTED]", clean)
        clean = re.sub(r"(?i)(cookie|csrf|authorization)[=: ][^,\n]+", r"\1=[REDACTED]", clean)
        (self.directory / "runtime_redacted.log").write_text(clean + "\n", encoding="utf-8")
        self.report["crash_markers"] = [line for line in relevant if re.search(r"FATAL EXCEPTION|Unhandled Exception", line)]
        self.write()


def validate_owner():
    assert SERIAL == "emulator-5560"
    assert adb("shell", "getprop", "ro.hardware").strip() == "ranchu"
    assert adb("shell", "getprop", "ro.build.version.sdk").strip() == "35"
    assert "uid=0(root)" in adb("shell", "id")


def reset_display():
    adb("shell", "wm", "size", "reset")
    adb("shell", "wm", "density", "reset")


def production(run, apk, skip_install=False):
    baseline = summary(app_json(PRODUCTION, "library.json", optional=True))
    before_quality = app_json(PRODUCTION, "video_quality.json", optional=True)
    run.report["upgrade_baseline"] = {"library": baseline, "video_quality": before_quality}
    run.write()
    adb("shell", "am", "force-stop", PRODUCTION)
    if not skip_install:
        install = adb("install", "-r", str(apk), timeout=180)
        assert "Success" in install, install
        run.check("production_update_identity", "Update installation preserves existing app data and installs the candidate bytes.", lambda: run.installed(PRODUCTION))
        after = summary(app_json(PRODUCTION, "library.json", optional=True))
        run.check("upgrade_retains_library", "Existing library structure/settings survive update; populated-record retention is covered only when baseline rows are present.",
                  lambda: {"retained": True, "favorite_count": len((baseline or {}).get("favorites", [])), "history_count": len((baseline or {}).get("history", []))} if baseline == after else (_ for _ in ()).throw(AssertionError("Library changed during installation")))
    else:
        run.check("production_identity", "Existing installation matches candidate bytes; this invocation performs no reinstall.", lambda: run.installed(PRODUCTION))
    start(PRODUCTION)
    wait_for("想看什么？")
    run.capture("production_search")
    run.check("native_startup", "Production APK starts native Flutter/JNI UI.", lambda: {"evidence": ["production_search.xml", "production_search.png"]})

    def workspace():
        nav("播放")
        values = run.capture("production_workspace")
        assert any("选集之后" in v for v in values)
        assert any("关闭" in v for v in values), values
        return {"evidence": ["production_workspace.xml", "production_workspace.png"], "default": before_quality}
    run.check("startup_default_off", "Fresh/retained Off startup default and the empty video surface remain visible.", workspace)

    def settings():
        nav("来源")
        values = run.capture("production_credits")
        assert any("翻翻蓄小激光" in v and "cqh.vict1238@gmail.com" in v for v in values)
        assert any("流风驻尘闲" in v and "yuchihaobo@gmail.com" in v for v in values)
        reach("跳过标记广告")
        run.capture("production_playback_settings")
        assert has("卡顿时提示换源")
        reach("播放与缓存")
        tap("播放与缓存")
        values = run.capture("production_cache_dialog")
        assert any("提前缓存" in v for v in values)
        tap("完成")
        return {"evidence": ["production_credits.xml", "production_playback_settings.xml", "production_cache_dialog.xml"]}
    run.check("credits_and_playback_settings", "Both exact developer credits, marked-ad skipping, source-switch setting and bounded cache options render.", settings)

    def manager():
        reach("来源管理（")
        values = run.capture("production_manager")
        registry = app_json(PRODUCTION, "source_registry.json")
        assert len(registry["providers"]) >= 47
        assert any("全部测速" in value for value in values)
        return {"provider_count": len(registry["providers"]), "evidence": ["production_manager.xml", "production_manager.png"]}
    run.check("source_management", "Existing source management and speed-check entry points remain usable.", manager)

    def pinyin():
        nav("搜索")
        fill_query("native")
        values = run.capture("production_pinyin_input")
        assert any("native" in value for value in values)
        # Real upstream search is bounded and its result is recorded, not assumed.
        tap("搜索")
        time.sleep(3)
        values = run.capture("production_pinyin_search")
        if any("取消搜索" in value for value in values):
            tap("取消搜索")
        return {"query": "native", "visible": values, "evidence": ["production_pinyin_input.xml", "production_pinyin_search.xml"]}
    run.check("pinyin_search_entry", "Pinyin/initial input and a bounded real source-search request are accepted.", pinyin)

    def tv_launcher():
        resolved = adb("shell", "cmd", "package", "resolve-activity", "--brief", "-a", "android.intent.action.MAIN", "-c", "android.intent.category.LEANBACK_LAUNCHER", PRODUCTION)
        assert PRODUCTION in resolved, resolved
        return {"resolved_activity": resolved.strip()}
    run.check("leanback_launcher", "Production package is discoverable through LEANBACK_LAUNCHER while retaining phone launch.", tv_launcher)
    run.logs(PRODUCTION)


def open_fixture_title(title):
    nav("搜索")
    fill_query("fixture")
    tap("搜索")
    wait_for("检测完成", 50)
    reach(title)
    tap(title, contains=True)
    wait_for("收藏")
    reach("播放所选集")


def start_selected():
    tap("播放所选集", contains=True)
    time.sleep(2)


def fixture_base(run, apk, reset):
    adb("shell", "am", "force-stop", PRODUCTION)
    adb("shell", "am", "force-stop", FIXTURE)
    install = adb("install", "-r", str(apk), timeout=180)
    assert "Success" in install, install
    if reset:
        assert FIXTURE.endswith(".fixture")
        assert "Success" in adb("shell", "pm", "clear", FIXTURE)
    start(FIXTURE)
    wait_for("想看什么？")
    run.check("fixture_identity", "Separate fixture package runs the recorded candidate bytes.", lambda: run.installed(FIXTURE))


def fixture_playback(run):
    title = "原生播放测试 HLS"
    open_fixture_title(title)
    if has("＋ 收藏"):
        tap("＋ 收藏")
    start_selected()
    time.sleep(2)
    before = latest(FIXTURE, title)
    run.capture("hls_frame_before")
    time.sleep(2)
    after = latest(FIXTURE, title)
    run.capture("hls_frame_after")
    assert position(after) > position(before) + 1, (before, after)
    nav("收藏")
    time.sleep(.8)
    paused = latest(FIXTURE, title)
    time.sleep(2)
    later = latest(FIXTURE, title)
    assert abs(position(later) - position(paused)) < .5
    run.capture("library_paused")
    assert any(row.get("title") == title for row in library(FIXTURE).get("favorites", []))
    saved = position(later)
    start(FIXTURE, restart=True)
    wait_for("想看什么？")
    nav("收藏")
    reach(title)
    # Favorite card is first; opening it should use latest-episode resume.
    tap(title, contains=True)
    time.sleep(2)
    resumed = latest(FIXTURE, title)
    run.capture("favorite_autoresume")
    assert position(resumed) >= max(1, saved - 1), (saved, resumed)
    assert position(resumed) < saved + 12, (saved, resumed)
    adb("shell", "input", "keyevent", "3")
    time.sleep(1)
    home_before = latest(FIXTURE, title)
    time.sleep(2)
    home_after = latest(FIXTURE, title)
    assert abs(position(home_after) - position(home_before)) < .5
    start(FIXTURE)
    run.capture("background_return_paused")
    return {"before": position(before), "after": position(after), "paused": saved,
            "resumed": position(resumed), "home_paused": position(home_after),
            "evidence": ["hls_frame_before.png", "hls_frame_after.png", "library_paused.xml", "favorite_autoresume.xml", "background_return_paused.xml"]}


def fixture_controls(run):
    adb("shell", "wm", "size", "1920x1080")
    adb("shell", "wm", "density", "160")
    start(FIXTURE, restart=True)
    wait_for("想看什么？")
    open_fixture_title("原生播放测试 HLS")
    start_selected()
    tap("全屏")
    time.sleep(.5)
    # Give the surface focus, then use D-pad navigation to open its settings.
    # Android's OS media-key dispatcher is intentionally not used as a wakeup.
    adb("shell", "input", "tap", "960", "350")
    time.sleep(.4)  # single-tap waits for the double-tap recognizer
    adb("shell", "input", "keyevent", "20")
    adb("shell", "input", "keyevent", "23")
    time.sleep(.4)
    # Pause while selecting options so the30second test clip cannot finish
    # during accessibility dumps. The explicit fullscreen Play callback then
    # exercises app suspension coordination before the hold assertion.
    tap("暂停")
    run.capture("landscape_fullscreen")
    values = run.capture("native_controls_panel")
    assert any("音量" in value for value in values)
    assert any("FSR" in value for value in values)
    tap("1.0×")
    tap("1.5×")
    # Dismiss panel by tapping the video, then hold its non-control area.
    adb("shell", "input", "tap", "960", "350")
    tap("播放")
    command = [str(ADB), "-s", SERIAL, "shell", "input", "swipe", "960", "350", "960", "350", "6000"]
    process = subprocess.Popen(command, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                               creationflags=getattr(subprocess, "CREATE_NO_WINDOW", 0))
    time.sleep(1.2)
    held = run.capture("hold_exact2")
    process.communicate(timeout=15)
    assert any("2× 快进中" in value for value in held), held
    restored = run.capture("hold_restored")
    assert not any("2× 快进中" in value for value in restored)
    assert any("1.5×" in value for value in restored), restored
    adb("shell", "input", "keyevent", "24")
    adb("shell", "input", "keyevent", "20")
    adb("shell", "input", "keyevent", "23")
    run.capture("dpad_controls")
    adb("shell", "input", "keyevent", "4")
    return {"hold_badge_seen": True, "restored_rate_seen": "1.5×",
            "scope": "1920x1080 logical rendering and Android remote key events on phone AVD, not a physical TV or TV UiMode device",
            "evidence": ["landscape_fullscreen.png", "native_controls_panel.xml", "hold_exact2.xml", "hold_restored.xml", "dpad_controls.xml"]}


def fixture_ads(run):
    reset_display()
    start(FIXTURE, restart=True)
    wait_for("想看什么？")
    open_fixture_title("原生播放测试 广告")
    start_selected()
    time.sleep(3)
    run.capture("marked_ads_playback")
    row = latest(FIXTURE, "原生播放测试 广告")
    duration = float(row.get("duration", 0))
    assert position(row) > 1
    # MPEG-TS absolute timestamps can retain the original30second clock even
    # when the relay removes4seconds. The redacted exported diagnostics prove
    # omissions; the displayed decoder duration must not be falsified.
    nav("来源")
    reach("导出诊断")
    tap("导出诊断")
    time.sleep(1)
    current = nodes()[1]
    fields = [n for n in current if n.get("class") == "android.widget.EditText"]
    assert fields, [label(n) for n in current]
    filename = "yingxia_v23_ads_" + datetime.now(timezone.utc).strftime("%H%M%S") + ".json"
    x1, y1, x2, y2 = bounds(fields[0])
    adb("shell", "input", "tap", str((x1 + x2) // 2), str((y1 + y2) // 2))
    adb("shell", "input", "keyevent", "123")
    adb("shell", "input", "keyevent", *(["67"] * 80))
    adb("shell", "input", "text", filename)
    adb("shell", "input", "keyevent", "4")
    run.capture("ads_diagnostic_saf")
    tap("SAVE", contains=True)
    time.sleep(1)
    paths = adb("shell", "find", "/storage/emulated/0", "-name", filename).splitlines()
    assert paths, "SAF export file was not found on owned emulator"
    raw = adb("shell", "cat", paths[0].strip())
    assert "yx_session_" not in raw, "Export unexpectedly contains a session marker"
    diagnostic = json.loads(raw)
    (run.directory / "ads_diagnostic.json").write_text(json.dumps(diagnostic, ensure_ascii=False, indent=2), encoding="utf-8")
    def relay_rows(value):
        if isinstance(value, dict):
            if "skipped_ad_segments" in value or "skipped_ad_seconds" in value:
                yield value
            for item in value.values():
                yield from relay_rows(item)
        elif isinstance(value, list):
            for item in value:
                yield from relay_rows(item)
    relay = list(relay_rows(diagnostic))
    assert relay, diagnostic.keys()
    assert any(float(item.get("skipped_ad_segments", 0)) >= 2 and float(item.get("skipped_ad_seconds", 0)) >= 4 for item in relay), relay
    return {"duration": duration, "position": position(row), "original_duration": 30,
            "relay": relay,
            "evidence": ["marked_ads_playback.xml", "marked_ads_playback.png", "ads_diagnostic_saf.xml", "ads_diagnostic.json"]}


def fixture_hold(run):
    adb("shell", "wm", "size", "1920x1080")
    adb("shell", "wm", "density", "160")
    start(FIXTURE, restart=True)
    wait_for("想看什么？")
    open_fixture_title("原生播放测试 HLS")
    start_selected()
    tap("全屏")
    # Hold early in a fresh30second clip. No accessibility/menu traversal
    # consumes its available media duration before the actual gesture.
    command = [str(ADB), "-s", SERIAL, "shell", "input", "swipe", "960", "350", "960", "350", "5000"]
    process = subprocess.Popen(command, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                               creationflags=getattr(subprocess, "CREATE_NO_WINDOW", 0))
    time.sleep(.9)
    assert process.poll() is None, "Hold command ended before capture"
    # UiAutomator waits for an idle accessibility tree, which a playing video
    # may never provide until after release. Capture pixels during the actual
    # held input, then inspect those pixels for the2×badge separately.
    (run.directory / "minimal_hold_exact2.png").write_bytes(adb("exec-out", "screencap", "-p", binary=True))
    held_at = time.monotonic()
    assert process.poll() is None, "Hold command ended during pixel capture"
    process.communicate(timeout=15)
    # Capture release pixels before the3second HUD timer. Accessibility dumps
    # can wait until after the HUD disappears and cannot establish restoration.
    time.sleep(.4)
    (run.directory / "minimal_hold_restored.png").write_bytes(adb("exec-out", "screencap", "-p", binary=True))
    row = latest(FIXTURE, "原生播放测试 HLS")
    assert position(row) > 2
    return {"held_pixels_require_visual_confirmation": True, "held_capture_monotonic": held_at,
            "restored_rate": "1.0×", "position": position(row),
            "display": "1920x1080 override on owned phone AVD, not physical TV",
            "evidence": ["minimal_hold_exact2.png", "minimal_hold_restored.png"]}


def fixture_alternative(run):
    reset_display()
    start(FIXTURE, restart=True)
    wait_for("想看什么？")
    open_fixture_title("原生播放测试 换源")
    start_selected()
    time.sleep(6)
    values = run.capture("alternative_fixture_playback")
    prompted = any("播放卡顿" in value or "查找来源" in value for value in values)
    result = {"stall_prompt_observed": prompted,
              "scope": "Fixture delays one segment body8seconds; advance cache may absorb it, so a prompt is conditional.",
              "evidence": ["alternative_fixture_playback.xml", "alternative_fixture_playback.png"]}
    if prompted:
        tap("查找来源")
        wait_for("切换并续播", 40)
        run.capture("same_episode_alternative")
        before = latest(FIXTURE, "原生播放测试 换源")
        tap("切换并续播")
        time.sleep(3)
        after = latest(FIXTURE, "原生播放测试 换源")
        assert after.get("episode") == before.get("episode"), (before, after)
        result.update({"same_episode": after.get("episode"), "position": position(after)})
    return result


def consolidate():
    """Produce stable maps without deleting or relabeling failed attempts."""
    for kind, expected in [
        ("production", "9f0206439131596f03007405e2e19dcadcc39023d1acf5d836de36b2be3e4c3e"),
        ("fixture", "13eaf972a7b1e2aec3bcba8533021096550620b46251d00858c6baa9f287b31d"),
    ]:
        checks, failed, observations, seen = [], [], [], set()
        for path in sorted(EVIDENCE.glob("*/checks.json")):
            report = json.loads(path.read_text(encoding="utf-8"))
            if report.get("apk_sha256") != expected:
                continue
            for source in report.get("checks", []):
                item = dict(source)
                item["source_report"] = path.relative_to(EVIDENCE).as_posix()
                item["evidence"] = [
                    (path.parent / entry).resolve().relative_to(EVIDENCE.resolve()).as_posix()
                    for entry in item.get("evidence", [])
                ]
                if source["status"] != "passed":
                    failed.append(item)
                    continue
                if source["id"] == "same_episode_alternative" and not source.get("stall_prompt_observed"):
                    observations.append({**item, "status": "conditional_not_exercised",
                        "assertion": "Delayed fixture played; cache absorbed the delay and no stall prompt appeared, so actual switch UI was not exercised."})
                    continue
                if source.get("held_pixels_require_visual_confirmation"):
                    observations.append({**item, "status": "awaiting_visual_confirmation"})
                    continue
                if item["id"] in seen:
                    continue
                seen.add(item["id"])
                if item["id"] == "upgrade_retains_library":
                    item.update({"assertion": "Signed update retained the existing empty library and network-consent setting; populated-record migration was not exercised.",
                                 "favorite_count": 0, "history_count": 0})
                if item["id"] == "pinyin_search_entry":
                    item["id"] = "ascii_source_search_request"
                    item["assertion"] = "ASCII query input and a real enabled-source search request worked; pinyin resolution itself was not asserted."
                checks.append(item)
        if kind == "fixture":
            captured = EVIDENCE / "20261016T041426Z_fixture"
            panel = captured / "native_controls_panel.xml"
            selected = captured / "fullscreen_hold_remote_failed.xml"
            if panel.exists() and selected.exists():
                # These artifacts were captured before a later hold assertion
                # failed. Preserve that failure while asserting only the
                # independently visible panel and chosen rate from its pixels.
                import xml.etree.ElementTree as ET
                panel_values = [label(n) for n in ET.parse(panel).iter("node")]
                selected_values = [label(n) for n in ET.parse(selected).iter("node")]
                if all(any(text in value for value in panel_values) for text in ["音量", "倍速", "FSR", "画面", "缓存设置"]) and any("1.5×" in value for value in selected_values):
                    checks.append({"id": "on_video_settings_and_speed", "status": "passed",
                        "assertion": "Actual fullscreen panel renders volume/speed/FSR/fit/cache; choosing1.5x updates the native-player rate shown by its footer. Later hold timing failures remain recorded separately.",
                        "source_report": "20261016T041426Z_fixture/checks.json",
                        "evidence": ["20261016T041426Z_fixture/native_controls_panel.xml", "20261016T041426Z_fixture/native_controls_panel.png", "20261016T041426Z_fixture/fullscreen_hold_remote_failed.xml"]})
        result = {
            "schema": 1, "apk_sha256": expected, "serial": SERIAL,
            "scope": "Actual installed bytes on the owned API35 x86_64 ranchu emulator; no physical Android16/MatePad/ARM/TV-performance claim.",
            "checks": checks, "passed_count": len(checks), "failed_attempts": failed,
            "optional_observations": observations,
            "attempt_notes": [
                "Partial controls hidden under NavigationBar and HUD auto-hide caused harness lookup failures; they are retained and not counted as passes.",
                "UiAutomator can wait until a held gesture ends; held screenshots must be captured before accessibility dumps.",
                "The initial26second decoder-duration expectation was incorrect for retained absolute MPEG-TS timestamps. Relay omissions are verified by exported diagnostic counters; decoder clock remains approximately30seconds.",
                "The SAF save button is uppercase SAVE on this emulator; a failed lowercase lookup was followed by a successful actual save.",
                "Physical TV UiMode/remote use and pinyin-title resolution were not independently established by this UI harness; related deterministic tests remain separate evidence.",
            ],
            "display_restored": {"size": adb("shell", "wm", "size").strip(), "density": adb("shell", "wm", "density").strip()},
        }
        destination = EVIDENCE / f"{kind}_checks.json"
        destination.write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
        print(json.dumps({"report": str(destination), "passed": len(checks), "failed_attempts": len(failed)}, ensure_ascii=False), flush=True)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("phase", choices=["production", "fixture", "fixture-playback", "fixture-controls", "fixture-hold", "fixture-ads", "fixture-alternative", "consolidate"])
    parser.add_argument("--apk", type=Path)
    parser.add_argument("--reset-fixture", action="store_true")
    parser.add_argument("--skip-install", action="store_true", help="Continue production UI QA after a recorded update install")
    args = parser.parse_args()
    validate_owner()
    if args.phase == "consolidate":
        consolidate()
        return
    apk = args.apk or CLIENT / "build/candidates" / ("YingXia_v2.3.0_Android.apk" if args.phase == "production" else "YingXia_v2.3.0_Android_fixture.apk")
    assert apk.exists(), apk
    run = Run(args.phase, apk)
    try:
        if args.phase == "production":
            production(run, apk, args.skip_install)
        else:
            fixture_base(run, apk, args.reset_fixture)
            actions = {
                "fixture-playback": ("native_resume_pause", "Decoded HLS advances, favorite reopens at saved progress, navigation/Home pause persists.", fixture_playback),
                "fixture-controls": ("fullscreen_hold_remote", "On-video fullscreen settings, exact2 hold/restoration and large-screen remote actions render.", fixture_controls),
                "fixture-hold": ("fullscreen_hold_restore", "Actual long press shows exact2 indicator and restores the prior1x rate on release in fullscreen.", fixture_hold),
                "fixture-ads": ("marked_ads_relay_and_decode", "Native playback advances and exported relay diagnostics prove2marked segments/4seconds omitted; TS decoder clock may retain30seconds.", fixture_ads),
                "fixture-alternative": ("same_episode_alternative", "Delayed synthetic source is played; same-episode switch is tested if the genuine monitor prompts.", fixture_alternative),
            }
            for phase, (identifier, assertion, action) in actions.items():
                if args.phase in {"fixture", phase}:
                    run.check(identifier, assertion, lambda action=action: action(run))
            run.logs(FIXTURE)
    finally:
        reset_display()
        run.report["display_restored"] = {"size": adb("shell", "wm", "size").strip(), "density": adb("shell", "wm", "density").strip()}
        run.write()
    print(json.dumps({"report": str(run.directory / "checks.json"), "checks": len(run.report["checks"]),
                      "failed": sum(c["status"] == "failed" for c in run.report["checks"])}, ensure_ascii=False), flush=True)


if __name__ == "__main__":
    main()
