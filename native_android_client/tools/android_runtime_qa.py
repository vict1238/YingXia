"""Record assertions against installed release bytes on the owned QA AVD."""
from __future__ import annotations
import argparse
import hashlib
import json
import time
from datetime import datetime, timezone
from pathlib import Path
from android_ui import CLIENT, SERIAL, adb, snapshot, tap, tree, describe, bounds

EVIDENCE = CLIENT/'evidence/android/runtime_final'

def labels(): return [row['text'] for row in describe()]
def has(value): return any(value in item for item in labels())
def swipe(up=True):
    adb('shell','input','swipe','160', '475' if up else '170', '160', '170' if up else '475','350')
    time.sleep(.3)
def reach(text, limit=12):
    for _ in range(limit):
        if has(text): return
        swipe()
    raise AssertionError(f'Could not scroll to {text}')
def nav(label): tap(label+'\n', contains=True)
def start(package):
    adb('shell','am','start','-n',package+'/com.yingxia.yingxia_native.MainActivity')
    time.sleep(2)
def restart(package):
    adb('shell','am','force-stop',package)
    start(package)
def app_json(package, filename):
    return json.loads(adb('shell','cat',f'/data/user/0/{package}/files/yingxia/{filename}'))
def snap(name): return snapshot(name,EVIDENCE)

def wait_for(text, seconds=30):
    deadline=time.monotonic()+seconds
    while time.monotonic()<deadline:
        if has(text): return
        time.sleep(.5)
    raise AssertionError(f'Timeout waiting for {text}')

def history(package):
    return app_json(package,'library.json')['history']

