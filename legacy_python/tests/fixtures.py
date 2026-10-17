"""Synthetic responses for deterministic tests; NOT real source responses."""
from yingxia.models import AppError
ROWS=[
 {'vod_id':101,'vod_name':'Example Journey 第二季','vod_year':'2024','type_name':'合成测试剧',
  'vod_content':'<p>测试简介</p><script>not executed</script>', 'vod_play_from':'alpha$$$beta',
  'vod_play_url':'第01集$https://cdn.example.com/a1.mp4#第02集$https://cdn.example.com/a2.mp4$$$第1集$https://cdn.example.com/b1.mp4'},
 {'vod_id':102,'vod_name':'Example Journey 第一季','vod_year':'2022','type_name':'合成测试剧',
  'vod_play_from':'alpha','vod_play_url':'第1集$https://cdn.example.com/s1.mp4'}]
class FakeNetwork:
 def __init__(self):self.calls=[];self.fail_suggest=False;self.fail_details=False
 def json(self,base,params=None):
  self.calls.append((base,params))
  if 'suggest' in base:
   if self.fail_suggest:raise AppError('fixture suggest failure','network',502)
   return {'list':[{'id':r['vod_id'],'name':r['vod_name']} for r in ROWS]}
  if self.fail_details:raise AppError('fixture detail failure','network',502)
  ids=str((params or {}).get('ids','')).split(',')
  return {'list':[r.copy() for r in ROWS if not ids[0] or str(r['vod_id']) in ids]}
 def fetch(self,url,limit=4_000_000):
  self.calls.append((url,None))
  return b'<html><a href="https://v.qq.com/x/cover/test/demo.html">Example Journey</a></html>', {'http':200,'content_type':'text/html'}
 def probe(self,name,url):
  return {'name':name,'endpoint':url,'http':200,'state':'fixture HTTP only','playback_verified':False}
