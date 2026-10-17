// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"html"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/antchfx/htmlquery"
)

type mediaGrant struct {
	URL     string
	Headers map[string]string
	Expires time.Time
}
type MediaRelay struct {
	mu     sync.Mutex
	grants map[string]mediaGrant
	keys   map[string]string
	net    Fetcher
}

func newMediaRelay(n Fetcher) *MediaRelay {
	return &MediaRelay{grants: map[string]mediaGrant{}, keys: map[string]string{}, net: n}
}
func (m *MediaRelay) register(link string, headers map[string]string) (string, error) {
	clean, e := publicURL(link)
	if e != nil {
		return "", e
	}
	h := map[string]string{}
	for k, v := range headers {
		if (strings.EqualFold(k, "Referer") || strings.EqualFold(k, "User-Agent")) && !hasControl(v) && len(v) < 4096 {
			h[k] = v
		}
	}
	b, _ := json.Marshal(Obj{"url": clean, "headers": h})
	key := string(b)
	m.mu.Lock()
	defer m.mu.Unlock()
	if id, ok := m.keys[key]; ok {
		if g, ok := m.grants[id]; ok {
			g.Expires = time.Now().Add(2 * time.Hour)
			m.grants[id] = g
			return "/api/media?ticket=" + id, nil
		}
	}
	if len(m.grants) >= 8192 {
		for id, g := range m.grants {
			if time.Now().After(g.Expires) {
				delete(m.grants, id)
			}
		}
		if len(m.grants) >= 8192 {
			m.grants = map[string]mediaGrant{}
			m.keys = map[string]string{}
		}
	}
	id := token()
	m.grants[id] = mediaGrant{clean, h, time.Now().Add(2 * time.Hour)}
	m.keys[key] = id
	return "/api/media?ticket=" + id, nil
}
func (m *MediaRelay) grant(id string) (mediaGrant, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	g, ok := m.grants[id]
	if !ok || time.Now().After(g.Expires) {
		return g, problem("播放链接已过期，请重新播放。", "media_expired", 404)
	}
	g.Expires = time.Now().Add(2 * time.Hour)
	m.grants[id] = g
	return g, nil
}

var hlsURI = re(`URI="([^"]+)"`)

func (m *MediaRelay) rewritePlaylist(body []byte, base string, headers map[string]string) ([]byte, error) {
	if len(body) > 4000000 || !strings.HasPrefix(strings.TrimSpace(string(body)), "#EXTM3U") {
		return nil, problem("HLS 播放列表无效。", "hls", 502)
	}
	lines := strings.Split(strings.ReplaceAll(string(body), "\r\n", "\n"), "\n")
	for i, line := range lines {
		s := strings.TrimSpace(line)
		if s == "" {
			continue
		}
		if strings.HasPrefix(s, "#") {
			var failed error
			lines[i] = hlsURI.ReplaceAllStringFunc(line, func(match string) string {
				parts := hlsURI.FindStringSubmatch(match)
				u, e := ruleURL(base, parts[1])
				if e != nil {
					failed = e
					return match
				}
				ticket, e := m.register(u, headers)
				if e != nil {
					failed = e
					return match
				}
				return `URI="` + ticket + `"`
			})
			if failed != nil {
				return nil, failed
			}
		} else {
			u, e := ruleURL(base, s)
			if e != nil {
				return nil, e
			}
			ticket, e := m.register(u, headers)
			if e != nil {
				return nil, e
			}
			lines[i] = ticket
		}
	}
	return []byte(strings.Join(lines, "\n")), nil
}

type mediaOpener interface {
	Open(context.Context, RequestSpec) (*http.Response, error)
}

