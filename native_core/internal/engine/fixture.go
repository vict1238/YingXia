// SPDX-License-Identifier: GPL-3.0-or-later
//go:build native_fixture

package engine

import (
	"bytes"
	"context"
	"embed"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

// Synthetic test media is compiled only into the separately named fixture
// executable. The shipping Windows backend and Android AAR use no fixture tag.
//
//go:embed fixturedata/demo/* fixturedata/hls/*
var nativeFixtureAssets embed.FS

type fixtureNetwork struct{}

func (n *fixtureNetwork) Fetch(ctx context.Context, link string, limit int) ([]byte, Obj, error) {
	return n.Request(ctx, RequestSpec{URL: link, Method: "GET", Limit: limit})
}
func (n *fixtureNetwork) Request(ctx context.Context, spec RequestSpec) ([]byte, Obj, error) {
	r, e := n.Open(ctx, spec)
	if e != nil {
		return nil, nil, e
	}
	defer r.Body.Close()
	b, e := io.ReadAll(r.Body)
	return b, Obj{"content_type": r.Header.Get("Content-Type"), "http": r.StatusCode}, e
}
func (n *fixtureNetwork) Open(ctx context.Context, spec RequestSpec) (*http.Response, error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	u, e := url.Parse(spec.URL)
	if e != nil || u.Hostname() != "media.example.com" {
		return nil, problem("fixture endpoint missing", "fixture", 502)
	}
	name := strings.TrimPrefix(path.Clean(u.Path), "/")
	b, e := nativeFixtureAssets.ReadFile("fixturedata/" + name)
	if e != nil {
		return nil, e
	}
	ct := "video/webm"
	if strings.HasSuffix(name, ".m3u8") {
		ct = "application/vnd.apple.mpegurl"
	}
	if strings.HasSuffix(name, ".ts") {
		ct = "video/mp2t"
	}
	h := http.Header{"Content-Type": {ct}, "Accept-Ranges": {"bytes"}}
	status := 200
	if raw := spec.Headers["Range"]; raw != "" {
		parts := strings.SplitN(strings.TrimPrefix(raw, "bytes="), "-", 2)
		if len(parts) != 2 {
			return nil, problem("fixture range invalid", "range", 416)
		}
		start, err := strconv.Atoi(parts[0])
		if err != nil || start < 0 || start >= len(b) {
			return nil, problem("fixture range invalid", "range", 416)
		}
		end := len(b) - 1
		if parts[1] != "" {
			end, err = strconv.Atoi(parts[1])
		}
		if err != nil || end < start {
			return nil, problem("fixture range invalid", "range", 416)
		}
		if end >= len(b) {
			end = len(b) - 1
		}
		h.Set("Content-Range", "bytes "+str(start)+"-"+str(end)+"/"+str(len(b)))
		b = b[start : end+1]
		status = 206
	}
	h.Set("Content-Length", str(len(b)))
	return &http.Response{StatusCode: status, Header: h, Body: io.NopCloser(bytes.NewReader(b)), ContentLength: int64(len(b)), Request: &http.Request{URL: u}}, nil
}
func (n *fixtureNetwork) JSON(_ context.Context, _ string, params Obj) (Obj, error) {
	items := []any{}
	for _, row := range fixtureRows() {
		id := sget(params, "ids")
		q := sget(params, "wd")
		if q == "native" || q == "test" || q == "fixture" {
			q = ""
		}
		if (id != "" && !strings.Contains(id, sget(row, "vod_id"))) || (id == "" && q != "" && !strings.Contains(sget(row, "vod_name"), q)) {
			continue
		}
		items = append(items, row)
	}
	return Obj{"list": items}, nil
}
func (n *fixtureNetwork) Probe(context.Context, string, string) Obj {
	return Obj{"state": "synthetic fixture", "playback_verified": false}
}

func fixtureRows() []Obj {
	return []Obj{
		{"vod_id": "90001", "vod_name": "原生播放测试 WebM", "vod_year": "2026", "type_name": "合成测试", "vod_play_from": "WebM 测试", "vod_play_url": "第01集$https://media.example.com/demo/clip1.webm#第02集$https://media.example.com/demo/clip2.webm"},
		{"vod_id": "90002", "vod_name": "原生播放测试 HLS", "vod_year": "2026", "type_name": "合成测试", "vod_play_from": "HLS 测试", "vod_play_url": "第01集$https://media.example.com/hls/master.m3u8#第02集$https://media.example.com/hls/master.m3u8"},
	}
}

func NewFixtureRuntime(dir string) (*Runtime, error) {
	_, previous := os.Stat(filepath.Join(dir, "library.json"))
	seedHistory := os.IsNotExist(previous)
	r, e := newRuntime(dir, &fixtureNetwork{})
	if e != nil {
		return nil, e
	}
	s := r.server.service
	enabled := map[string]any{}
	for _, p := range s.manager.providers() {
		enabled[p.ID] = p.ID == "wsyzy"
	}
	if e = s.store.SetMany(Obj{"network_consent": true, "auto_update_sources": false, "auto_next": false, "enabled_providers": enabled}); e != nil {
		r.Stop()
		return nil, e
	}
	if seedHistory {
		_, e = s.store.Opened(sourceFilmID("vod", "wsyzy", "90001"), "原生播放测试 WebM", "第01集", "合成测试", "media")
	}
	if e != nil {
		r.Stop()
		return nil, e
	}
	return r, nil
}
