import unittest
from source_check import validate_platform_url, parse_source_groups, SOURCES

class PreflightUnitTests(unittest.TestCase):
    def test_exact_source_count(self): self.assertEqual(len(SOURCES), 7)
    def test_https_official(self): self.assertEqual(validate_platform_url('https://www.bilibili.com/bangumi/play/ep15014#x'), 'https://www.bilibili.com/bangumi/play/ep15014')
    def test_no_userinfo(self):
        with self.assertRaises(ValueError): validate_platform_url('https://x:y@v.qq.com/x/page/a')
    def test_no_fake_domain(self):
        with self.assertRaises(ValueError): validate_platform_url('https://v.qq.com.example.invalid/x/page/a')
    def test_no_plain_http(self):
        with self.assertRaises(ValueError): validate_platform_url('http://v.qq.com/x/page/a')
    def test_no_nonstandard_port(self):
        with self.assertRaises(ValueError): validate_platform_url('https://v.qq.com:8000/x/page/a')
    def test_preserves_groups(self):
        result=parse_source_groups('第1集$https://one.example.invalid/1.m3u8$$$第1集$https://two.example.invalid/1.m3u8')
        self.assertEqual(len(result),2)
    def test_rejects_script(self): self.assertEqual(parse_source_groups('x$javascript:alert(1)'),[])
    def test_empty_sources(self): self.assertEqual(parse_source_groups(''),[])
    def test_unnamed_media(self): self.assertEqual(parse_source_groups('https://a.example.invalid/x')[0]['episodes'][0]['name'],'')

if __name__=='__main__': unittest.main(verbosity=2)
