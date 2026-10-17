"""Update handoff pointers after Android 2.2 publication and source verification."""
from pathlib import Path
import hashlib
import json

ROOT=Path(__file__).resolve().parents[2]
RELEASES=ROOT/'releases'
def sha(p):
    with p.open('rb') as stream:return hashlib.file_digest(stream,'sha256').hexdigest()
def prepend(name,text):
    p=ROOT/name
    body=p.read_text(encoding='utf-8-sig')
    if text.splitlines()[0] not in body:p.write_text(text+'\n\n'+body,encoding='utf-8')
def main():
    validation=json.loads((RELEASES/'YingXia_v2.2.0_Android_VALIDATION.json').read_text(encoding='utf-8'))
    packaging=json.loads((RELEASES/'YingXia_v2.2.0_Android_PACKAGING.json').read_text(encoding='utf-8'))
    apk=validation['software'][0]
    archive=packaging['source_archive']
    assert sha(RELEASES/apk['file'])==apk['sha256']
    assert sha(RELEASES/archive['file'])==archive['sha256']
    notice='''> 2026-10-15 最新交付：**Android 原生版 2.2.0**。安装 `releases/YingXia_v2.2.0_Android.apk`（Android 8.0+，三 ABI，沿用 Android 2.0 包名和发布签名，可覆盖更新）。本轮按 Windows 2.2 移植全部业务与来源功能，独立源码为 `native_android_client/`、`native_android_core/`；Windows 2.2 及所有旧交付保持原样。

内置47来源（30 CMS、16 Kazumi、1既有目录），保留自动更新、来源测速和完整管理、访问检测与搜索进度、选集/倍速/全屏、收藏历史和暂停。超分默认关闭，临时档位与启动默认值分开；Android GPU 路径调整真实 Surface 并核实 FSR pass，支持自动降档和设备能力回退。模拟器正常使用兼容视频输出并关闭超分。诊断可通过 Android 系统保存窗口导出 JSON。

分析与49项Dart测试通过；Android实际Go引擎14项、5子例与1生命周期测试通过。正式APK和隔离合成媒体实测涵盖页面、默认值、更新、来源管理、HLS可见画面、进度、切集、暂停、停止重播和清空持久化。GPU探针的具体条件、实际结果及ARM/16KB设备/功耗未测边界见发布验证JSON。交付原字节未在验收后重编。

源码、许可和哈希：`releases/YingXia_v2.2.0_Android_Source.zip`、`releases/YingXia_v2.2.0_Android_README_zh.md`、`releases/YingXia_v2.2.0_Android_VALIDATION.json`、`releases/YingXia_v2.2.0_Android_PACKAGING.json`。下文是历史版本记录。'''
    prepend('00_READ_ME_FIRST.md',notice)
    prepend('02_PROJECT_CONTEXT.md',notice)
    prepend('03_CURRENT_STATE_AND_LIMITATIONS.md',notice)
    prepend('04_CODE_MAP_AND_BUILD.md','''# Android 2.2 当前代码与构建

Android客户端：`native_android_client/`；独立Go/JNI后台：`native_android_core/`。先阅读客户端、后台 README，以及 `releases/YingXia_v2.2.0_Android_VALIDATION.json`。外置工具构建后台 AAR，客户端 `tools/build_android.ps1` 生成已签名候选 APK，`tools/verify_android_apk.py` 检查三 ABI 与签名、NDK strip 后 JNI 对应。`tools/package_android_release.py` 只打包已验证的正式字节和对应源码，排除工具链、签名私钥、缓存及用户数据。Windows 当前代码仍为 `native_windows_client/` + `native_windows_core/`，未在本轮修改。下文为历史地图。''')
    prepend('05_NEXT_STEPS_AND_ACCEPTANCE.md','''# Android 2.2 审阅入口

安装 `releases/YingXia_v2.2.0_Android.apk`。本轮模拟器与构建验收已完成；后续真机审阅重点为实际来源播放、横竖屏、音频和手机 GPU 的超分档位、持续性能与耗电。保留默认关闭及自动降档，不把模拟器或桌面 GPU 数据当作手机基准。完整范围见 Android 2.2 发布验证记录。Windows 与旧版发布件保持原样。下文为历史计划。''')
    prepend('07_HANDOFF_VERIFICATION.md',f'''# Android 2.2 交付核验

APK：`{apk['file']}`，{apk['size']} bytes，SHA-256 `{apk['sha256']}`。

源码：`{archive['file']}`，{archive['size']} bytes，SHA-256 `{archive['sha256']}`。{archive['verified_files']}个文件的ZIP CRC与解压SHA均经核验。原始检查和软件对应见Android 2.2 VALIDATION/PACKAGING。旧发布件基线哈希已复核不变。下文为历史交接核验。''')
    prepend('AGENTS.md','''# Current Android iteration: 2.2.0

Latest user-directed delivery is `releases/YingXia_v2.2.0_Android.apk`, ported from Windows native 2.2. Current Android sources are `native_android_client/` and `native_android_core/`. Read their README files and Android 2.2 READMe/VALIDATION/PACKAGING in releases. Preserve all published APKs, EXEs and source archives; keep Windows 2.2 and historical native_client/native_core unchanged.

Android uses native Flutter widgets, JNI Go backend and libmpv. Embedded source count is47; live88lin/Kazumi updates can change that count. Preserve source management/speed/cancel/update, search media access ranking, local favorites/history, clear confirmation, playback/stop/replay/pause behavior, exact developer credits and minimal UI. Startup FSR default is Off; playback choices do not replace the explicit persisted default. Real Android Surface resizing/pass verification and auto downgrade are retained; unsupported outputs explicitly fall back Off. Android document export uses ACTION_CREATE_DOCUMENT without storage permissions.

Validation includes49 Dart tests, clean analysis, actual Android Go tests and installed APK/synthetic HLS checks. Keep GPU QA variant/context conditions separate from shipping configuration and ARM performance. Test only on owned emulator with isolated data; never clear Windows user records or phone data. Private signing keys, SDKs and caches remain outside source packages. The following Windows notices are historical where superseded here.''')
    state_path=ROOT/'project_state.json'
    state=json.loads(state_path.read_text(encoding='utf-8-sig'))
    state['project']['native_windows_internal_version']='2.2.0-native-windows'
    state['project']['native_android_public_version']='v2.2.0'
    state['project']['native_android_internal_version']='2.2.0-native-android'
    state['current_user_request']='按照Windows原生版2.2制作安卓版，保留全部功能并交付APK。'
    state['latest_delivery']={'edition':'android_native','source':['native_android_client','native_android_core'],'software':apk,'source_archive':archive,'validation':'releases/YingXia_v2.2.0_Android_VALIDATION.json','packaging':'releases/YingXia_v2.2.0_Android_PACKAGING.json','architecture':'Flutter native UI + in-process JNI Go + libmpv, no browser required'}
    state_path.write_text(json.dumps(state,ensure_ascii=False,indent=2),encoding='utf-8')
    sums_path=ROOT/'SHA256SUMS.txt'
    sums=sums_path.read_text(encoding='utf-8-sig')
    for item in [apk,archive]:
        line=f"{item['sha256']}  releases/{item['file']}"
        if line not in sums:sums+='\n'+line+'\n'
    sums_path.write_text(sums,encoding='utf-8')
    print('Android 2.2 handoff pointers updated; historical release bytes preserved.')
if __name__=='__main__':main()
