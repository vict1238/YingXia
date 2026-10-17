"""Pure, independently testable metadata parsing. No automatic same-title merges."""
from __future__ import annotations
from dataclasses import dataclass, field, asdict
from html import unescape
from html.parser import HTMLParser
import re
import unicodedata
from urllib.parse import urlsplit

class AppError(Exception):
    def __init__(self, message: str, code: str = 'error', status: int = 400):
        super().__init__(message)
        self.message, self.code, self.status = message, code, status

class _Text(HTMLParser):
    def __init__(self):
        super().__init__(); self.parts = []; self.hidden = 0
    def handle_starttag(self, tag, attrs):
        if tag in ('script', 'style'): self.hidden += 1
        if tag in ('p', 'br', 'div', 'li'): self.parts.append(' ')
    def handle_endtag(self, tag):
        if tag in ('script', 'style'): self.hidden = max(0, self.hidden - 1)
    def handle_data(self, data):
        if not self.hidden: self.parts.append(data)

def text(value, limit: int = 1500) -> str:
    parser = _Text()
    try:
        parser.feed(str(value or '')[:20000]); raw = ''.join(parser.parts)
    except Exception:
        raw = re.sub(r'<[^>]*>', '', str(value or ''))
    return re.sub(r'\s+', ' ', unescape(raw)).strip()[:limit]

SITES = r'(?:腾讯视频|爱奇艺|优酷|芒果TV|哔哩哔哩|bilibili|搜狐视频|腾讯|B站)'
NUM = r'[0-9零〇一二两三四五六七八九十百千]+'

def normalize_title(value: str) -> str:
    """Keep English spaces and seasons. Strip only boundary platform boilerplate."""
    s = unicodedata.normalize('NFKC', text(value, 250))
    s = re.sub(r'\s*[-_|｜·:：]\s*' + SITES + r'(?:\s*[-_|｜].*)?$', '', s, flags=re.I)
    s = re.sub(r'^' + SITES + r'\s*[-_|｜:：]\s*', '', s, flags=re.I)
    s = re.sub(r'第\s*' + NUM + r'\s*[集话期](?:\s*[上下中])?', '', s)
    s = re.sub(r'\s*[-_|｜]\s*(?:在线观看|高清正版|免费观看|完整版|全集)\s*$', '', s)
    s = s.replace('《', '').replace('》', '').strip(' -_|｜')
    return re.sub(r'\s+', ' ', s)

def number(value: str) -> int | None:
    value = unicodedata.normalize('NFKC', str(value).strip())
    if value.isdecimal():
        n = int(value); return n if n < 100000 else None
    digits = dict(zip('零〇一二两三四五六七八九', [0,0,1,2,2,3,4,5,6,7,8,9]))
    units = {'十':10,'百':100,'千':1000}
    if not value or any(c not in digits and c not in units for c in value): return None
    if all(c in digits for c in value): return int(''.join(str(digits[c]) for c in value))
    result, current = 0, 0
    for c in value:
        if c in digits: current = digits[c]
        else: result += (current or 1) * units[c]; current = 0
    return result + current

def season(value: str) -> int | None:
    m = re.search(r'第\s*(' + NUM + r')\s*季', value)
    if m: return number(m.group(1))
    m = re.search(r'\b(?:season\s*|s)(\d{1,3})\b', value, re.I)
    return int(m[1]) if m else None

def episode_number(label: str, url: str = '') -> int | None:
    """A platform ep/ss/vid is an identifier, NEVER an ordinal. URL deliberately unused."""
    label = unicodedata.normalize('NFKC', text(label, 200))
    if re.search(r'预告|花絮|番外|特别|SP\b|trailer|special', label, re.I): return None
    m = re.search(r'第\s*(' + NUM + r')\s*[集话期]', label)
    if not m: m = re.search(r'\b(?:episode|ep|e)\s*0*(\d{1,4})\b', label, re.I)
    if not m: m = re.fullmatch(r'(' + NUM + r')(?:\s*[集话期])?', label)
    return number(m.group(1)) if m else None

