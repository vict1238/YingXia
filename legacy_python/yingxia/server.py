"""Loopback-only application server. Session + CSRF + Origin + Host validation."""
from __future__ import annotations
from http.server import ThreadingHTTPServer,BaseHTTPRequestHandler
from http.cookies import SimpleCookie
from pathlib import Path
from urllib.parse import urlsplit,parse_qs,unquote
import json
import logging
import mimetypes
import re
import secrets
import threading
import time
from .models import AppError
from .service import Service,ROOT

LOG=logging.getLogger('yingxia')

class Server(ThreadingHTTPServer):
    daemon_threads=True
    def __init__(self,service:Service,port:int=0):
        super().__init__(('127.0.0.1',port),Handler)
        self.service=service;self.token=secrets.token_urlsafe(32);self.session=secrets.token_urlsafe(32)
        self.origin=f'http://127.0.0.1:{self.server_address[1]}'
        self.cookie_name=f'yx_session_{self.server_address[1]}'
        self.last_seen=time.monotonic();self.had_client=False
        self.network_slots=threading.BoundedSemaphore(4)
    def shutdown_later(self): threading.Thread(target=self.shutdown,daemon=True).start()

class Handler(BaseHTTPRequestHandler):
    protocol_version='HTTP/1.1'
    server_version='YingXia/1.0';sys_version=''
    def log_message(self,fmt,*args):
        # Deliberately avoid logging full URLs, query strings and POST payloads.
        LOG.debug('local request %s',self.command)
    def _headers(self,status,ctype,length,extra=None):
        self.send_response(status)
        self.send_header('Content-Type',ctype);self.send_header('Content-Length',str(length))
        self.send_header('Cache-Control','no-store')
        self.send_header('X-Content-Type-Options','nosniff')
        self.send_header('Referrer-Policy','no-referrer')
        self.send_header('Cross-Origin-Resource-Policy','same-origin')
        self.send_header('X-Frame-Options','DENY')
        self.send_header('Permissions-Policy','camera=(), microphone=(), geolocation=(), payment=(), usb=()')
        self.send_header('Content-Security-Policy',
            "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; "
            "connect-src 'self'; media-src 'self' https: http: blob:; frame-src https: http:; "
            "object-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
        for k,v in (extra or {}).items():self.send_header(k,v)
        self.end_headers()
    def reply(self,data,status=200):
        raw=json.dumps(data,ensure_ascii=False,allow_nan=False).encode('utf-8')
        self._headers(status,'application/json; charset=utf-8',len(raw));self.wfile.write(raw)
    def fail(self,err):
        try:self.reply({'ok':False,'error':err.code,'message':err.message},err.status)
        except (BrokenPipeError,ConnectionResetError):pass
    def common(self):
        if self.headers.get('Host','')!=self.server.origin.split('//',1)[1]:
            raise AppError('Host 校验失败。','host',403)
        origin=self.headers.get('Origin')
        if origin and origin!=self.server.origin:raise AppError('跨站请求已拒绝。','origin',403)
        if self.headers.get('Sec-Fetch-Site')=='cross-site':raise AppError('跨站请求已拒绝。','origin',403)
    def authenticated(self,write=False):
        try:
            cookies=SimpleCookie();cookies.load(self.headers.get('Cookie',''))
            candidate=cookies[self.server.cookie_name].value if self.server.cookie_name in cookies else ''
        except Exception:candidate=''
        if not secrets.compare_digest(candidate,self.server.session):raise AppError('会话失效，请重新打开软件。','session',403)
        if write and not secrets.compare_digest(self.headers.get('X-YingXia-CSRF',''),self.server.token):
            raise AppError('安全令牌失效，请刷新软件。','csrf',403)
        self.server.last_seen=time.monotonic();self.server.had_client=True
    def do_OPTIONS(self):
        self.send_error(405)
    def do_GET(self):
        try:
            self.common();p=urlsplit(self.path);path=unquote(p.path)
            if path=='/bootstrap.js':
                self.authenticated()
                raw=('window.YX_BOOT='+json.dumps({'csrf':self.server.token,'origin':self.server.origin})+';').encode()
                self._headers(200,'text/javascript; charset=utf-8',len(raw));self.wfile.write(raw);return
            if path.startswith('/api/'):
                self.authenticated();s=self.server.service
                if path=='/api/state':data=s.state()
                elif path=='/api/detail':data=s.detail_public(parse_qs(p.query).get('id',[''])[0])
                elif path=='/api/library':data={'favorites':s.store.favorites(),'history':s.store.history()}
                elif path=='/api/bindings':
                    q=parse_qs(p.query);data=s.store.bindings(q.get('id',[''])[0],q.get('episode',[''])[0])
                elif path=='/api/report':data=s.export_report()
                else:raise AppError('接口不存在。','not_found',404)
                self.reply({'ok':True,'data':data});return
            web=ROOT/'web';asset=(web/('index.html' if path=='/' else path.lstrip('/'))).resolve()
            if not asset.is_relative_to(web.resolve()) or not asset.is_file():raise AppError('文件不存在。','not_found',404)
            if path!='/':self.authenticated()
            extra={}
            if path=='/':
                extra['Set-Cookie']=f'{self.server.cookie_name}={self.server.session}; HttpOnly; SameSite=Strict; Path=/'
                self.server.had_client=True;self.server.last_seen=time.monotonic()
            size=asset.stat().st_size;start=0;end=size-1;status=200
            rg=self.headers.get('Range','')
            if rg and asset.suffix=='.webm':
                m=re.fullmatch(r'bytes=(\d+)-(\d*)',rg)
                if not m:raise AppError('不支持此 Range。','range',416)
                start=int(m[1]);end=min(int(m[2]) if m[2] else end,end)
                if start>end:raise AppError('Range 超出范围。','range',416)
                status=206;extra.update({'Content-Range':f'bytes {start}-{end}/{size}','Accept-Ranges':'bytes'})
            ctype={'.js':'text/javascript','.css':'text/css','.html':'text/html','.webm':'video/webm'}.get(asset.suffix,mimetypes.guess_type(str(asset))[0] or 'application/octet-stream')
            if asset.suffix in ('.html','.js','.css'):ctype+='; charset=utf-8'
            self._headers(status,ctype,end-start+1,extra)
            with asset.open('rb') as f:
                f.seek(start);left=end-start+1
                while left:
                    chunk=f.read(min(65536,left))
                    if not chunk:break
                    self.wfile.write(chunk);left-=len(chunk)
        except AppError as e:self.fail(e)
        except (BrokenPipeError,ConnectionResetError):pass
        except Exception:
            LOG.exception('GET handler failed');self.fail(AppError('内部错误，详情见本机日志。','internal',500))
    def do_POST(self):
        acquired=False
        try:
            self.common();self.authenticated(write=True)
            if self.headers.get_content_type()!='application/json':raise AppError('只接受 JSON。','content_type',415)
            try:n=int(self.headers.get('Content-Length','0'))
            except ValueError:raise AppError('请求大小无效。')
            if not 0<n<=65536:raise AppError('请求大小超限。','too_large',413)
            self.connection.settimeout(12)
            try:data=json.loads(self.rfile.read(n))
            except (ValueError,TimeoutError):raise AppError('请求内容不正确。','bad_json')
            if not isinstance(data,dict):raise AppError('请求内容应为对象。')
            path=urlsplit(self.path).path;s=self.server.service
            if path in ('/api/search','/api/diagnose','/api/resolve'):
                acquired=self.server.network_slots.acquire(blocking=False)
                if not acquired:raise AppError('请求较多，请等待当前查询完成。','busy',429)
            if path=='/api/search':result=s.search(data)
            elif path=='/api/settings':result=s.settings(data)
            elif path=='/api/import-platform':result=s.import_platform(data)
            elif path=='/api/bind':result=s.bind(data)
            elif path=='/api/match':result=s.match(data)
            elif path=='/api/resolve':result=s.resolve(data)
            elif path=='/api/favorite':result={'saved':s.store.toggle_favorite(s.detail(data.get('film_id','')).public())}
            elif path=='/api/progress':
                s.store.progress(str(data.get('history_id','')),data.get('evidence'),data.get('position',0));result={}
            elif path=='/api/clear-history':s.store.clear_history();result={}
            elif path=='/api/diagnose':result=s.diagnose()
            elif path=='/api/ping':result={}
            elif path=='/api/quit':result={'closing':True}
            else:raise AppError('接口不存在。','not_found',404)
            self.reply({'ok':True,'data':result})
            if path=='/api/quit':self.server.shutdown_later()
        except AppError as e:self.fail(e)
        except (BrokenPipeError,ConnectionResetError):pass
        except Exception:
            LOG.exception('POST handler failed');self.fail(AppError('内部错误，详情见本机日志。','internal',500))
        finally:
            if acquired:self.server.network_slots.release()
