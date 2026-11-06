"""Regenerate the offline TV input asset from pinned, licensed dictionaries."""

import argparse
from collections import defaultdict
from hashlib import sha256
import json
from pathlib import Path
import re
from urllib.request import urlopen
import unicodedata


CLIENT = Path(__file__).resolve().parents[1]
GO_PINYIN = CLIENT.parent / 'native_android_core/vendor/github.com/mozillazg/go-pinyin'
RIME_COMMIT = '0c6861ef7420ee780270ca6d993d18d4101049d0'
RIME_ROOT = f'https://raw.githubusercontent.com/rime/rime-pinyin-simp/{RIME_COMMIT}/'
RIME_FILES = {
    'pinyin_simp.dict.yaml': 'e341598343a0f0f2035bb1aafc34a7f3bb7887deeecb3f60796262aaa2983e6b',
    'LICENSE': 'cfc7749b96f63bd31c3c42b5c471bf756814053e847c10f3eb003417bc523d30',
    'AUTHORS': 'f4cff0fcbca4668ac449c24a53be547e162bc60cce63fdc5d5906801a452edc4',
}
# Project-authored convenience entries; words and single characters are not
# restricted to these titles. Alternate spellings help when a name is polyphonic.
TITLE_EXAMPLES = {
    'liulangdiqiu': '流浪地球', 'ne zha': '哪吒', 'na zha': '哪吒',
    'nezhazhimo tong nao hai': '哪吒之魔童闹海',
    'nazhazhimo tong nao hai': '哪吒之魔童闹海',
    'nezha zhi mo tong jiang shi': '哪吒之魔童降世',
    'nazha zhi mo tong jiang shi': '哪吒之魔童降世',
    'xingjichuanyue': '星际穿越', 'haizeiwang': '海贼王',
    'mingzhentankenan': '名侦探柯南', 'gui miezhi ren': '鬼灭之刃',
    'zhang song de fu li lian': '葬送的芙莉莲', 'dahuaxiyou': '大话西游',
    'qingyunian': '庆余年', 'kuangbiao': '狂飙', 'santi': '三体',
    'zhenhuanzhuan': '甄嬛传', 'langyabang': '琅琊榜',
    'hongloumeng': '红楼梦', 'xiyouji': '西游记',
    'sanguoyanyi': '三国演义', 'shuihuzhuan': '水浒传',
    'guoqing': '国庆', 'zhongguo': '中国', 'nihao': '你好',
}
GO_ROW = re.compile(r'^\s*(0x[\dA-Fa-f]+):\s*"([^"\\]+)",', re.M)
SYLLABLE = re.compile(r'[a-z]{1,6}\Z')
RIME_WORD = re.compile(r'[\u3007\u3400-\u9fff]+\Z')


def plain_pinyin(raw):
    # Preserve diaeresis as v before removing tone marks.
    marked = unicodedata.normalize('NFD', raw.strip().lower()).replace('u\u0308', 'v')
    return ''.join(c for c in marked if not unicodedata.combining(c)).replace('u:', 'v')


def read_rime(name, source):
    if source is not None:
        data = (source / name).read_bytes()
    else:
        with urlopen(RIME_ROOT + name, timeout=20) as response:
            data = response.read(2_000_000)
    assert sha256(data).hexdigest() == RIME_FILES[name], f'Unexpected Rime {name} bytes'
    return data


