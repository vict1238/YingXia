// SPDX-License-Identifier: GPL-3.0-or-later
package engine

import (
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
)

type Platform struct {
	Name   string
	Hosts  []string
	Search string
	Path   string
}

var platforms = map[string]Platform{
	"qq":       {"腾讯视频", []string{"v.qq.com", "m.v.qq.com"}, "https://v.qq.com/x/search/?q={query}", `^/(?:x/(?:cover|page|m)/|tv/)`},
	"iqiyi":    {"爱奇艺", []string{"www.iqiyi.com", "m.iqiyi.com", "www.iq.com"}, "https://so.iqiyi.com/so/q_{query}", `^/(?:v_|w_|play/)`},
	"youku":    {"优酷", []string{"v.youku.com", "m.youku.com"}, "https://so.youku.com/search_video/q_{query}", `^/(?:v_show/|video|v_)`},
	"bilibili": {"哔哩哔哩", []string{"www.bilibili.com", "m.bilibili.com"}, "https://search.bilibili.com/all?keyword={query}", `^/(?:video/|bangumi/play/)`},
	"mgtv":     {"芒果TV", []string{"www.mgtv.com", "m.mgtv.com", "w.mgtv.com"}, "https://so.mgtv.com/so?k={query}", `^/b/`},
}
var reserved = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"), netip.MustParsePrefix("127.0.0.0/8"), netip.MustParsePrefix("169.254.0.0/16"), netip.MustParsePrefix("172.16.0.0/12"), netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"), netip.MustParsePrefix("192.168.0.0/16"), netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("198.51.100.0/24"), netip.MustParsePrefix("203.0.113.0/24"), netip.MustParsePrefix("224.0.0.0/4"), netip.MustParsePrefix("240.0.0.0/4"), netip.MustParsePrefix("::/96"), netip.MustParsePrefix("64:ff9b:1::/48"), netip.MustParsePrefix("100::/64"), netip.MustParsePrefix("2001::/23"), netip.MustParsePrefix("2001:db8::/32"), netip.MustParsePrefix("2002::/16"), netip.MustParsePrefix("fc00::/7"), netip.MustParsePrefix("fe80::/10"), netip.MustParsePrefix("ff00::/8"),
}

func isPublicIP(s string) bool {
	a, e := netip.ParseAddr(s)
	if e != nil {
		return false
	}
	a = a.Unmap()
	if !a.IsGlobalUnicast() || a.IsPrivate() || a.IsLoopback() || a.IsLinkLocalUnicast() {
		return false
	}
	for _, p := range reserved {
		if p.Contains(a) {
			return false
		}
	}
	return true
}
func publicURL(v string) (string, error) {
	if len(v) > 4096 || hasControl(v) || strings.Contains(v, "\\") {
		return "", problem("网址为空、过长或含非法字符。", "unsafe_url")
	}
	p, e := url.Parse(strings.TrimSpace(v))
	if e != nil || p.Opaque != "" || p.User != nil || p.Hostname() == "" || (p.Scheme != "https" && p.Scheme != "http") {
		return "", problem("仅支持不含账号密码的 HTTP / HTTPS 网址。", "unsafe_url")
	}
	h := strings.TrimSuffix(strings.ToLower(p.Hostname()), ".")
	port := p.Port()
	if port != "" && port != "80" && port != "443" {
		return "", problem("不允许非标准网络端口。", "unsafe_url")
	}
	if h == "localhost" || strings.HasSuffix(h, ".localhost") || strings.HasSuffix(h, ".local") || strings.HasSuffix(h, ".internal") {
		return "", problem("不允许本机或内网地址。", "unsafe_url")
	}
	if ip := net.ParseIP(h); ip != nil {
		if !isPublicIP(h) {
			return "", problem("不允许本机或内网地址。", "unsafe_url")
		}
	} else if !strings.Contains(h, ".") || re(`^[\d.]+$`).MatchString(h) || strings.HasPrefix(h, "0x") {
		return "", problem("不允许特殊数值 IP 或内网地址。", "unsafe_url")
	}
	for _, r := range h {
		if r > 127 {
			return "", problem("当前来源域名须使用标准 ASCII / Punycode 格式。", "unsafe_url")
		}
	}
	p.Fragment = ""
	if p.Path == "" {
		p.Path = "/"
	}
	return p.String(), nil
}
func platformURL(v, expected string) (string, string, error) {
	s, e := publicURL(v)
	if e != nil {
		return "", "", e
	}
	p, _ := url.Parse(s)
	if p.Scheme != "https" {
		return "", "", problem("平台播放页请使用 HTTPS 地址。", "bad_platform")
	}
	found := ""
	for k, pf := range platforms {
		for _, h := range pf.Hosts {
			if strings.EqualFold(h, p.Hostname()) {
				found = k
				break
			}
		}
	}
	if found == "" || (expected != "" && expected != found) {
		return "", "", problem("网址与所选平台不符。", "bad_platform")
	}
	if !re(platforms[found].Path).MatchString(p.Path) {
		return "", "", problem("请粘贴具体播放页，不是搜索页或首页。", "bad_platform")
	}
	keep := map[string]bool{"vid": true, "cid": true, "id": true, "tvid": true, "albumid": true, "p": true, "ep": true, "episode": true, "cur": true}
	q := url.Values{}
	for k, vs := range p.Query() {
		if keep[strings.ToLower(k)] {
			for _, val := range vs {
				if len(val) < 200 {
					q.Add(k, val)
				}
			}
		}
	}
	p.RawQuery = q.Encode()
	p.Fragment = ""
	return found, p.String(), nil
}
func quote(s string) string { return strings.ReplaceAll(url.QueryEscape(s), "+", "%20") }
func hostName(s string) string {
	u, e := url.Parse(s)
	if e != nil {
		return ""
	}
	return u.Hostname()
}
func officialSearch(platform, query string) (string, error) {
	p, ok := platforms[platform]
	if !ok {
		return "", problem("请选择支持的平台。", "bad_platform")
	}
	return strings.ReplaceAll(p.Search, "{query}", quote(query)), nil
}
func redactedURL(s string) string {
	p, e := url.Parse(s)
	if e != nil {
		return "[invalid url]"
	}
	p.RawQuery = ""
	p.Fragment = ""
	p.User = nil
	return p.String()
}
func boolSetting(settings Obj, key string, def bool) bool {
	v, ok := settings[key].(bool)
	if !ok {
		return def
	}
	return v
}
func numValue(v any) (float64, bool) { n, e := strconv.ParseFloat(str(v), 64); return n, e == nil }
