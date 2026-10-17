#!/usr/bin/env python3
"""Windows-friendly source preflight (Python 3.10+, standard library only).
Checks HTTP/catalog responses. NEVER labels HTML or HTTP 200 as playable video.
No login, no cookies imported, no remote JavaScript, no DRM processing.
"""
from __future__ import annotations
import argparse
import concurrent.futures
import datetime as dt
import html
import json
import pathlib
import re
import socket
import ssl
import sys
import urllib.error
import urllib.parse
import urllib.request

SOURCES = [
    ('麒麟解析', 'https://free.maccms.xyz/?url='),
    ('TXNQ解析', 'https://bfq.txnp.cn/player?url='),
    ('七哥解析', 'https://jx.202617.xyz/tv.php?url='),
    ('fongmi解析', 'https://json.fongmi.cc/web?url='),
    ('冰豆解析', 'https://bd.jx.cn/?url='),
    ('HLS解析', 'https://jx.hls.one/?url='),
    ('Player-JY', 'https://jx.playerjy.com/?url='),
]
OFFICIAL_HOSTS = {'v.qq.com', 'm.v.qq.com', 'www.iqiyi.com', 'm.iqiyi.com',
    'www.iq.com', 'v.youku.com', 'm.youku.com', 'www.bilibili.com',
    'm.bilibili.com', 'www.mgtv.com', 'm.mgtv.com'}
SEARCH = 'https://wsyzy.cc/index.php/ajax/suggest?'
API = 'https://api.wsyzy.net/api.php/provide/vod/'
MAX_BYTES = 2_000_000


def validate_platform_url(value: str) -> str:
    """Optional external forwarding accepts only explicit official page URLs."""
    if not value:
        return ''
    p = urllib.parse.urlsplit(value)
    if p.scheme != 'https' or p.hostname not in OFFICIAL_HOSTS or p.username or p.password:
        raise ValueError('平台地址必须是支持的平台 HTTPS 播放页，不能包含用户名或密码。')
    if p.port not in (None, 443):
        raise ValueError('不允许非标准端口。')
    if any(c in value for c in ('\r', '\n', '\x00')):
        raise ValueError('地址包含非法字符。')
    return urllib.parse.urlunsplit((p.scheme, p.netloc, p.path, p.query, ''))


def request(name: str, url: str, timeout: float) -> dict:
    result = {'name': name, 'url': url, 'observed_at': dt.datetime.now(dt.timezone.utc).isoformat(),
              'playback_verified': False}
    req = urllib.request.Request(url, headers={
        'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) SourcePreflight/0.1',
        'Accept': 'application/json,text/html;q=0.9,*/*;q=0.5'})
    try:
        with urllib.request.urlopen(req, timeout=timeout, context=ssl.create_default_context()) as response:
            raw = response.read(MAX_BYTES + 1)
            if len(raw) > MAX_BYTES:
                raise ValueError('响应超过 2 MB，已停止读取。')
            charset = response.headers.get_content_charset() or 'utf-8'
            try:
                text = raw.decode(charset, errors='replace')
            except LookupError:
                text = raw.decode('utf-8', errors='replace')
            result.update(http_status=response.status, final_url=response.url,
                          content_type=response.headers.get('Content-Type', ''), bytes=len(raw))
            # HTML is not evidence of video playback.
            result['state'] = 'HTTP 响应可读取；未验证播放'
            result['excerpt'] = re.sub(r'\s+', ' ', re.sub(r'<[^>]*>', ' ', text))[:240]
            if '此去一别，不再相逢' in text or '江湖路远，后会无期' in text:
                result['state'] = '入口含告别文案；待核验，不标记可用'
            try:
                data = json.loads(text)
                if isinstance(data, (dict, list)):
                    result['json'] = data
                    result['state'] = 'JSON 可解析；未验证匹配或播放'
            except (json.JSONDecodeError, ValueError):
                pass
    except urllib.error.HTTPError as e:
        result.update(http_status=e.code, state='HTTP 请求失败；不代表所有网络均失效', error=str(e))
    except (urllib.error.URLError, TimeoutError, socket.timeout, OSError, ValueError) as e:
        result.update(state='请求未完成；网络环境或服务原因尚未区分', error=str(e))
    return result


def parse_source_groups(value: str) -> list[dict]:
    """Preserve every group. Parse metadata only; do not fetch media addresses."""
    groups = []
    for i, group in enumerate((value or '').split('$$$')):
        episodes = []
        for item in group.split('#'):
            name, sep, url = item.partition('$')
            if not sep:
                name, url = '', name
            p = urllib.parse.urlsplit(url)
            if p.scheme in ('http', 'https') and p.hostname and not p.username:
                episodes.append({'name': name, 'url': url})
        if episodes:
            groups.append({'group_index': i, 'episodes': episodes})
    return groups


