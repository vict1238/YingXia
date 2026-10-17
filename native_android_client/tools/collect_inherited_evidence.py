"""Derive the Android source-provenance subset from the frozen Windows 2.2 tree.

Run only in the original handoff workspace. Build/runtime tools do not depend on
the Windows tree. Raw workbook, subscription/rule snapshots and HTTP bodies are
deliberately represented by hashes rather than copied into Android sources.
"""
from __future__ import annotations

import hashlib
import json
from pathlib import Path
import shutil
from urllib.parse import urlsplit, urlunsplit

ROOT = Path(__file__).resolve().parents[2]
CLIENT = ROOT / "native_android_client"
SOURCE = ROOT / "native_windows_client/evidence/v2_2/source_screening"
DEST = CLIENT / "evidence/windows_2_2/source_screening"


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def load(name):
    return json.loads((SOURCE / name).read_text(encoding="utf-8-sig"))


def save(name, value):
    (DEST / name).write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def endpoint(raw):
    parsed = urlsplit(str(raw or ""))
    host = parsed.hostname or ""
    if ":" in host:
        host = f"[{host}]"
    if parsed.port is not None:
        host += f":{parsed.port}"
    return urlunsplit((parsed.scheme, host, parsed.path, "", ""))


def probe(value):
    if not isinstance(value, dict):
        return None
    keys = ("checked_at", "status", "bytes", "sha256", "elapsed_ms", "content_type", "json",
            "valid_items", "matched_sample_id", "has_play_fields", "hls_header",
            "media_segments_downloaded", "decode_tested")
    return {key: value[key] for key in keys if key in value}


def decision(value):
    keys = ("record_id", "name", "sheet", "row", "decision", "reason", "final_decision",
            "final_reason", "existing_provider_ids", "sample_id", "item_count")
    result = {key: value[key] for key in keys if key in value}
    if "url" in value:
        result["endpoint"] = endpoint(value["url"])
    return result


def main():
    DEST.mkdir(parents=True, exist_ok=True)
    verbatim = ("accepted_providers.json", "aliases.json", "standard_port_fallback_probes.json")
    for name in verbatim:
        shutil.copyfile(SOURCE / name, DEST / name)
    summary = load("screening_summary.json")
    summary["workbook"]["path"] = "YingXia_Video_Sources_20261015.xlsx (not bundled)"
    summary["reports"] = {
        "cms": "cms_decisions.json", "search_detail_playlist": "search_detail_proof.json",
        "anime_web_parser_player": "other_types_decisions.json",
        "upstream": "kazumi_api9_provenance.json", "original_report_hashes": "source_origin_manifest.json",
        "original_workbook_and_response_snapshots_bundled": False,
    }
    save("screening_summary.json", summary)
    cms = load("cms_screening_report.json")
    save("cms_decisions.json", {
        "source_report_sha256": sha(SOURCE / "cms_screening_report.json"),
        "checked_at": cms["checked_at"], "scope": cms["scope"],
        "records": [decision(row) for row in cms["records"]],
    })
    save("search_detail_proof.json", [{
        **decision(row), "compatible_search_detail": row.get("compatible_search_detail"),
        "search": probe(row.get("search")), "detail": probe(row.get("detail")),
        "playlist": probe(row.get("playlist")),
    } for row in load("search_detail_playlist_probes.json")])
    other = load("other_types_screening_report.json")
    save("other_types_decisions.json", {
        "source_report_sha256": sha(SOURCE / "other_types_screening_report.json"),
        "checked_at": other["checked_at"],
        "groups": {key: [decision(row) for row in value if isinstance(row, dict)]
                   for key, value in other.items() if isinstance(value, list)},
    })
    api = load("kazumi_api9_primary_source.json")
    rule = load("kazumi_rule_updates.json")
    save("kazumi_api9_provenance.json", {
        "application_repository": api["repository"], "application_commit": api["commit"],
        "application_files": [{"path": row["path"], "probe": probe(row["probe"])} for row in api["files"]],
        "rules_repository": rule["upstream"]["repository"], "rules_commit": rule["upstream"]["commit"],
        "rule_probe": probe(rule["upstream"]["rule_probe"]),
        "updated_rules": [{"id": row["id"], "name": row["name"],
                           "api": row["rule"]["api"], "version": row["rule"]["version"],
                           "sourceTemplate": row["rule"].get("searchApiConfig", {}).get("sourceTemplate", "")}
                          for row in rule["providers"]],
        "full_production_rule_seed": "../../../../native_android_core/internal/engine/providers.json",
    })
    originals = ("screening_summary.json", "cms_screening_report.json", "search_detail_playlist_probes.json",
                 "other_types_screening_report.json", "kazumi_api9_primary_source.json", "kazumi_rule_updates.json",
                 "workbook_extracted.json", "subscription_probes.json", "kazumi_current_pinned_registry.json") + verbatim
    save("source_origin_manifest.json", {
        "schema": 1, "origin": "frozen Windows 2.2 screening, 2026-10-15; not a new Android network probe",
        "derivation": "whitelisted decisions and status/time/response hashes; no original workbook, response bodies, credentials or full upstream rule snapshot",
        "originals": [{"path": "native_windows_client/evidence/v2_2/source_screening/" + name,
                       "size": (SOURCE / name).stat().st_size, "sha256": sha(SOURCE / name),
                       "copied_verbatim": name in verbatim} for name in originals],
    })
    shader_source = ROOT / "native_windows_client/evidence/fsr_shader"
    shader_dest = CLIENT / "evidence/fsr_shader"
    shader_dest.mkdir(parents=True, exist_ok=True)
    shaders = ("shader_provenance.json", "angle_rgb_validation.json", "mpv_rcas_metadata_fix.md", "render_loop_audit.md")
    for name in shaders:
        shutil.copyfile(shader_source / name, shader_dest / name)
    (shader_dest / "inherited_files.json").write_text(json.dumps({
        "scope": "historical Windows shader provenance and algorithm audit only; Android runtime results are separate",
        "files": [{"file": name, "size": (shader_source / name).stat().st_size, "sha256": sha(shader_source / name)} for name in shaders],
    }, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({"source_evidence_files": len(list(DEST.iterdir())), "historical_shader_files": len(shaders)}))


if __name__ == "__main__":
    main()
