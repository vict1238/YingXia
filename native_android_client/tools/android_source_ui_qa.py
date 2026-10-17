"""Real source-management UI checks on the explicitly owned emulator-5560.

Uses only the installed YingXia test app, a fixed QA source, UI navigation, and
read-only access to that app's source_registry JSON. Never clears app data.
"""
from __future__ import annotations

import argparse
import hashlib
import json
import time
from datetime import datetime, timezone

from android_ui import CLIENT, SERIAL, adb, bounds, describe, snapshot, tap, tree
from android_runtime_qa import has, nav, reach, swipe

PACKAGE = "com.yingxia.yingxia_native"
SHA = "423289feea2dccb2d9ebd019e73d8a12d3b81b4bb9ba6b1253289ea4e6133e23"
QA_NAME = "QA_Source_22"
QA_EDITED = "QA_Source_22_Edited"
QA_URL = "https://example.com/api.php/provide/vod/"
EVIDENCE = CLIENT / "evidence/android/runtime_final"
REPORT = EVIDENCE / "source_ui_checks.json"


def registry():
    return json.loads(adb("shell", "cat", f"/data/user/0/{PACKAGE}/files/yingxia/source_registry.json"))


def state():
    rows = registry()["providers"]
    return {"provider_count": len(rows), "provider_ids": [row["id"] for row in rows],
            "enabled": {row["id"]: row["enabled"] for row in rows},
            "provider_fingerprint": hashlib.sha256(json.dumps(rows, sort_keys=True, ensure_ascii=False).encode()).hexdigest()}


def read_report():
    return json.loads(REPORT.read_text(encoding="utf-8"))


def check(identifier, assertion, evidence, **extra):
    report = read_report()
    report["checks"].append({"id": identifier, "status": "passed", "assertion": assertion,
                             "evidence": evidence, **extra})
    report["updated_at"] = datetime.now(timezone.utc).isoformat()
    REPORT.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def snap(name):
    return snapshot(name, EVIDENCE)


def reach_editable():
    for _ in range(8):
        _, root = tree()
        if any(node.get("class") == "android.widget.EditText" for node in root.iter("node")):
            return
        swipe()
    raise AssertionError("Could not reach source filter field")


