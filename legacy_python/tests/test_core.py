import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch
from yingxia.models import *
from yingxia.security import *
from yingxia.catalog import Catalog,locate_platform
from yingxia.service import Service
from yingxia.storage import Store
from .fixtures import FakeNetwork,ROWS

class MatchingTests(unittest.TestCase):
 def test_english_spaces_preserved(self):self.assertEqual(normalize_title('Example Journey - 腾讯视频'),'Example Journey')
 def test_season_separated(self):self.assertEqual(normalize_title('示例剧 第二季 - 腾讯视频'),'示例剧 第二季')
 def test_season_attached(self):self.assertEqual(normalize_title('示例剧第二季 - 腾讯视频'),'示例剧第二季')
 def test_episode_removed_not_season(self):self.assertEqual(normalize_title('示例剧 第二季 第12集 - 腾讯视频'),'示例剧 第二季')
 def test_chinese_episode(self):self.assertEqual(episode_number('示例剧 第十二集'),12)
 def test_fullwidth_number(self):self.assertEqual(episode_number('第１２集'),12)
 def test_bilibili_id_never_ordinal(self):self.assertIsNone(episode_number('示例剧','https://www.bilibili.com/bangumi/play/ep15014'))
 def test_bilibili_title_wins(self):self.assertEqual(episode_number('第1集','https://www.bilibili.com/bangumi/play/ep15014'),1)
 def test_trailer_not_ordinal(self):self.assertIsNone(episode_number('第1集预告'))
 def test_chinese_number(self):self.assertEqual(number('一百零二'),102)
 def test_english_season(self):self.assertEqual(season('Example Journey Season 2'),2)
 def test_season_rank(self):self.assertGreater(ranking('示例剧 第二季','示例剧 第二季'),ranking('示例剧 第二季','示例剧 第一季'))
 def test_html_sanitized(self):self.assertEqual(text('<b>名称</b><script>bad()</script>'),'名称')
 def test_all_groups_kept(self):self.assertEqual(len(groups_from_vod(ROWS[0]['vod_play_url'],'a$$$b')),2)
 def test_group_names_kept(self):self.assertEqual([g.name for g in groups_from_vod(ROWS[0]['vod_play_url'],'a$$$b')],['a','b'])
 def test_http_only_groups(self):self.assertEqual(groups_from_vod('第一集$javascript:alert(1)'),[])
 def test_invalid_url_skipped(self):self.assertEqual(groups_from_vod('第一集$https://[abc'),[])
 def test_password_url_skipped(self):self.assertEqual(groups_from_vod('第一集$https://u:pw@example.com/a'),[])
 def test_missing_episode_not_first(self):
  g=groups_from_vod(ROWS[0]['vod_play_url'])[1]
  with self.assertRaises(AppError):choose_episode(g,'第02集')
 def test_equivalent_episode(self):
  g=groups_from_vod(ROWS[0]['vod_play_url'])[1];self.assertEqual(choose_episode(g,'第01集').name,'第1集')
 def test_part_label_not_dropped(self):
  g=groups_from_vod(ROWS[0]['vod_play_url'])[0]
  with self.assertRaises(AppError):choose_episode(g,'第01集上')
 def test_public_model_hides_media_url(self):
  f=film_from_vod(ROWS[0]);self.assertNotIn('url',f.public()['groups'][0]['episodes'][0])

class URLTests(unittest.TestCase):
 def test_public_https(self):self.assertEqual(public_url('https://example.com/a'),'https://example.com/a')
 def test_private_loopback(self):
  with self.assertRaises(AppError):public_url('http://127.0.0.1/')
 def test_private_rfc1918(self):
  with self.assertRaises(AppError):public_url('http://192.168.1.1/')
 def test_private_ipv6(self):
  with self.assertRaises(AppError):public_url('http://[::1]/')
 def test_credentials(self):
  with self.assertRaises(AppError):public_url('https://user:pass@example.com/')
 def test_file_url(self):
  with self.assertRaises(AppError):public_url('file:///c:/secret')
 def test_scheme_javascript(self):
  with self.assertRaises(AppError):public_url('javascript:alert(1)')
 def test_port(self):
  with self.assertRaises(AppError):public_url('https://example.com:8888/')
 def test_controls(self):
  with self.assertRaises(AppError):public_url('https://example.com/\r\nHost:x')
 def test_decimal_host(self):
  with self.assertRaises(AppError):public_url('http://2130706433/')
 def test_dns_private(self):
  with patch('socket.getaddrinfo',return_value=[(2,1,6,'',('10.0.0.2',443))]):
   with self.assertRaises(AppError):public_url('https://example.com/',resolve=True)
 def test_platform_strip_tracking_and_tokens(self):
  p,u=platform_url('https://v.qq.com/x/cover/a/b.html?token=secret&vid=abc&foo=bar#frag')
  self.assertEqual(p,'qq');self.assertEqual(u,'https://v.qq.com/x/cover/a/b.html?vid=abc')
 def test_platform_host_suffix_attack(self):
  with self.assertRaises(AppError):platform_url('https://v.qq.com.evil.com/x/cover/a.html')
 def test_wrong_platform(self):
  with self.assertRaises(AppError):platform_url('https://v.qq.com/x/cover/a.html','iqiyi')
 def test_search_page_not_playback(self):
  with self.assertRaises(AppError):platform_url('https://v.qq.com/x/search/?q=a')
 def test_search_query_encoding(self):self.assertIn('%26',official_search('qq','a & b'))

