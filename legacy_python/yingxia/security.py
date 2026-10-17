"""Local API and outbound URL validation. No cookies, credentials or DRM handling."""
from __future__ import annotations
import ipaddress
import socket
import re
from urllib.parse import urlsplit, urlunsplit, parse_qsl, urlencode, quote
from .models import AppError

PLATFORMS = {
 'qq': {'name':'腾讯视频','hosts':['v.qq.com','m.v.qq.com'], 'search':'https://v.qq.com/x/search/?q={query}'},
 'iqiyi': {'name':'爱奇艺','hosts':['www.iqiyi.com','m.iqiyi.com','www.iq.com'], 'search':'https://so.iqiyi.com/so/q_{query}'},
 'youku': {'name':'优酷','hosts':['v.youku.com','m.youku.com'], 'search':'https://so.youku.com/search_video/q_{query}'},
 'bilibili': {'name':'哔哩哔哩','hosts':['www.bilibili.com','m.bilibili.com'], 'search':'https://search.bilibili.com/all?keyword={query}'},
 'mgtv': {'name':'芒果TV','hosts':['www.mgtv.com','m.mgtv.com','w.mgtv.com'], 'search':'https://so.mgtv.com/so?k={query}'},
}

def public_url(value: str, resolve: bool = False) -> str:
    if not isinstance(value,str) or len(value)>4096: raise AppError('网址为空或过长。','unsafe_url')
    if any(ord(c)<32 for c in value) or '\\' in value: raise AppError('网址含非法字符。','unsafe_url')
    try:
        p = urlsplit(value.strip())
        host, port = p.hostname, p.port
    except ValueError: raise AppError('网址格式不正确。','unsafe_url')
    if p.scheme not in ('https','http') or not host or p.username or p.password:
        raise AppError('仅支持不含账号密码的 HTTP / HTTPS 网址。','unsafe_url')
    if port not in (None,80,443): raise AppError('不允许非标准网络端口。','unsafe_url')
    h = host.rstrip('.').lower()
    if h=='localhost' or h.endswith(('.localhost','.local','.internal')) or '.' not in h:
        raise AppError('不允许访问本机或内网地址。','unsafe_url')
    try:
        addr = ipaddress.ip_address(h)
        if not addr.is_global: raise AppError('不允许访问本机或内网地址。','unsafe_url')
    except ValueError:
        if re.fullmatch(r'[\d.]+',h) or h.startswith('0x'): raise AppError('不接受特殊数值 IP 地址。','unsafe_url')
    if resolve:
        try: infos=socket.getaddrinfo(h,port or (443 if p.scheme=='https' else 80),type=socket.SOCK_STREAM)
        except socket.gaierror: raise AppError('DNS 解析失败。请检查网络、DNS 或系统代理；不能据此认定片源失效。','dns',502)
        if not infos or any(not ipaddress.ip_address(info[4][0]).is_global for info in infos):
            raise AppError('目标解析到非公网地址，已阻止请求。','unsafe_url')
    return urlunsplit((p.scheme,p.netloc,p.path or '/',p.query,''))


def platform_url(value: str, expected: str | None = None) -> tuple[str,str]:
    s = public_url(value)
    p = urlsplit(s)
    if p.scheme!='https': raise AppError('平台播放页请使用 HTTPS 地址。','bad_platform')
    found = next((k for k,v in PLATFORMS.items() if p.hostname in v['hosts']),None)
    if not found or (expected and expected!=found): raise AppError('网址与所选平台不符。','bad_platform')
    paths = {'qq':r'^/(?:x/(?:cover|page|m)/|tv/)', 'iqiyi':r'^/(?:v_|w_|play/)',
             'youku':r'^/(?:v_show/|video|v_)', 'bilibili':r'^/(?:video/|bangumi/play/)', 'mgtv':r'^/b/'}
    if not re.search(paths[found],p.path): raise AppError('请粘贴具体播放页，不是搜索页或平台首页。','bad_platform')
    keep = {'vid','cid','id','tvid','albumid','p','ep','episode','cur'}
    query = urlencode([(k,v) for k,v in parse_qsl(p.query) if k.lower() in keep and len(v)<200])
    return found,urlunsplit(('https',p.netloc,p.path,query,''))


def official_search(platform: str, query: str) -> str:
    if platform not in PLATFORMS: raise AppError('请选择支持的平台。','bad_platform')
    return PLATFORMS[platform]['search'].format(query=quote(query,safe=''))


def redacted_url(value: str) -> str:
    try:
        p=urlsplit(value); return urlunsplit((p.scheme,p.netloc,p.path,'',''))
    except Exception: return '[invalid url]'