def save_report(path: pathlib.Path, results: list[dict], title: str, platform: str) -> None:
    path.mkdir(parents=True, exist_ok=True)
    control_results = [x for x in results if x['name'].startswith('网络对照')]
    controls_failed = bool(control_results) and all('error' in x for x in control_results)
    report = {'version': 'preflight-0.1', 'title': title, 'platform_url_supplied': bool(platform),
              'source_blob': '30260d8fb3db47b4f14fcc1a3faf8efadf99a43a',
              'controls_all_failed': controls_failed,
              'scope': 'HTTP/catalog checks ONLY; no browser execution, no decoding, no verified playback.',
              'results': results}
    (path / 'report.json').write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding='utf-8')
    lines = ['# 线路预检报告', '', '**本报告不能证明影片已经能播放。**', '',
             f'搜索词：{title}', f'提供平台播放页：{"是" if platform else "否；仅检查入口"}', '',
             '| 项目 | HTTP | 结果 |', '|---|---:|---|']
    for r in results:
        lines.append(f"| {r['name']} | {r.get('http_status', '—')} | {r['state']} |")
    if controls_failed:
        lines += ['', '**普通网站对照请求也全部失败，先检查本机网络、DNS、代理或证书；不能把所有来源判成失效。**']
    lines += ['', '只确认响应、JSON 和目录格式。匹配正确性需要核对年份、季数、版本；实际播放需要浏览器解码证据。',
              '详细错误和候选数据位于 report.json。报告可能包含你输入的平台网址，分享前请删除私人参数。']
    (path / 'report.md').write_text('\n'.join(lines), encoding='utf-8')
    (path / 'report.txt').write_text('\n'.join(lines), encoding='utf-8-sig')


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--title', default='三体', help='无损云目录搜索词，不代表指定结果一定匹配')
    parser.add_argument('--platform-url', default='', help='可选；将此播放页 URL 转交给七条第三方线路。不要含私人参数。')
    parser.add_argument('--timeout', type=float, default=8)
    parser.add_argument('--output', default='local_check_results')
    parser.add_argument('--details', action='store_true', help='读取最多 3 个候选的详情，不播放、不抓取媒体')
    args = parser.parse_args(argv)
    if not (1 <= args.timeout <= 30):
        parser.error('--timeout 必须为 1 到 30 秒。')
    if not 1 <= len(args.title.strip()) <= 120:
        parser.error('搜索词长度必须为 1 到 120。')
    try:
        platform = validate_platform_url(args.platform_url)
    except ValueError as exc:
        parser.error(str(exc))
    print('线路预检 0.1：仅检查网络和目录，不把 HTTP 200 当成播放成功。', flush=True)
    if platform:
        print('将把所提供的平台网址发送给七条第三方线路。', flush=True)
    jobs = [('网络对照 example.com', 'https://example.com/'),
            ('网络对照 python.org', 'https://www.python.org/'),
            ('无损云搜索', SEARCH + urllib.parse.urlencode({'mid': 1, 'wd': args.title})),
            ('无损云 API 入口', API), ('无损云播放器入口', 'https://wsyzy.vip/m3u8/')]
    jobs += [(name, base + urllib.parse.quote(platform, safe='') if platform else base.split('?')[0])
             for name, base in SOURCES]
    results = []
    with concurrent.futures.ThreadPoolExecutor(max_workers=3) as pool:
        futures = {pool.submit(request, name, url, args.timeout): name for name, url in jobs}
        for future in concurrent.futures.as_completed(futures):
            result = future.result()
            if result['name'] == '无损云搜索' and isinstance(result.get('json'), dict):
                candidates = result['json'].get('list', [])
                if isinstance(candidates, list):
                    candidates = [x for x in candidates if isinstance(x, dict) and x.get('id') and x.get('name')]
                    result['candidates'] = candidates
                    result['state'] = f'返回 {len(candidates)} 个候选；未验证同名匹配/播放'
            results.append(result)
            print(f"{result['name']}: {result['state']}", flush=True)
    if args.details:
        search_result = next((x for x in results if x['name'] == '无损云搜索'), {})
        for candidate in search_result.get('candidates', [])[:3]:
            url = API + '?' + urllib.parse.urlencode({'ac': 'detail', 'ids': str(candidate['id'])})
            r = request('候选详情 ' + str(candidate['name'])[:80], url, args.timeout)
            payload = r.get('json')
            if isinstance(payload, dict) and isinstance(payload.get('list'), list):
                for entry in payload['list']:
                    if isinstance(entry, dict):
                        groups = parse_source_groups(str(entry.get('vod_play_url') or ''))
                        r.setdefault('metadata', []).append({
                            'title': entry.get('vod_name'), 'year': entry.get('vod_year'),
                            'type': entry.get('type_name'), 'group_count': len(groups),
                            'episodes_per_group': [len(g['episodes']) for g in groups]})
            results.append(r)
    output = pathlib.Path(args.output).resolve()
    save_report(output, results, args.title, platform)
    print(f'报告已保存：{output / "report.md"}')
    print('重要：没有进行视频解码验证，因此本次没有任何线路被标记为「已确认可播放」。')
    return 0


if __name__ == '__main__':
    try:
        raise SystemExit(main())
    except KeyboardInterrupt:
        print('\n已取消。')
        raise SystemExit(130)