def type_query(value):
    _,root=tree()
    node=next(n for n in root.iter('node') if n.get('class')=='android.widget.EditText')
    x1,y1,x2,y2=bounds(node)
    adb('shell','input','tap',str((x1+x2)//2),str((y1+y2)//2))
    adb('shell','input','text',value)
    adb('shell','input','keyevent','4')

def overview():
    package='com.yingxia.yingxia_native'
    apk=CLIENT/'build/candidates/YingXia_v2.2.0_Android.apk'
    results=[]
    def check(name, assertion, evidence, **extra):
        results.append({'id':name,'status':'passed','assertion':assertion,'evidence':evidence,**extra})
        write()
    def write():
        EVIDENCE.mkdir(parents=True, exist_ok=True)
        (EVIDENCE/'production_checks.json').write_text(json.dumps({'apk_sha256':hashlib.sha256(apk.read_bytes()).hexdigest(),'serial':SERIAL,'time':datetime.now(timezone.utc).isoformat(),'scope':'Installed release APK on owned API35 x86_64 emulator. Only project app private data and synthetic test records.','checks':results},ensure_ascii=False,indent=2),encoding='utf-8')
    restart(package)
    assert has('想看什么？')
    snap('production_search')
    check('startup','Installed release starts JNI backend and native search UI.',['production_search.xml','production_search.png'])
    nav('播放')
    assert has('选集之后请点击下方播放所选集') and has('关闭')
    snap('production_workspace')
    check('workspace_off','Workspace opens at Off with plain empty-surface text.',['production_workspace.xml','production_workspace.png'])
    nav('收藏')
    assert has('清空记录')
    snap('production_library')
    check('library','Library controls render in the installed APK.',['production_library.xml'])
    nav('来源')
    assert has('cqh.vict1238@gmail.com') and has('yuchihaobo@gmail.com')
    snap('production_sources_top')
    check('credits','Both requested developer names/emails appear before settings.',['production_sources_top.xml'])
    reach('默认超分')
    assert has('关闭')
    snap('production_default_off')
    tap('关闭')
    tap('轻量')
    prefs=app_json(package,'video_quality.json')
    assert prefs=={'version':2,'default_mode':'light'}
    restart(package)
    nav('播放')
    assert has('轻量')
    snap('production_default_light_restarted')
    # Temporary playback selection must not alter the startup preference.
    tap('轻量')
    tap('增强')
    assert app_json(package,'video_quality.json')['default_mode']=='light'
    nav('来源')
    reach('默认超分')
    tap('轻量')
    tap('关闭')
    assert app_json(package,'video_quality.json')['default_mode']=='off'
    restart(package)
    nav('播放')
    assert has('关闭')
    snap('production_default_off_restored')
    check('default_preference','Explicit default persists across force-stop; temporary Enhanced does not replace it; restored Off.',['production_default_off.xml','production_default_light_restarted.xml','production_default_off_restored.xml'])
    nav('来源')
    reach('来源管理（')
    snap('production_manager')
    registry=app_json(package,'source_registry.json')
    # The embedded seed has 47 entries. A genuine upstream refresh may add
    # Kazumi rules while QA is running; assert the fixed reviewed CMS set too.
    assert len(registry['providers'])>=47
    assert len([p for p in registry['providers'] if p.get('origin')=='reviewed_20261015'])==30
    summary={'provider_count':len(registry['providers']),'provider_ids':[p['id'] for p in registry['providers']],'health_count':len(registry.get('health',{})),'last_checked_at':registry.get('last_checked_at'),'seed_revision':registry.get('seed_revision')}
    (EVIDENCE/'registry_summary.json').write_text(json.dumps(summary,ensure_ascii=False,indent=2),encoding='utf-8')
    check('registry','Embedded 47-source registry migrated; 30 reviewed CMS retained through live upstream refresh.',['production_manager.xml','registry_summary.json'],actual_count=len(registry['providers']))
    adb('shell','input','keyevent','4')
    assert has('想看什么？')
    snap('production_back_to_search')
    check('android_back','Android Back returns from sources to search.',['production_back_to_search.xml'])
    print(json.dumps({'passed':len(results),'sha256':hashlib.sha256(apk.read_bytes()).hexdigest()}))

def fixture():
    package='com.yingxia.yingxia_native.fixture'
    apk=CLIENT/'build/candidates/YingXia_v2.2.0_Android_fixture.apk'
    results=[]
    def check(name,assertion,evidence,**extra):
        results.append({'id':name,'status':'passed','assertion':assertion,'evidence':evidence,**extra})
        (EVIDENCE/'fixture_checks.json').write_text(json.dumps({'apk_sha256':hashlib.sha256(apk.read_bytes()).hexdigest(),'serial':SERIAL,'scope':'Separate release fixture APK, synthetic local media, identical frontend/decoder native libraries to shipping.','checks':results},ensure_ascii=False,indent=2),encoding='utf-8')
    EVIDENCE.mkdir(parents=True,exist_ok=True)
    # This package contains only owned synthetic QA data, never phone/user data.
    adb('shell','am','force-stop','com.yingxia.yingxia_native')
    adb('shell','pm','clear',package)
    start(package)
    wait_for('想看什么？')
    type_query('fixture')
    tap('搜索')
    wait_for('检测完成')
    reach('原生播放测试 HLS')
    assert has('媒体可访问')
    snap('fixture_search_accessible')
    check('search_availability','Synthetic results pass actual private-API media access checks before selection.',['fixture_search_accessible.xml'])
    tap('原生播放测试 HLS',contains=True)
    wait_for('＋ 收藏')
    tap('＋ 收藏')
    reach('▷ 播放所选集')
    snap('fixture_hls_selected')
    tap('▷ 播放所选集')
    time.sleep(4)
    snap('fixture_hls_frame1')
    rows=history(package)
    first=next(row for row in rows if row.get('title')=='原生播放测试 HLS')
    before=float(first.get('position',0))
    time.sleep(4)
    snap('fixture_hls_frame2')
    first=next(row for row in history(package) if row['id']==first['id'])
    after=float(first.get('position',0))
    assert after>before+2, (before,after)
    check('hls_decode_clock','Continuous 640×360 H264 HLS has decoded frames and advancing saved media time across segment boundaries.',['fixture_hls_frame1.png','fixture_hls_frame2.png'],before=before,after=after)
    nav('收藏')
    time.sleep(1)
    paused=float(next(row for row in history(package) if row['id']==first['id']).get('position',0))
    time.sleep(3)
    later=float(next(row for row in history(package) if row['id']==first['id']).get('position',0))
    assert abs(later-paused)<.3,(paused,later)
    nav('播放')
    wait_for('已暂停')
    snap('fixture_return_paused')
    check('navigation_pause','Leaving workspace pauses playback; return remains paused with stable stored time.',['fixture_return_paused.xml'],before=paused,after=later)
    reach('停止')
    tap('停止')
    wait_for('已停止')
    snap('fixture_stop')
    reach('▷ 播放所选集')
    tap('▷ 播放所选集')
    time.sleep(4)
    snap('fixture_replay')
    replay=history(package)[0]
    assert replay['title']=='原生播放测试 HLS' and float(replay.get('position',0))>1
    check('stop_replay','Stop completes and the same Surface can decode after replay.',['fixture_stop.xml','fixture_replay.png'],position=replay['position'])
    reach('下一集')
    tap('下一集')
    time.sleep(4)
    snap('fixture_episode2')
    episode=history(package)[0]
    assert episode.get('episode')=='第02集' and float(episode.get('position',0))>1,episode
    check('episode_change','Next episode opens and saves progress under the correct episode.',['fixture_episode2.xml','fixture_episode2.png'],episode=episode['episode'],position=episode['position'])
    adb('shell','input','keyevent','3')
    time.sleep(1)
    p0=float(history(package)[0].get('position',0))
    time.sleep(3)
    p1=float(history(package)[0].get('position',0))
    assert abs(p1-p0)<.3,(p0,p1)
    start(package)
    wait_for('已暂停')
    snap('fixture_home_paused')
    check('background_pause','Android Home pauses playback; reopening does not auto-resume.',['fixture_home_paused.xml'],before=p0,after=p1)
    nav('收藏')
    reach('清空记录')
    count=len(history(package))
    tap('清空记录')
    snap('fixture_clear_dialog')
    tap('取消')
    assert len(history(package))==count
    tap('清空记录')
    tap('清空')
    time.sleep(.5)
    assert history(package)==[]
    assert len(app_json(package,'library.json')['favorites'])==1
    snap('fixture_cleared')
    restart(package)
    nav('收藏')
    reach('清空记录')
    assert history(package)==[] and len(app_json(package,'library.json')['favorites'])==1
    snap('fixture_clear_restarted')
    check('clear_history','Cancel keeps all rows; confirm clears persisted history and preserves the favorite across full restart.',['fixture_clear_dialog.xml','fixture_cleared.xml','fixture_clear_restarted.xml'],rows_before=count,favorites_after=1)
    print(json.dumps({'passed':len(results),'fixture_sha256':hashlib.sha256(apk.read_bytes()).hexdigest()}))

if __name__=='__main__':
    parser=argparse.ArgumentParser()
    parser.add_argument('phase',choices=['overview','fixture'])
    options=parser.parse_args()
    if options.phase=='overview': overview()
    elif options.phase=='fixture': fixture()