func (m *MediaRelay) serve(w http.ResponseWriter, r *http.Request) {
	g, e := m.grant(r.URL.Query().Get("ticket"))
	if e != nil {
		sendError(w, e)
		return
	}
	headers := map[string]string{}
	for k, v := range g.Headers {
		headers[k] = v
	}
	if v := r.Header.Get("Range"); v != "" {
		if !re(`^bytes=\d*-\d*$`).MatchString(v) {
			sendError(w, problem("分段请求无效。", "range", 416))
			return
		}
		headers["Range"] = v
	}
	ctx, cancel := context.WithTimeout(r.Context(), 85*time.Second)
	defer cancel()
	spec := RequestSpec{URL: g.URL, Method: "GET", Headers: headers, Limit: 32000000}
	var res *http.Response
	if opener, ok := m.net.(mediaOpener); ok {
		res, e = opener.Open(ctx, spec)
	} else {
		var b []byte
		var meta Obj
		b, meta, e = fetchRequest(ctx, m.net, spec)
		if e == nil {
			res = &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(b)), Request: &http.Request{URL: &url.URL{}}}
			res.Header.Set("Content-Type", sget(meta, "content_type"))
			u, _ := url.Parse(g.URL)
			res.Request.URL = u
		}
	}
	if e != nil {
		sendError(w, e)
		return
	}
	defer res.Body.Close()
	reader := bufio.NewReader(res.Body)
	prefix, _ := reader.Peek(16)
	ct := strings.ToLower(res.Header.Get("Content-Type"))
	u, _ := url.Parse(g.URL)
	if strings.Contains(ct, "mpegurl") || strings.EqualFold(path.Ext(u.Path), ".m3u8") || strings.HasPrefix(strings.TrimSpace(string(prefix)), "#EXTM3U") {
		body, e := io.ReadAll(io.LimitReader(reader, 4000001))
		if e != nil {
			sendError(w, e)
			return
		}
		base := g.URL
		if res.Request != nil && res.Request.URL != nil && res.Request.URL.Host != "" {
			base = res.Request.URL.String()
		}
		body, e = m.rewritePlaylist(body, base, g.Headers)
		if e != nil {
			sendError(w, e)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.WriteHeader(200)
		if r.Method != "HEAD" {
			_, _ = w.Write(body)
		}
		return
	}
	if strings.Contains(ct, "text/html") || strings.HasPrefix(strings.ToLower(strings.TrimSpace(string(prefix))), "<!doctype") {
		sendError(w, problem("来源返回网页，无法作为媒体播放。", "not_media", 502))
		return
	}
	if !strings.HasPrefix(ct, "video/") && !strings.HasPrefix(ct, "audio/") {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	for _, key := range []string{"Content-Length", "Content-Range", "Accept-Ranges"} {
		if v := res.Header.Get(key); v != "" {
			w.Header().Set(key, v)
		}
	}
	w.WriteHeader(res.StatusCode)
	if r.Method != "HEAD" {
		_, _ = io.Copy(w, io.LimitReader(reader, 512*1024*1024))
	}
}
func isMediaURL(link string) bool {
	u, e := url.Parse(link)
	if e != nil {
		return false
	}
	switch strings.ToLower(path.Ext(u.Path)) {
	case ".m3u8", ".mp4", ".webm", ".m4v", ".mov":
		return true
	}
	return false
}

var playerObject = re(`(?s)(?:var\s+|let\s+|const\s+)?player_[a-zA-Z0-9_]+\s*=\s*(\{[^\n]*?\})\s*;`)
var rawMediaURL = re(`https?:[^\s<>"']+?\.(?:m3u8|mp4|webm)(?:\?[^\s<>"']*)?`)

func extractPageMedia(raw []byte, base string) (string, []string) {
	for _, match := range playerObject.FindAllSubmatch(raw, 8) {
		var o Obj
		if json.Unmarshal(match[1], &o) == nil {
			link := sget(o, "url")
			switch sget(o, "encrypt") {
			case "1":
				if decoded, e := url.QueryUnescape(link); e == nil {
					link = decoded
				}
			case "2":
				if b, e := base64.StdEncoding.DecodeString(link); e == nil {
					link = string(b)
					if decoded, e := url.QueryUnescape(link); e == nil {
						link = decoded
					}
				}
			}
			if isMediaURL(link) {
				if u, e := ruleURL(base, link); e == nil {
					return u, nil
				}
			}
		}
	}
	doc, e := htmlquery.Parse(strings.NewReader(string(raw)))
	if e == nil {
		nodes, _ := htmlquery.QueryAll(doc, "//video[@src] | //video/source[@src] | //source[@src]")
		for _, node := range nodes {
			if u, e := ruleURL(base, htmlquery.SelectAttr(node, "src")); e == nil && isMediaURL(u) {
				return u, nil
			}
		}
	}
	text := strings.ReplaceAll(string(raw), `\/`, `/`)
	matches := rawMediaURL.FindAllString(text, 8)
	if len(matches) == 1 {
		if u, e := publicURL(html.UnescapeString(matches[0])); e == nil {
			return u, nil
		}
	}
	frames := []string{}
	if doc != nil {
		nodes, _ := htmlquery.QueryAll(doc, "//iframe[@src]")
		for i, node := range nodes {
			if i >= 3 {
				break
			}
			if u, e := ruleURL(base, htmlquery.SelectAttr(node, "src")); e == nil {
				frames = append(frames, u)
			}
		}
	}
	return "", frames
}
func resolveRuleMedia(ctx context.Context, n Fetcher, link string, rule KazumiRule) (string, error) {
	if isMediaURL(link) {
		return link, nil
	}
	headers := ruleHeaders(rule)
	b, _, e := fetchRequest(ctx, n, RequestSpec{URL: link, Method: "GET", Headers: headers, Limit: 4000000})
	if e != nil {
		return "", e
	}
	media, frames := extractPageMedia(b, link)
	if media != "" {
		return media, nil
	}
	for _, frame := range frames {
		b, _, e := fetchRequest(ctx, n, RequestSpec{URL: frame, Method: "GET", Headers: headers, Limit: 2000000})
		if e == nil {
			if media, _ := extractPageMedia(b, frame); media != "" {
				return media, nil
			}
		}
	}
	return "", problem("此来源需要网页播放器。", "page_player")
}