def ranking(query: str, title: str, year: str = '') -> int:
    q, t = normalize_title(query).casefold(), normalize_title(title).casefold()
    qs, ts = season(q), season(t)
    score = 100 if q == t else 65 if q in t else 30 if t in q else 0
    if qs is not None and ts != qs: score -= 90
    y = re.search(r'\b(19\d{2}|20\d{2})\b', query)
    if y and str(year): score += 15 if y[1] == str(year) else -30
    return score

@dataclass
class Episode:
    key: str
    name: str
    url: str
    ordinal: int | None = None
    def public(self): return {'key':self.key, 'name':self.name, 'ordinal':self.ordinal}

@dataclass
class Group:
    key: str
    name: str
    episodes: list[Episode] = field(default_factory=list)
    def public(self): return {'key':self.key, 'name':self.name, 'episodes':[e.public() for e in self.episodes]}

@dataclass
class Film:
    id: str
    title: str
    year: str = ''
    kind: str = ''
    area: str = ''
    remarks: str = ''
    summary: str = ''
    actors: str = ''
    director: str = ''
    source: str = '无损云目录（未实播验证）'
    groups: list[Group] = field(default_factory=list)
    demo: bool = False
    def public(self):
        d = asdict(self); d['groups'] = [g.public() for g in self.groups]; return d


def groups_from_vod(value: str, names: str = '') -> list[Group]:
    if not isinstance(value, str): return []
    labels = str(names or '').split('$$$')
    groups = []
    for gi, raw in enumerate(value.split('$$$')[:40]):
        episodes = []
        for ei, item in enumerate(raw.split('#')[:3000]):
            name, sep, url = item.partition('$')
            if not sep: name, url = '', name
            url = url.strip()
            try:
                p = urlsplit(url)
                if p.scheme not in ('http','https') or not p.hostname or p.username or p.password: continue
                if any(c in url for c in ('\n','\r','\x00','\\')): continue
            except ValueError: continue
            label = text(name, 120) or f'条目 {ei + 1}'
            episodes.append(Episode(f'e{ei}', label, url, episode_number(label)))
        if episodes:
            groups.append(Group(f'g{gi}', text(labels[gi],100) if gi<len(labels) and labels[gi] else f'来源组 {gi+1}', episodes))
    return groups


def film_from_vod(row: dict) -> Film:
    ident = str(row.get('vod_id') or row.get('id') or '')
    if not re.fullmatch(r'\d{1,15}', ident): raise AppError('目录返回了无效影片编号。', 'bad_catalog')
    return Film(id=ident, title=text(row.get('vod_name') or row.get('name'),200),
                year=text(row.get('vod_year'),20), kind=text(row.get('type_name'),50),
                area=text(row.get('vod_area'),50), remarks=text(row.get('vod_remarks'),100),
                summary=text(row.get('vod_content')), actors=text(row.get('vod_actor'),300),
                director=text(row.get('vod_director'),150),
                groups=groups_from_vod(row.get('vod_play_url',''),row.get('vod_play_from','')))


def choose_episode(group: Group, name: str) -> Episode:
    exact = [e for e in group.episodes if e.name == name]
    if len(exact) == 1: return exact[0]
    # Only unambiguous plain episode labels may match by ordinal. Never drop 上/下/预告.
    n = episode_number(name)
    if re.search(r'上|下|中|预告|花絮|番外|SP|特别', name, re.I): n = None
    same = [e for e in group.episodes if n is not None and e.ordinal == n and not re.search(r'上|下|中|预告|花絮|番外', e.name)]
    if len(same) == 1: return same[0]
    raise AppError('目标来源没有可唯一匹配的这一集，请手动选择；不会自动跳到第一集。','episode_missing')
