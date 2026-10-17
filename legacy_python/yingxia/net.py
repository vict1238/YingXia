"""Bounded HTTP requests; no account cookies, no TLS downgrades, no remote code."""
from __future__ import annotations
import json
import socket
import ssl
import time
import urllib.error
import urllib.request
from urllib.parse import urlencode
from .models import AppError
from .security import public_url, redacted_url

class SafeRedirect(urllib.request.HTTPRedirectHandler):
    max_redirections = 4
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        public_url(newurl, resolve=True)
        return super().redirect_request(req,fp,code,msg,headers,newurl)

class Network:
    def __init__(self, timeout: float = 9): self.timeout = timeout
    def fetch(self, url: str, limit: int = 4_000_000) -> tuple[bytes,dict]:
        url = public_url(url, resolve=True)
        started = time.monotonic()
        req = urllib.request.Request(url, headers={
            'User-Agent':'Mozilla/5.0 (Windows NT 10.0; Win64; x64) YingXia/1.0',
            'Accept':'application/json,text/html;q=0.8,*/*;q=0.1', 'Accept-Encoding':'identity'})
        # The default ProxyHandler respects OS proxy settings, including Windows registry.
        opener = urllib.request.build_opener(SafeRedirect(),urllib.request.HTTPSHandler(context=ssl.create_default_context()))
        try:
            with opener.open(req,timeout=self.timeout) as res:
                chunks, count = [], 0
                while True:
                    if time.monotonic()-started>self.timeout+3:
                        raise AppError('读取响应超时。请稍后重试或换来源。','timeout',504)
                    block = res.read(min(65536,limit+1-count))
                    if not block: break
                    count += len(block)
                    if count>limit: raise AppError('来源响应过大，已停止读取。','too_large',502)
                    chunks.append(block)
                return b''.join(chunks),{'http':res.status,'content_type':res.headers.get('Content-Type',''),
                    'endpoint':redacted_url(res.url),'elapsed_ms':round((time.monotonic()-started)*1000)}
        except AppError: raise
        except urllib.error.HTTPError as e:
            raise AppError(f'来源返回 HTTP {e.code}；可能是限流、地区或服务限制。','http',502) from e
        except (TimeoutError,socket.timeout): raise AppError('连接超时，请检查网络或换来源。','timeout',504)
        except (urllib.error.URLError,OSError,ssl.SSLError) as e:
            reason=str(getattr(e,'reason',e))
            if 'CERTIFICATE' in reason.upper():
                raise AppError('TLS 证书校验失败。请检查系统时间或代理证书；程序不会跳过证书验证。','certificate',502) from e
            raise AppError('请求未完成。请检查本机网络、DNS、系统代理或来源状态。','network',502) from e

    def json(self, base: str, params: dict | None = None):
        url=base+('?' + urlencode(params) if params else '')
        raw,_=self.fetch(url)
        try:
            obj=json.loads(raw.decode('utf-8-sig'))
        except (ValueError,UnicodeError):
            raise AppError('接口返回的不是有效 JSON，可能是验证页、广告页或接口已变化。','bad_json',502)
        if not isinstance(obj,dict): raise AppError('接口数据结构不符合预期。','bad_schema',502)
        return obj

    def probe(self,name:str,url:str) -> dict:
        row={'name':name,'endpoint':redacted_url(url),'playback_verified':False,'state':'未完成'}
        try:
            raw,meta=self.fetch(url,2_000_000); row.update(meta)
            txt=raw.decode('utf-8',errors='replace')
            row['state']='HTTP 可读取；未验证视频播放'
            try:
                data=json.loads(txt)
                if isinstance(data,dict): row['state']='JSON 可解析；未验证匹配或播放'
            except ValueError: pass
            if '江湖路远，后会无期' in txt or '此去一别，不再相逢' in txt:
                row['state']='入口含告别文案；疑似异常，未启用'
        except AppError as e: row.update(state=e.message,error=e.code)
        return row