def generate(rime, go):
    chars = defaultdict(dict)
    words = defaultdict(dict)
    for row in rime.decode('utf-8-sig').partition('\n...')[2].splitlines():
        columns = row.split('\t')
        if len(columns) != 3:
            continue
        value, code, weight = columns
        syllables = plain_pinyin(code).split()
        if not RIME_WORD.fullmatch(value) or not syllables or not weight.isdecimal():
            continue
        if not all(SYLLABLE.fullmatch(s) for s in syllables):
            continue
        key = ''.join(syllables)
        target = chars if len(value) == 1 else words
        entry = target[key]
        entry[value] = max(int(weight), entry.get(value, -1))

    # The Go table extends single-character coverage; Rime's weighted entries
    # stay ahead of supplemental readings. Keep Unicode order for unknown ranks.
    for point, codes in GO_ROW.findall(go.decode('utf-8-sig')):
        char = chr(int(point, 16))
        for raw in codes.split(','):
            code = plain_pinyin(raw)
            if SYLLABLE.fullmatch(code):
                chars[code].setdefault(char, -1)

    for pronunciation, phrase in TITLE_EXAMPLES.items():
        words[pronunciation.replace(' ', '')][phrase] = 10**12

    def ordered(entries):
        return sorted(entries, key=lambda value: (-entries[value], value))

    assert len(chars) >= 400 and len(words) >= 25_000, 'Invalid or incomplete dictionary source'
    assert '你' in chars['ni'] and '球' in chars['qiu'] and '绿' in chars['lv']
    return {
        'format': 1,
        'notice': 'Derived and modified from Rime pinyin-simp and go-pinyin; see NOTICE.txt.',
        'chars': {key: ''.join(ordered(rows)) for key, rows in sorted(chars.items())},
        'words': {key: ordered(rows) for key, rows in sorted(words.items())},
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--rime-source', type=Path, help='Directory of already downloaded, pinned Rime files.')
    parser.add_argument('--output', type=Path, default=CLIENT / 'assets/tv_input')
    args = parser.parse_args()
    out = args.output
    go = (GO_PINYIN / 'pinyin_dict.go').read_bytes()
    dictionary = generate(read_rime('pinyin_simp.dict.yaml', args.rime_source), go)
    out.mkdir(parents=True, exist_ok=True)
    serialized = json.dumps(dictionary, ensure_ascii=False, separators=(',', ':')).encode('utf-8') + b'\n'
    (out / 'pinyin.json').write_bytes(serialized)
    (out / 'LICENSE.rime-pinyin-simp.txt').write_bytes(read_rime('LICENSE', args.rime_source))
    (out / 'LICENSE.go-pinyin.txt').write_bytes((GO_PINYIN / 'LICENSE').read_bytes())
    authors = read_rime('AUTHORS', args.rime_source).decode('utf-8').strip()
    notice = f'''映匣：电视遥控器拼音字典 / YingXia TV Pinyin lookup data

The bundled pinyin.json is modified and generated from these inputs:

1. Rime pinyin-simp, Apache License 2.0, {RIME_COMMIT}
   https://github.com/rime/rime-pinyin-simp/tree/{RIME_COMMIT}
   pinyin_simp.dict.yaml SHA-256: {RIME_FILES['pinyin_simp.dict.yaml']}
   Upstream AUTHORS:
{authors}

2. go-pinyin v0.21.0, MIT, Copyright (c) 2016 mozillazg
   https://github.com/mozillazg/go-pinyin/tree/v0.21.0
   Input: native_android_core/vendor/github.com/mozillazg/go-pinyin/pinyin_dict.go
   Input SHA-256 (as checked out): {sha256(go).hexdigest()}

Changes: invert character readings, remove tone marks, normalize ü to v,
combine and rank pinyin lookup entries, and add a small project-maintained set
of film-title spellings. The upstream source files are unmodified.

To regenerate this asset, run native_android_client/tools/generate_tv_pinyin.py
from the project checkout with Python 3. The script uses pinned, hash-verified
upstream files; --rime-source supplies those same files for an offline build.

Output pinyin.json SHA-256: {sha256(serialized).hexdigest()}
'''
    (out / 'NOTICE.txt').write_bytes(notice.encode('utf-8'))
    print(f"{len(dictionary['chars'])} syllable keys, {len(dictionary['words'])} phrase keys, {len(serialized)} bytes")


if __name__ == '__main__':
    main()
