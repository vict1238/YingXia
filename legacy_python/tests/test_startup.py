import http.client
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from urllib.parse import urlsplit

class StartupTests(unittest.TestCase):
 def test_cli_start_session_and_clean_shutdown(self):
  root=Path(__file__).resolve().parents[1]
  with tempfile.TemporaryDirectory() as tmp:
   proc=subprocess.Popen([sys.executable,str(root/'main.py'),'--no-browser','--data-dir',tmp],
       stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True,encoding='utf-8',
       env={**os.environ,'PYTHONIOENCODING':'utf-8'})
   try:
    lines=[proc.stdout.readline(),proc.stdout.readline()]
    url=next(line.strip()[8:] for line in lines if line.startswith('APP_URL='))
    p=urlsplit(url)
    def req(path,method='GET',body=None,headers=None):
     c=http.client.HTTPConnection(p.hostname,p.port,timeout=5)
     c.request(method,path,body=body,headers=headers or {});r=c.getresponse();result=r.status,dict(r.getheaders()),r.read();c.close();return result
    status,h,_=req('/');self.assertEqual(status,200)
    cookie=h['Set-Cookie'].split(';')[0]
    status,h,body=req('/bootstrap.js',headers={'Cookie':cookie})
    boot=json.loads(body.decode().split('=',1)[1].rstrip(';'))
    status,_,_=req('/api/quit','POST',b'{}',{'Cookie':cookie,'Origin':url,'Content-Type':'application/json','X-YingXia-CSRF':boot['csrf']})
    self.assertEqual(status,200);proc.communicate(timeout=5);self.assertEqual(proc.returncode,0)
   finally:
    if proc.poll() is None:proc.kill();proc.communicate()
