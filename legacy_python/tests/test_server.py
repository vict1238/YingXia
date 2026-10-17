import http.client
import json
import tempfile
import threading
import unittest
from pathlib import Path
from yingxia.server import Server
from yingxia.service import Service
from .fixtures import FakeNetwork

class HTTPTests(unittest.TestCase):
 def setUp(self):
  self.tmp=tempfile.TemporaryDirectory();self.s=Service(Path(self.tmp.name),FakeNetwork());self.server=Server(self.s)
  self.thread=threading.Thread(target=self.server.serve_forever,daemon=True);self.thread.start()
  self.cookie=self.server.cookie_name+'='+self.server.session;self.origin=self.server.origin
 def tearDown(self):self.server.shutdown();self.server.server_close();self.s.store.close();self.tmp.cleanup()
 def req(self,path='/',method='GET',body=None,headers=None):
  c=http.client.HTTPConnection(*self.server.server_address,timeout=5)
  h=headers or {};raw=json.dumps(body).encode() if body is not None else None
  c.request(method,path,body=raw,headers=h);r=c.getresponse();data=r.read();result=(r.status,dict(r.getheaders()),data);c.close();return result
 def auth(self,write=False):
  h={'Cookie':self.cookie}
  if write:h.update({'Content-Type':'application/json','X-YingXia-CSRF':self.server.token,'Origin':self.origin})
  return h
 def test_root_sets_httponly_cookie(self):
  status,h,b=self.req();self.assertEqual(status,200);self.assertIn('HttpOnly',h['Set-Cookie']);self.assertIn('SameSite=Strict',h['Set-Cookie'])
 def test_root_csp(self):
  _,h,_=self.req();self.assertIn("frame-ancestors 'none'",h['Content-Security-Policy']);self.assertNotIn('unsafe-eval',h['Content-Security-Policy'])
 def test_unauth_api(self):self.assertEqual(self.req('/api/state')[0],403)
 def test_bootstrap_auth(self):self.assertEqual(self.req('/bootstrap.js')[0],403)
 def test_authenticated_api(self):self.assertEqual(self.req('/api/state',headers=self.auth())[0],200)
 def test_host_attack(self):self.assertEqual(self.req(headers={'Host':'attacker.example'})[0],403)
 def test_origin_attack(self):self.assertEqual(self.req('/api/state',headers={**self.auth(),'Origin':'https://attacker.example'})[0],403)
 def test_missing_csrf(self):self.assertEqual(self.req('/api/search','POST',{'demo':True},{**self.auth(),'Content-Type':'application/json'})[0],403)
 def test_post_fixture_demo(self):
  status,h,b=self.req('/api/search','POST',{'demo':True},self.auth(True));self.assertEqual(status,200);self.assertTrue(json.loads(b)['data']['demo'])
 def test_bad_json(self):
  # JSON array is a valid JSON value but not a valid command object.
  self.assertEqual(self.req('/api/search','POST',[],self.auth(True))[0],400)
 def test_path_traversal(self):self.assertEqual(self.req('/%2e%2e/service.py',headers=self.auth())[0],404)
 def test_media_range(self):
  status,h,b=self.req('/demo/clip1.webm',headers={**self.auth(),'Range':'bytes=0-99'});self.assertEqual(status,206);self.assertEqual(len(b),100);self.assertTrue(h['Content-Range'].startswith('bytes 0-99/'))
 def test_invalid_range(self):self.assertEqual(self.req('/demo/clip1.webm',headers={**self.auth(),'Range':'bytes=999999999-'})[0],416)
 def test_unknown_api(self):self.assertEqual(self.req('/api/nope',headers=self.auth())[0],404)
