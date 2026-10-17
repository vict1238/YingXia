"""Publish the already checked APK bytes and write the release evidence index."""
from __future__ import annotations
import hashlib
import json
import re
import shutil
from datetime import datetime,timezone
from pathlib import Path

CLIENT=Path(__file__).resolve().parents[1]
ROOT=CLIENT.parent
RELEASES=ROOT/'releases'
E=CLIENT/'evidence/android'
NAME='YingXia_v2.2.0_Android.apk'
def read(p): return json.loads(p.read_text(encoding='utf-8-sig'))
def sha(p):
    with p.open('rb') as stream:return hashlib.file_digest(stream,'sha256').hexdigest()
def record(p):return {'file':p.name,'size':p.stat().st_size,'sha256':sha(p)}
def main():
    candidate=CLIENT/'build/candidates'/NAME
    apk=read(E/'apk_package.json')
    assert apk['sha256']==sha(candidate) and apk['go_jni_matches_aar_after_ndk_strip']
    production=read(E/'runtime_final/production_checks.json')
    fixture=read(E/'runtime_final/fixture_checks.json')
    sources=read(E/'runtime_final/source_ui_checks.json')
    assert production['apk_sha256']==apk['sha256']
    assert sources['apk_sha256']==apk['sha256']
    assert len(production['checks'])>=9 and len(fixture['checks'])>=7 and sources['checks']
    assert all(row['status']=='passed' for report in [production,fixture,sources] for row in report['checks'])
    correspondence=read(E/'fixture_release_correspondence.json')
    assert correspondence['production_sha256']==apk['sha256'] and correspondence['fixture_sha256']==fixture['apk_sha256']
    assert correspondence['identical_native_frontend_decoder_libraries']==12
    assert 'No issues found!' in (E/'flutter_analyze.log').read_text(encoding='utf-8')
    tests=(E/'flutter_tests.log').read_text(encoding='utf-8')
    assert re.search(r'\+49: All tests passed!',tests)
    # Verify all historical release artifacts recorded before the Android fork.
    baseline=read(E/'immutable_baseline.json')
    for old in baseline:
        p=RELEASES/old['file']
        assert sha(p)==old['sha256'] and p.stat().st_size==old['size']
    backend=read(ROOT/'native_android_core/evidence/android_validation.json')
    assert backend['go_tests']['engine_top_level_pass']==14 and backend['go_tests']['core_lifecycle_pass']==1
    assert sha(CLIENT/'android/app/libs/yingxia_core.aar')==backend['production_aar']['sha256']
    gpu_path=E/'gpu_debug/summary.json'
    gpu=read(gpu_path) if gpu_path.exists() else read(E/'gpu_baseline/summary.json')
    target=RELEASES/NAME
    validation_path=RELEASES/'YingXia_v2.2.0_Android_VALIDATION.json'
    readme_path=RELEASES/'YingXia_v2.2.0_Android_README_zh.md'
    for path in [target,validation_path,readme_path,target.with_suffix('.apk.sha256.txt')]:
        if path.exists():raise FileExistsError(f'Preserve published artifact: {path.name}')
    shutil.copyfile(candidate,target)
    assert sha(target)==apk['sha256']
    software=record(target)
    validation={
        'schema':1,'version':'2.2.0-native-android','verified_at':datetime.now(timezone.utc).isoformat(),
        'software':[software],'package':apk,'production_android_aar_sha256':backend['production_aar']['sha256'],
        'flutter':{'analyze':'No issues found','tests_passed':49,'logs':['native_android_client/evidence/android/flutter_analyze.log','native_android_client/evidence/android/flutter_tests.log']},
        'go':{'android_engine_top_level':14,'schema_subcases':5,'lifecycle':1,'vet':'PASS','report':'native_android_core/evidence/android_validation.json'},
        'runtime':{'production':production,'fixture':fixture,'source_management':sources,'frontend_decoder_correspondence':correspondence},
        'gpu_probe':gpu,
        'gpu_variant':read(E/'gpu_debug_variant/artifact.json'),
        'gpu_probe_notes':'Standard emulator playback uses mediacodec_embed and explicitly disables FSR. The ordinary GPU fixture could not create its emulator context. The successful GPU QA run uses the separately documented clone with only gpu-debug=yes added, synthetic 640x360 HLS, software decoding and a 1920x1080 fullscreen target. It verifies both actual FSR pass graphs, source-size restoration and observed Enhanced-to-Light downgrade, not the unmodified production GPU context or ARM performance.',
        'source_registry':{'embedded':47,'reviewed_cms':30,'kazumi_pinned':16,'legacy':1,'upstream_updates_may_change_count':True},
        'runtime_source_manifest':'native_android_client/evidence/android/runtime_source_manifest.json',
        'scope':['Actual release APK installed over the same signed Android package on an owned API35 x86_64 emulator.','Synthetic continuous H264 HLS through the same frontend, decoder, private API and storage paths.','Three-ABI APK signature, AOT, shaders and Go JNI correspondence checked.','No physical ARM device, 16KB-page device, battery/thermal run or audible sound-quality benchmark.','External metadata access and source updates are separate from complete third-party video playback availability.'],
        'historical_artifacts_unchanged':baseline,'private_signing_material_packaged':False,
        'published_without_rebuild_after_runtime_validation':True,
    }
    validation_path.write_text(json.dumps(validation,ensure_ascii=False,indent=2),encoding='utf-8')
    target.with_suffix('.apk.sha256.txt').write_text(f'{software["sha256"]}  {NAME}\n',encoding='utf-8')
    readme=f'''# 映匣 Android 原生版 2.2.0

安装 `{NAME}`，支持 Android 8.0 及以上。通用 APK 包含 arm64-v8a、armeabi-v7a、x86_64。可覆盖更新此前同签名的 Android 2.0，保留应用数据。手机独立运行，无需电脑后台或浏览器。

以 Windows 2.2 为基线，保留内置 47 个来源、访问检测与可取消的搜索进度、来源测速、查找筛选、分页、启停、自定义来源管理，以及 88lin/Kazumi 手动与自动更新。更新后实际来源数量会变化。来源测速表示接口响应，具体媒体入口继续由逐片访问检测判断。

播放工作台保留选集、切换片源、倍速、全屏、暂停、停止重播及平台绑定。切页或进入后台时暂停；回到前台等待手动继续。收藏、历史和设置存放在应用私有目录，清空记录保留收藏。手机竖屏及短横屏采用底部导航；空播放区保留选集提示，实际播放使用下方按钮。

超分默认关闭，可临时选择关闭／轻量／增强，也可在来源与诊断设置独立的下次启动默认档位。轻量为双线性放大加 RCAS，增强为 EASU 加 RCAS；实际 Surface 输出按比例放大、最高 1920×1080，持续渲染压力会自动降档。设备输出不支持或无法核实着色器执行时回退关闭。Android GPU 支持与持续性能依设备而定；本轮未做 ARM 真机、耗电或发热测试。没有插帧。

开发人员：翻翻蓄小激光（cqh.vict1238@gmail.com）、流风驻尘闲（yuchihaobo@gmail.com）。诊断报告可通过 Android 系统保存窗口导出 JSON。

验证记录包含 49 项 Dart 测试、Android 实际 Go 测试（14 个引擎测试、5 个子例与 1 个生命周期测试）、正式 APK 与隔离合成视频的安装运行检查。合成测试 APK 的 12 个界面与解码原生库与正式 APK 逐字节一致。运行范围和 GPU 探针条件详见 `YingXia_v2.2.0_Android_VALIDATION.json` 与源码中的原始证据。

APK 大小：{software['size']:,} bytes；SHA-256：`{software['sha256']}`。包名 `com.yingxia.yingxia_native`，版本码 20200，发布签名与旧版相同。源码、许可与构建说明在 `YingXia_v2.2.0_Android_Source.zip`；完整依赖通知在源码 `native_android_client/THIRD_PARTY_NOTICES.md`。
'''
    readme_path.write_text(readme,encoding='utf-8')
    print(json.dumps({'software':software,'production_checks':len(production['checks']),'fixture_checks':len(fixture['checks']),'source_checks':len(sources['checks'])},ensure_ascii=False))
if __name__=='__main__':main()
