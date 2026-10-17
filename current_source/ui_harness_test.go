// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

type uiNetwork struct{ *fakeNetwork }

func (n *uiNetwork) Request(ctx context.Context, r RequestSpec) ([]byte, Obj, error) {
	if strings.HasPrefix(r.URL, "https://cdn.example.com/") {
		delay := 900 * time.Millisecond
		if strings.Contains(r.URL, "/cancel/") {
			delay = 1800 * time.Millisecond
		}
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case <-time.After(delay):
		}
		if strings.Contains(r.URL, "bad") || strings.Contains(r.URL, "blocked") {
			return nil, nil, problem("测试服务器返回 HTTP 503", "http", 502)
		}
		return []byte(availabilityMP4()), Obj{"content_type": "video/mp4", "http": 206}, nil
	}
	if strings.HasPrefix(r.URL, "https://media.example.com/hls/") {
		file := strings.TrimPrefix(r.URL, "https://media.example.com/")
		b, e := demoAssets.ReadFile("testdata/" + file)
		ct := "video/mp2t"
		if strings.HasSuffix(file, ".m3u8") {
			ct = "application/vnd.apple.mpegurl"
		}
		return b, Obj{"content_type": ct}, e
	}
	return n.fakeNetwork.Fetch(ctx, r.URL, r.Limit)
}
func (n *uiNetwork) Fetch(ctx context.Context, u string, limit int) ([]byte, Obj, error) {
	return n.Request(ctx, RequestSpec{URL: u, Method: "GET", Limit: limit})
}
func (n *uiNetwork) JSON(ctx context.Context, base string, p Obj) (Obj, error) {
	query := sget(p, "wd")
	ids := sget(p, "ids")
	if query == "筛选测试" || query == "取消测试" || strings.HasPrefix(ids, "91") || strings.HasPrefix(ids, "92") {
		cancelFixture := query == "取消测试" || strings.HasPrefix(ids, "92")
		prefix, start, title := "filter", 911, "筛选测试"
		if cancelFixture {
			prefix, start, title = "cancel", 921, "取消测试"
		}
		rows := []Obj{
			{"vod_id": start, "vod_name": title + " 无法访问", "vod_play_from": "失效线路", "vod_play_url": "第01集$https://cdn.example.com/" + prefix + "/bad.mp4"},
			{"vod_id": start + 1, "vod_name": title + " 可访问", "vod_play_from": "正常线路", "vod_play_url": "第01集$https://cdn.example.com/" + prefix + "/good.mp4"},
			{"vod_id": start + 2, "vod_name": title + " 备用线路", "vod_play_from": "失效线路$$$正常备用", "vod_play_url": "第01集$https://cdn.example.com/" + prefix + "/blocked.mp4$$$第01集$https://media.example.com/hls/master.m3u8"},
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(180 * time.Millisecond):
		}
		list := []any{}
		for _, row := range rows {
			if strings.Contains(base, "suggest") {
				list = append(list, Obj{"id": row["vod_id"], "name": row["vod_name"]})
			} else if ids == "" || strings.Contains(","+ids+",", ","+str(row["vod_id"])+",") {
				list = append(list, row)
			}
		}
		return Obj{"list": list}, nil
	}
	if strings.HasPrefix(base, "https://api.github.com/repos/") {
		b, _ := assets.ReadFile("providers.json")
		var r SourceRegistry
		_ = json.Unmarshal(b, &r)
		for _, u := range r.Upstreams {
			if strings.Contains(base, u.Repository) {
				return Obj{"sha": u.Commit}, nil
			}
		}
	}
	return n.fakeNetwork.JSON(ctx, base, p)
}

// Explicitly enabled only in a test binary; fixtures are NOT in the shipped EXE.
func TestUIServerHarness(t *testing.T) {
	dir := os.Getenv("YINGXIA_UI_TEST_DIR")
	if dir == "" {
		t.Skip("manual browser harness not requested")
	}
	s, e := newService(dir, &uiNetwork{&fakeNetwork{}})
	mustNoErr(t, e)
	attachTestFixtures(s)
	s.links["test-hls"] = Film{ID: "test-hls", Title: "HLS 合成测试", Kind: "测试", Source: "测试片源", Groups: []Group{{Key: "g0", Name: "测试线路", Episodes: []Episode{{Key: "e0", Name: "第01集", URL: "https://media.example.com/hls/master.m3u8", Ordinal: intPtr(1)}, {Key: "e1", Name: "第02集", URL: "https://media.example.com/hls/master.m3u8", Ordinal: intPtr(2)}}}}}
	srv, e := newServer(s, 0)
	mustNoErr(t, e)
	srv.files = fixtureFiles{srv.files}
	fmt.Println("APP_URL=" + srv.origin)
	e = srv.HTTP.Serve(srv.listener)
	if e != http.ErrServerClosed {
		t.Fatal(e)
	}
}