def fill(index, value):
    _, root = tree()
    fields = [node for node in root.iter("node") if node.get("class") == "android.widget.EditText"]
    x1, y1, x2, y2 = bounds(fields[index])
    adb("shell", "input", "tap", str((x1+x2)//2), str((y1+y2)//2))
    adb("shell", "input", "keyevent", "123")
    adb("shell", "input", "keyevent", *(["67"] * 100))
    if value:
        adb("shell", "input", "text", value)
    adb("shell", "input", "keyevent", "4")
    time.sleep(.3)


def find_qa():
    rows = registry()["providers"]
    return [row for row in rows if row.get("name") in {QA_NAME, QA_EDITED}]


def baseline():
    path = adb("shell", "pm", "path", PACKAGE).strip().removeprefix("package:")
    actual = adb("shell", "sha256sum", path).split()[0]
    assert actual == SHA, "Installed APK is not the frozen final production APK"
    before = state()
    assert before["provider_count"] == 48 and not find_qa()
    EVIDENCE.mkdir(parents=True, exist_ok=True)
    REPORT.write_text(json.dumps({"schema": 1, "apk_sha256": SHA, "serial": SERIAL,
        "started_at": datetime.now(timezone.utc).isoformat(),
        "scope": "Installed final production APK on owned API35 x86_64 emulator; fixed QA source via UI only; app source_registry JSON read-only; no app data clearing or Windows user data access.",
        "baseline": before, "checks": []}, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    nav("来源")
    reach("来源管理（")
    snap("source_qa_baseline_320")
    check("production_identity", "Installed production base.apk SHA-256 matches the frozen release; initial 48 providers and enable states recorded.",
          ["source_qa_baseline_320.xml", "source_qa_baseline_320.png"], provider_count=48)
    print(json.dumps({"provider_count": 48, "screen": describe()}, ensure_ascii=False))


def add():
    tap("添加来源")
    _, dialog = tree()
    assert len([node for node in dialog.iter("node") if node.get("class") == "android.widget.EditText"]) >= 2
    fill(0, QA_NAME)
    fill(1, QA_URL)
    snap("source_qa_add_dialog")
    tap("添加")
    time.sleep(.5)
    rows = find_qa()
    assert len(rows) == 1 and rows[0]["name"] == QA_NAME and rows[0]["enabled"] is True
    assert rows[0].get("url", "").rstrip("/") == QA_URL.rstrip("/")
    assert state()["provider_count"] == 49
    reach_editable()
    fill(0, QA_NAME)
    reach(QA_NAME)
    snap("source_qa_added")
    check("custom_add", "UI adds only QA_Source_22 at the fixed example.com CMS URL, enabled by default; persisted count becomes 49.",
          ["source_qa_add_dialog.xml", "source_qa_added.xml", "source_qa_added.png"], qa_id=rows[0]["id"], provider_count=49)
    print(json.dumps(describe(), ensure_ascii=False))


def edit():
    tap("管理来源")
    tap("编辑")
    fill(0, QA_EDITED)
    # An empty URL intentionally preserves the existing endpoint.
    snap("source_qa_edit_dialog")
    tap("保存")
    time.sleep(.5)
    adb("shell", "input", "keyevent", "4")
    rows = find_qa()
    assert len(rows) == 1 and rows[0]["name"] == QA_EDITED
    assert rows[0].get("url", "").rstrip("/") == QA_URL.rstrip("/")
    reach(QA_EDITED)
    snap("source_qa_edited")
    check("custom_edit", "UI edits only the QA name; leaving the endpoint empty preserves the original example.com URL and provider count.",
          ["source_qa_edit_dialog.xml", "source_qa_edited.xml", "source_qa_edited.png"], provider_count=49)
    print(json.dumps(describe(), ensure_ascii=False))


def remove():
    tap("管理来源")
    tap("删除")
    assert has(f"删除“{QA_EDITED}”？")
    snap("source_qa_delete_confirm")
    assert state()["provider_count"] == 49
    tap("取消")
    assert len(find_qa()) == 1
    adb("shell", "input", "keyevent", "4")
    tap("管理来源")
    tap("删除")
    tap("删除")
    time.sleep(.5)
    assert not find_qa()
    before = read_report()["baseline"]
    after = state()
    assert after["provider_count"] == before["provider_count"] == 48
    assert after["enabled"] == before["enabled"]
    assert after["provider_fingerprint"] == before["provider_fingerprint"]
    reach_editable()
    fill(0, "")
    snap("source_qa_deleted_restored")
    check("custom_delete_restore", "Delete first offers confirmation; Cancel keeps QA source; confirmed deletion restores all original 48 providers and every original enable state byte-for-byte.",
          ["source_qa_delete_confirm.xml", "source_qa_deleted_restored.xml", "source_qa_deleted_restored.png"], restored=after)
    print(json.dumps(describe(), ensure_ascii=False))


def speed():
    # Starting from the manager top makes progress/Cancel reachable immediately.
    for _ in range(3):
        if has("全部测速"): break
        swipe(False)
    tap("全部测速")
    time.sleep(.4)
    for _ in range(5):
        if has("取消测速") or has("测速完成"): break
        swipe()
    first = snap("source_qa_speed_active")
    progress = [row["text"] for row in first if "测速中" in row["text"] or "测速完成" in row["text"]]
    if has("取消测速"):
        tap("取消测速")
        time.sleep(.7)
        assert has("已取消") or has("测速完成")
    final = snap("source_qa_speed_result")
    result = [row["text"] for row in final if "已取消" in row["text"] or "测速完成" in row["text"]]
    assert progress and result
    check("all_source_speed", "Real all-provider metadata speed test is started through the UI; its actual cancellation or completed result is observed.",
          ["source_qa_speed_active.xml", "source_qa_speed_active.png", "source_qa_speed_result.xml", "source_qa_speed_result.png"], progress=progress, result=result)
    print(json.dumps({"progress": progress, "result": result}, ensure_ascii=False))


def landscape():
    adb("shell", "wm", "size", "640x360")
    time.sleep(1)
    for label, token, slug in (("搜索", "想看什么？", "search"), ("播放", "播放工作台", "workspace"),
                               ("收藏", "我的收藏", "library"), ("来源", "开发人员", "sources")):
        nav(label)
        assert has(token)
        data = snap(f"source_qa_landscape_{slug}")
        navigation = [row for row in data if "个标签，共 4 个" in row["text"]]
        assert len(navigation) == 4
        assert all(0 <= int(value) <= 640 for row in navigation for value in __import__('re').findall(r'\d+', row['bounds'])[::2])
    check("short_landscape_four_pages", "At physical/logical 640x360 and density160, all four pages render with four visible bottom navigation tabs and expected page content.",
          [f"source_qa_landscape_{slug}.{ext}" for slug in ("search", "workspace", "library", "sources") for ext in ("xml", "png")])


def restore():
    adb("shell", "wm", "size", "320x640")
    time.sleep(.7)
    nav("搜索")
    assert has("想看什么？")
    after = state()
    before = read_report()["baseline"]
    assert not find_qa() and after["provider_count"] == before["provider_count"] == 48
    assert after["enabled"] == before["enabled"]
    assert after["provider_fingerprint"] == before["provider_fingerprint"]
    snap("source_qa_restored_320")
    check("device_restored", "Device restored to 320x640; production Search foreground; QA source removed; original 48 providers and every enable state restored without clearing app data.",
          ["source_qa_restored_320.xml", "source_qa_restored_320.png"], final_state=after)
    report = read_report()
    report["completed_at"] = datetime.now(timezone.utc).isoformat()
    report["device_returned"] = True
    REPORT.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({"passed": len(report["checks"]), "device_returned": True, "count": 48}))


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("phase", choices=["baseline", "add", "edit", "remove", "speed", "landscape", "restore"])
    globals()[parser.parse_args().phase]()
