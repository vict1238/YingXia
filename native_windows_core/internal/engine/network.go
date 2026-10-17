// SPDX-License-Identifier: GPL-3.0-or-later
package engine

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type Fetcher interface {
	Fetch(context.Context, string, int) ([]byte, Obj, error)
	JSON(context.Context, string, Obj) (Obj, error)
	Probe(context.Context, string, string) Obj
}
type Network struct {
	client  *http.Client
	proxies sync.Map
}

func newNetwork() *Network {
	n := &Network{}
	tr := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}, MaxIdleConns: 10, IdleConnTimeout: 30 * time.Second, TLSHandshakeTimeout: 8 * time.Second, ResponseHeaderTimeout: 9 * time.Second, DisableCompression: true}
	tr.Proxy = func(r *http.Request) (*url.URL, error) {
		p, e := proxyForRequest(r)
		if p != nil {
			port := p.Port()
			if port == "" {
				if p.Scheme == "https" {
					port = "443"
				} else {
					port = "80"
				}
			}
			n.proxies.Store(net.JoinHostPort(p.Hostname(), port), true)
		}
		return p, e
	}
	tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		if _, ok := n.proxies.Load(addr); ok {
			return (&net.Dialer{Timeout: 8 * time.Second}).DialContext(ctx, network, addr)
		}
		host, port, e := net.SplitHostPort(addr)
		if e != nil {
			return nil, e
		}
		ips, e := lookupPublic(ctx, host)
		if e != nil {
			return nil, e
		}
		var last error
		for _, ip := range ips {
			c, e := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ip, port))
			if e == nil {
				return c, nil
			}
			last = e
		}
		return nil, last
	}
	n.client = &http.Client{Transport: tr, Timeout: 12 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 4 {
			return problem("来源重定向次数过多。", "redirect", 502)
		}
		if len(via) > 0 && !strings.EqualFold(req.URL.Hostname(), via[0].URL.Hostname()) {
			req.Header.Del("Authorization")
			req.Header.Del("Apikey")
			req.Header.Del("X-Api-Key")
		}
		_, e := publicURL(req.URL.String())
		if e != nil {
			return e
		}
		_, e = lookupPublic(req.Context(), req.URL.Hostname())
		return e
	}}
	return n
}
func lookupPublic(ctx context.Context, host string) ([]string, error) {
	ips, e := net.DefaultResolver.LookupIPAddr(ctx, host)
	if e != nil {
		return nil, problem("DNS 解析失败，请检查网络或代理。", "dns", 502)
	}
	if len(ips) == 0 {
		return nil, problem("DNS 未返回地址。", "dns", 502)
	}
	out := []string{}
	for _, i := range ips {
		if !isPublicIP(i.IP.String()) {
			return nil, problem("目标解析到非公网地址，已阻止请求。", "unsafe_url")
		}
		out = append(out, i.IP.String())
	}
	return out, nil
}
func (n *Network) Fetch(ctx context.Context, link string, limit int) ([]byte, Obj, error) {
	return n.Request(ctx, RequestSpec{URL: link, Method: "GET", Limit: limit})
}
func (n *Network) Request(ctx context.Context, spec RequestSpec) ([]byte, Obj, error) {
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	start := time.Now()
	res, e := n.Open(ctx, spec)
	if e != nil {
		return nil, nil, e
	}
	defer res.Body.Close()
	limit := spec.Limit
	if limit <= 0 || limit > 32000000 {
		limit = 4000000
	}
	raw, e := io.ReadAll(io.LimitReader(res.Body, int64(limit+1)))
	if e != nil {
		return nil, nil, problem("读取响应未完成。", "network", 502)
	}
	if len(raw) > limit {
		return nil, nil, problem("来源响应过大。", "too_large", 502)
	}
	return raw, Obj{"http": res.StatusCode, "content_type": res.Header.Get("Content-Type"), "endpoint": redactedURL(res.Request.URL.String()), "elapsed_ms": time.Since(start).Milliseconds()}, nil
}
func (n *Network) Open(ctx context.Context, spec RequestSpec) (*http.Response, error) {
	link := spec.URL
	link, e := publicURL(link)
	if e != nil {
		return nil, e
	}
	p, _ := url.Parse(link)
	if _, e = lookupPublic(ctx, p.Hostname()); e != nil {
		return nil, e
	}
	method := spec.Method
	if method == "" {
		method = http.MethodGet
	}
	if method != "GET" && method != "POST" {
		return nil, problem("请求方法不支持。", "method")
	}
	req, e := http.NewRequestWithContext(ctx, method, link, strings.NewReader(string(spec.Body)))
	if e != nil {
		return nil, problem("网址无效。", "unsafe_url")
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) YingXia/"+version)
	req.Header.Set("Accept", "application/json,text/html;q=0.8,*/*;q=0.1")
	req.Header.Set("Accept-Encoding", "identity")
	for k, v := range spec.Headers {
		if (allowedRuleHeader(k) || strings.EqualFold(k, "Range")) && !hasControl(v) && len(v) < 4096 {
			req.Header.Set(k, v)
		}
	}
	client := *n.client
	client.Timeout = 0 // Request bounds metadata to 12 s; media uses its streaming context.
	res, e := client.Do(req)
	if e != nil {
		var ae *AppError
		if errors.As(e, &ae) {
			return nil, ae
		}
		var ne net.Error
		if errors.As(e, &ne) && ne.Timeout() {
			return nil, problem("连接超时，请检查网络或换来源。", "timeout", 504)
		}
		if strings.Contains(strings.ToLower(e.Error()), "certificate") {
			return nil, problem("TLS 证书校验失败。", "certificate", 502)
		}
		return nil, problem("网络请求未完成，请检查本机网络、代理或来源状态。", "network", 502)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		res.Body.Close()
		return nil, problem("来源返回 HTTP "+str(res.StatusCode), "http", 502)
	}
	return res, nil
}
func (n *Network) JSON(ctx context.Context, base string, params Obj) (Obj, error) {
	p, e := url.Parse(base)
	if e != nil {
		return nil, problem("来源配置错误。", "config")
	}
	q := p.Query()
	for k, v := range params {
		q.Set(k, str(v))
	}
	p.RawQuery = q.Encode()
	b, _, e := n.Fetch(ctx, p.String(), 4000000)
	if e != nil {
		return nil, e
	}
	return decodeObject(b)
}
func decodeObject(b []byte) (Obj, error) {
	var o Obj
	d := json.NewDecoder(strings.NewReader(strings.TrimPrefix(string(b), "\ufeff")))
	d.UseNumber()
	if e := d.Decode(&o); e != nil || o == nil {
		return nil, problem("接口未返回有效 JSON 对象，可能是验证页或接口已变化。", "bad_json", 502)
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return nil, problem("接口含多余 JSON 内容。", "bad_json", 502)
	}
	return o, nil
}
func (n *Network) Probe(ctx context.Context, name, link string) Obj {
	r := Obj{"name": name, "endpoint": redactedURL(link), "playback_verified": false, "state": "未完成"}
	b, m, e := n.Fetch(ctx, link, 2000000)
	if e != nil {
		r["state"] = e.Error()
		var ae *AppError
		if errors.As(e, &ae) {
			r["error"] = ae.Code
		}
		return r
	}
	for k, v := range m {
		r[k] = v
	}
	r["state"] = "HTTP 连接正常"
	if _, e := decodeObject(b); e == nil {
		r["state"] = "JSON 响应正常"
	}
	s := string(b)
	if strings.Contains(s, "江湖路远，后会无期") || strings.Contains(s, "此去一别，不再相逢") {
		r["state"] = "入口含告别文案；疑似异常"
	}
	return r
}