class ServiceTests(unittest.TestCase):
 def setUp(self):
  self.tmp=tempfile.TemporaryDirectory();self.net=FakeNetwork();self.s=Service(Path(self.tmp.name),self.net)
 def tearDown(self):self.s.store.close();self.tmp.cleanup()
 def allow(self):self.s.settings({'network_consent':True})
 def test_network_off_default(self):self.assertFalse(self.s.state()['network_consent'])
 def test_no_network_without_consent(self):
  with self.assertRaises(AppError):self.s.search({'query':'x'})
  self.assertEqual(self.net.calls,[])
 def test_demo_no_network(self):
  r=self.s.search({'demo':True});self.assertTrue(r['demo']);self.assertEqual(len(r['items']),2);self.assertEqual(self.net.calls,[])
 def test_catalog_search_and_detail(self):
  self.allow();r=self.s.search({'query':'Example Journey 第二季'});self.assertEqual(r['items'][0]['id'],'101');self.assertEqual(len(self.s.detail('101').groups),2)
 def test_fallback(self):
  self.allow();self.net.fail_suggest=True;r=self.s.search({'query':'Example'});self.assertEqual(len(r['items']),2)
 def test_summary_when_details_fail(self):
  self.allow();self.net.fail_details=True;r=self.s.search({'query':'Example'});self.assertEqual(len(r['items']),2);self.assertFalse(r['items'][0]['groups'])
 def test_not_found_detail(self):
  self.allow()
  with self.assertRaises(AppError):self.s.detail('999')
 def test_demo_resolve_and_history(self):
  p=self.s.resolve({'film_id':'demo-orbit-1','group_key':'g0','episode_key':'e0'});self.assertEqual(p['kind'],'media');self.assertFalse(p['playback_verified']);self.assertEqual(self.s.store.history()[0]['evidence'],'opened')
 def test_remote_catalog_resolve(self):
  self.allow();p=self.s.resolve({'film_id':'101','group_key':'g0','episode_key':'e0'});self.assertEqual(p['kind'],'page');self.assertIn('wsyzy.vip',p['url']);self.assertIn('%3A%2F%2F',p['url'])
 def test_missing_binding_is_error(self):
  self.allow()
  with self.assertRaises(AppError):self.s.resolve({'film_id':'101','group_key':'g0','episode_key':'e0','source':'qilin'})
 def test_hls_disabled(self):
  self.assertFalse(next(x for x in self.s.enabled_sources() if x['id']=='hls')['enabled'])
 def test_disabled_parser_rejected(self):
  self.allow()
  with self.assertRaises(AppError):self.s.resolve({'film_id':'101','group_key':'g0','episode_key':'e0','source':'hls','platform_url':'https://v.qq.com/x/cover/a/b.html'})
 def test_platform_parser_encoded(self):
  self.allow();p=self.s.resolve({'film_id':'101','group_key':'g0','episode_key':'e0','source':'qilin','platform_url':'https://v.qq.com/x/cover/a/b.html?token=secret'})
  self.assertNotIn('secret',p['url']);self.assertIn('free.maccms.xyz',p['url'])
 def test_import_is_offline(self):
  f=self.s.import_platform({'url':'https://v.qq.com/x/cover/a/b.html','title':'测试'});self.assertTrue(f['id'].startswith('link-'));self.assertEqual(self.net.calls,[])
 def test_platform_locator(self):
  self.allow();r=self.s.search({'query':'Example Journey','mode':'qq'});self.assertEqual(len(r['items']),1);self.assertTrue(r['items'][0]['id'].startswith('link-'))
 def test_episode_match_missing(self):
  with self.assertRaises(AppError):self.s.match({'film_id':'demo-orbit-1','group_key':'g1','match_name':'第02集'})
 def test_favorite_roundtrip(self):
  f=self.s.detail('demo-orbit-1').public();self.assertTrue(self.s.store.toggle_favorite(f));self.assertEqual(len(self.s.store.favorites()),1);self.assertFalse(self.s.store.toggle_favorite(f))
 def test_progress_evidence(self):
  p=self.s.resolve({'film_id':'demo-orbit-1','group_key':'g0','episode_key':'e0'});self.s.store.progress(p['history_id'],'browser_time_advanced',3.2);self.assertEqual(self.s.store.history()[0]['position'],3.2)
 def test_nan_progress_rejected(self):
  with self.assertRaises(AppError):self.s.store.progress('x','browser_time_advanced',float('nan'))
 def test_history_opt_out(self):
  self.s.settings({'save_history':False});self.s.resolve({'film_id':'demo-orbit-1','group_key':'g0','episode_key':'e0'});self.assertEqual(self.s.store.history(),[])
 def test_settings_type(self):
  with self.assertRaises(AppError):self.s.settings({'network_consent':'yes'})
 def test_diagnostics_not_playback(self):
  self.allow();r=self.s.diagnose();self.assertFalse(r['playback_verified']);self.assertTrue(all(not x['playback_verified'] for x in r['results']))
 def test_export_no_search_or_media(self):
  self.allow();self.s.search({'query':'Example'});r=str(self.s.export_report());self.assertNotIn('a1.mp4',r);self.assertNotIn('Example Journey',r)
