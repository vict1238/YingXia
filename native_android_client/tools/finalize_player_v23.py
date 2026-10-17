"""Publish already-validated bytes and their honest, scoped release records."""
from pathlib import Path
import hashlib
import json
import re
import shutil

ROOT = Path(__file__).resolve().parents[2]
RELEASE = ROOT / 'releases'
ANDROID = ROOT / 'native_android_client/evidence/v2_3'
WINDOWS = ROOT / 'native_windows_client/evidence/v2_3'

def load(path):
    return json.loads(path.read_text(encoding='utf-8-sig'))

def sha(path):
    return hashlib.file_digest(path.open('rb'), 'sha256').hexdigest()

def write_new(path, content):
    with path.open('x', encoding='utf-8') as stream:
        stream.write(content)

def all_pass(report):
    assert report['checks'] and all(x['status'] == 'passed' for x in report['checks'])

def main():
    production = load(ANDROID / 'android_runtime/production_checks.json')
    fixture = load(ANDROID / 'android_runtime/fixture_checks.json')
    window = load(WINDOWS / 'window_checks.json')
    for report in (production, fixture, window):
        all_pass(report)
    assert production['apk_sha256'] == '9f0206439131596f03007405e2e19dcadcc39023d1acf5d836de36b2be3e4c3e'
    assert fixture['apk_sha256'] == '13eaf972a7b1e2aec3bcba8533021096550620b46251d00858c6baa9f287b31d'
    assert window['sha256'] == '17aac338a81ba012f5b30327be7194381e45eb6872262348dccd598545be3652'
    for old in load(ANDROID / 'previous_artifacts.json'):
        assert sha(Path(old['Path'])).upper() == old['Hash'].upper(), old['Path']
    for client, expected in [('native_windows_client',56),('native_android_client',75)]:
        log = (ROOT/client/'evidence/v2_3/flutter_tests.txt').read_text(encoding='utf-8-sig')
        assert f'+{expected}: All tests passed!' in log
        assert 'No issues found!' in (ROOT/client/'evidence/v2_3/flutter_analyze.txt').read_text(encoding='utf-8-sig')
        frozen = load(ROOT/client/'evidence/v2_3/source_freeze.json')
        for record in frozen['files']:
            assert sha(ROOT/record['file']) == record['sha256'], record['file']
    specs = [
      ('Native_Windows', ROOT/'native_windows_client/build/YingXia_v2.3.0_Native_Windows_x64_candidate2.exe', 'YingXia_v2.3.0_Native_Windows_x64.exe', window['sha256'],56),
      ('Android', ROOT/'native_android_client/build/candidates/YingXia_v2.3.0_Android.apk', 'YingXia_v2.3.0_Android.apk', production['apk_sha256'],75),
    ]
    for edition, candidate, name, checksum, tests in specs:
        assert sha(candidate) == checksum
        targets = [RELEASE/name, RELEASE/(name+'.sha256.txt'), RELEASE/f'YingXia_v2.3.0_{edition}_VALIDATION.json', RELEASE/f'YingXia_v2.3.0_{edition}_README_zh.md']
        assert not any(p.exists() for p in targets), 'Preserve previous artifacts'
    backend = load(ROOT/'native_android_core/evidence/v2_3/backend_validation.json')
    software = []
    for edition, candidate, name, checksum, tests in specs:
        target = RELEASE/name
        with candidate.open('rb') as source, target.open('xb') as dest:
            shutil.copyfileobj(source,dest)
        assert sha(target) == checksum
        record = {'file':name,'size':target.stat().st_size,'sha256':checksum}
        software.append(record)
        write_new(RELEASE/(name+'.sha256.txt'),f'{checksum}  {name}\n')
        android = edition == 'Android'
        validation = {
          'schema':1,'version':'2.3.0','date':'2026-10-16','software':[record],
          'build':load(ANDROID/'apk_verification.json') if android else {'bundle_files':245,'reused_renderer':'exact unchanged published DLL, source correspondence verified','renderer_report':'native_windows_client/evidence/v2_3/renderer_compatibility.json'},
          'flutter':{'tests_passed':tests,'analysis':'No issues found','source_freeze':f'{"native_android_client" if android else "native_windows_client"}/evidence/v2_3/source_freeze.json'},
          'backend':backend,
          'runtime':{'production':production,'fixture':fixture} if android else window,
          'transport':None if android else load(WINDOWS/'transport.json'),
          'preserved_previous_2_2_artifacts':True,
          'scope_limits':[
            'Owned API35 x86_64 Android emulator; MatePad Pro Android16 and physical ARM/TV sustained performance remain unmeasured.',
            'FSR rendering code is preserved; this iteration adds controls but does not repeat integrated-GPU throughput benchmarks.',
            'Only finite HLS explicit ad cues are removed; unmarked/burned-in advertising is not automatically recognized.',
            'Windows fixture backend launch was blocked by ApplicationControl4551; no system policy was changed; no Windows new-decoder fixture pass is claimed.',
          ],
        }
        write_new(RELEASE/f'YingXia_v2.3.0_{edition}_VALIDATION.json',json.dumps(validation,ensure_ascii=False,indent=2)+'\n')
        label = 'Android' if android else 'Windows 原生'
        install = 'Android 8.0 及以上；沿用原包名、签名，可覆盖安装。包含 ARM64、ARMv7、x86_64。' if android else 'Windows x64。先退出旧版，再打开 EXE；收藏、历史和设置沿用本机原目录。'
        extras = 'Android 增加长按画面临时 2 倍速，松开恢复；电视提供启动入口、方向键焦点与大屏控制条。' if android else '播放区和全屏共用新的原生控制条，保留原有简洁布局。'
        text = f'''# 映匣 {label} 2.3.0

软件：`{name}`。{install}

本版加入标记广告跳过、提前缓存、卡顿换源提示、收藏与历史断点续播，以及播放区内的音量、倍速、超分、画面比例、快进快退和缓存设置。{extras}

搜索可输入中文近似片名、完整拼音或首字母；多个匹配片名会显示候选供选择。联想结合本地已见片名与联网片名联想，之后仍按现有流程检查实际来源。换源先核对作品、年份、季数和当前集数，确认后从当前时间继续；临近片尾或已播完的记录从本集开头重播。清空历史同时清除断点，保留收藏。

缓存提供“标准／提前缓存”。卡顿时增加缓冲等待并提示检查其他来源；媒体转发修复固定总时限和长响应截断，分片按需预取、去重和有限重试。前向包缓存 Android 为 48／80 MiB、Windows 为 96／160 MiB，另有 8 MiB 回看缓存与后台 24 MiB 分片缓存；不使用持续增长的磁盘缓存。

超分启动默认仍为关闭，可在来源与诊断设置下次启动的默认值。保留来源管理、测速、88lin／Kazumi 更新、访问检查、收藏、开发人员名单和诊断导出。

广告处理依据 HLS 清单中的明确广告标记，设置中可关闭；未标记或直接烧录在画面中的广告无法保证自动识别。MatePad Pro（Android 16）仍需用此版进行实机持续播放复测。

本版通过 {tests} 项 Flutter 检查与无问题静态分析；后台最新源码在自建 Android 模拟器实际通过 37 项引擎测试、15 个子例和 1 项生命周期测试。Windows 启动器另通过 4 项顶层测试。实际窗口／安装包、失败尝试和设备范围见 `YingXia_v2.3.0_{edition}_VALIDATION.json`，不把模拟器结果当作 ARM 设备或核显性能测试。

源码与许可：`YingXia_v2.3.0_{edition}_Source.zip`。Go、Flutter、libmpv、FFmpeg、FSR 与 go-pinyin 的对应源码／许可证和构建说明随源码包保存。发布哈希：`{checksum}`。
'''
        write_new(RELEASE/f'YingXia_v2.3.0_{edition}_README_zh.md',text)
    notice = '''> 2026-10-16 最新交付：Windows 原生版与 Android 均为 **2.3.0**，软件、源码、许可与实际验证见 `releases/YingXia_v2.3.0_*`。本轮加入标记广告跳过、有限提前缓存、卡顿换源提示、收藏／历史断点续播、播放区控件和拼音／首字母／模糊搜索；Android 增加长按临时 2 倍速与电视大屏适配。Windows 56 项、Android 75 项 Flutter 测试通过，后台最新源码在隔离模拟器通过 37 项引擎测试、15 子例与 1 项生命周期测试。实际运行范围与失败尝试见发布验证记录，MatePad Pro Android 16 仍需真机复测。此前发布文件保持原字节。下文为历史版本说明。\n\n'''
    for name in ['00_READ_ME_FIRST.md','02_PROJECT_CONTEXT.md','03_CURRENT_STATE_AND_LIMITATIONS.md','04_CODE_MAP_AND_BUILD.md']:
        p = ROOT/name
        p.write_text(notice+p.read_text(encoding='utf-8-sig'),encoding='utf-8')
    p = ROOT/'AGENTS.md'
    heading = '''# Current player iteration: Windows and Android 2.3.0

Latest user-directed work changes BOTH native_windows_client/native_windows_core and native_android_client/native_android_core. Releases are YingXia_v2.3.0_Native_Windows_x64.exe and YingXia_v2.3.0_Android.apk; preserve them and all earlier software/source archives. Read their 2.3 README, VALIDATION and PACKAGING records before further work. Keep historical native_client/native_core/current_source unchanged.

Features include conservative explicit-HLS-ad skipping, bounded memory packet+segment caches, stall suggestions with exact same-episode verified alternatives, latest-library resume, on-video controls, pinyin/initial/fuzzy suggestions, Android temporary2x hold and TV support. Startup FSR default stays Off; user default remains distinct from temporary selection. Keep developer credits/minimal UI and generation-safe history clearing. Sources remain provider-managed with speed/update controls.

Validation:56 Windows and75 Android Flutter tests, clean analysis;37 actual Android Go engine tests,15 subcases and1 lifecycle test;29 production private-transport checks;4 actual Windows launcher top-level tests. Installed APK/runtime and actual Windows window evidence have exact artifact hashes and clearly scoped attempts. Do not count harness failures or blocked Windows fixture launch as passes. Physical MatePad Pro Android16/ARM/TV sustained performance was not measured. Tests use ONLY isolated Windows data and the owned QA emulator; never touch user standalone records or phone data. Signing material/toolchains stay outside source packages. The following notices are historical where superseded.

'''
    p.write_text(heading+p.read_text(encoding='utf-8-sig'),encoding='utf-8')
    p = ROOT/'project_state.json'
    if p.exists():
        state = load(p)
        state['latest_player_iteration'] = {'version':'2.3.0','date':'2026-10-16','software':software,'validation':'See releases/YingXia_v2.3.0_*_VALIDATION.json','source_roots':['native_windows_client','native_windows_core','native_android_client','native_android_core']}
        p.write_text(json.dumps(state,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    print(json.dumps({'software':software,'previous_artifacts_preserved':True},ensure_ascii=False,indent=2))

if __name__ == '__main__': main()
