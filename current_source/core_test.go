// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

var ctx = context.Background()

// Test clips are only embedded in test binaries, never the release EXE.
//
//go:embed testdata/demo/*.webm testdata/hls/*
var demoAssets embed.FS

type fixtureFiles struct{ fs.FS }

func (f fixtureFiles) Open(name string) (fs.File, error) {
	if strings.HasPrefix(name, "demo/") {
		return demoAssets.Open("testdata/" + name)
	}
	return f.FS.Open(name)
}

func attachTestFixtures(s *Service) {
	for _, f := range demoFilms() {
		s.links[f.ID] = f
	}
}

func demoFilms() []Film {
	return []Film{
		{ID: "demo-orbit-1", Title: "轨道片段 第一季", Year: "测试", Kind: "合成测试", Source: "测试片源", Demo: true, Groups: []Group{{"g0", "本机 WebM", []Episode{{"e0", "第01集", "/demo/clip1.webm", intPtr(1)}, {"e1", "第02集", "/demo/clip2.webm", intPtr(2)}}}, {"g1", "备用测试组", []Episode{{"e0", "第1集", "/demo/clip1.webm", intPtr(1)}}}}},
		{ID: "demo-orbit-2", Title: "轨道片段 第二季", Year: "测试", Kind: "合成测试", Source: "测试片源", Demo: true, Groups: []Group{{"g0", "本机 WebM", []Episode{{"e0", "第一集", "/demo/clip2.webm", intPtr(1)}}}}},
	}
}

func require(t *testing.T, b bool, message string) {
	t.Helper()
	if !b {
		t.Fatal(message)
	}
}
func mustNoErr(t *testing.T, e error) {
	t.Helper()
	if e != nil {
		t.Fatal(e)
	}
}
func errCode(e error) string {
	var a *AppError
	if errors.As(e, &a) {
		return a.Code
	}
	return ""
}
func TestMetadata(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{"english_spaces", "Example Journey - 腾讯视频", "Example Journey"},
		{"season_spaced", "示例剧 第二季 - 腾讯视频", "示例剧 第二季"},
		{"season_joined", "示例剧第二季 - 腾讯视频", "示例剧第二季"},
		{"episode_suffix", "示例剧 第12集 - 腾讯视频", "示例剧"},
		{"cn_episode", "示例剧 第十二集 - 爱奇艺", "示例剧"},
		{"title_quotes", "《测试影片》 - 腾讯视频", "测试影片"},
		{"prefix_platform", "腾讯视频：Example Journey", "Example Journey"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeTitle(tc.in); got != tc.want {
				t.Fatalf("%q != %q", got, tc.want)
			}
		})
	}
	for _, tc := range []struct {
		in string
		n  int
	}{{"第十二集", 12}, {"第１２集", 12}, {"EP003", 3}, {"Episode 9", 9}, {"第〇二集", 2}, {"第一百零二集", 102}, {"01", 1}, {"十五", 15}, {"一千零一", 1001}, {"十二", 12}} {
		t.Run("number_"+tc.in, func(t *testing.T) {
			var n *int
			if strings.Contains(tc.in, "集") || strings.HasPrefix(tc.in, "E") {
				n = episodeNumber(tc.in)
			} else {
				n = number(tc.in)
			}
			require(t, n != nil && *n == tc.n, fmt.Sprint(n))
		})
	}
	for _, s := range []string{"预告第1集", "SP 01", "特别篇1", "https://www.bilibili.com/bangumi/play/ep15014", "ep15014", "abc"} {
		t.Run("not_ordinal_"+s, func(t *testing.T) { require(t, episodeNumber(s) == nil, "unexpected ordinal") })
	}
	t.Run("strip_html", func(t *testing.T) {
		require(t, text("<p>Hi</p><script>evil()</script><style>x</style>", 50) == "Hi", "unsafe text")
	})
	t.Run("season_rank", func(t *testing.T) {
		require(t, ranking("Example Journey 第二季", "Example Journey 第二季", "2024") > ranking("Example Journey 第二季", "Example Journey 第一季", "2022"), "season ranking")
	})
	t.Run("all_groups", func(t *testing.T) {
		g := groupsFromVOD("第01集$https://a.example.com/a#第02集$https://a.example.com/b$$$第1集$https://b.example.com/a", "a$$$b")
		require(t, len(g) == 2 && len(g[0].Episodes) == 2 && len(g[1].Episodes) == 1, "lost groups")
		ep, e := chooseEpisode(g[1], "第01集")
		mustNoErr(t, e)
		require(t, ep.Name == "第1集", "wrong episode")
		_, e = chooseEpisode(g[1], "第02集")
		require(t, errCode(e) == "episode_missing", "fallback to first")
	})
	t.Run("part_labels", func(t *testing.T) {
		g := Group{Episodes: []Episode{{"1", "第1集上", "https://a.example.com/a", intPtr(1)}, {"2", "第1集下", "https://a.example.com/b", intPtr(1)}}}
		_, e := chooseEpisode(g, "第1集")
		require(t, e != nil, "ambiguity ignored")
	})
	t.Run("invalid_media", func(t *testing.T) {
		g := groupsFromVOD("a$javascript:alert(1)#b$file:///x#c$https://u:p@example.com/a#d$https://a.example.com/ok", "")
		require(t, len(g) == 1 && len(g[0].Episodes) == 1, "unsafe media accepted")
	})
	t.Run("public_hides_url", func(t *testing.T) {
		f, _ := filmFromVOD(fixtureRows()[0])
		b, _ := json.Marshal(f.public())
		require(t, !strings.Contains(string(b), "cdn.example.com"), "raw media leaked")
	})
}
func TestSecurity(t *testing.T) {
	for _, s := range []string{"http://127.0.0.1/", "http://localhost/", "http://10.0.0.1/", "http://[::1]/", "http://[::ffff:127.0.0.1]/", "http://192.168.1.1/", "http://2130706433/", "http://0177.0.0.1/", "http://0x7f000001/", "http://a.internal/", "http://printer/", "file:///etc/passwd", "javascript:alert(1)", "https://u:p@example.com/", "https://example.com:8888/", "https://example.com/\nX", "https://example.com\\x", "http://100.64.0.1/", "http://169.254.1.1/", "http://224.1.1.1/", "http://192.0.2.1/"} {
		t.Run(s, func(t *testing.T) { _, e := publicURL(s); require(t, e != nil, "unsafe URL accepted") })
	}
	for _, s := range []string{"https://example.com/a?b=c", "https://8.8.8.8/", "https://[2606:4700:4700::1111]/"} {
		t.Run("public_"+s, func(t *testing.T) { _, e := publicURL(s); mustNoErr(t, e) })
	}
	t.Run("strip_private_parameters", func(t *testing.T) {
		p, u, e := platformURL("https://v.qq.com/x/cover/a/b.html?token=secret&vid=abc&foo=bar#frag", "")
		mustNoErr(t, e)
		require(t, p == "qq" && u == "https://v.qq.com/x/cover/a/b.html?vid=abc", u)
	})
	for _, tc := range [][2]string{{"https://v.qq.com.evil.com/x/cover/a.html", ""}, {"https://v.qq.com/x/search/?q=a", ""}, {"https://v.qq.com/x/cover/a.html", "iqiyi"}, {"http://v.qq.com/x/cover/a.html", ""}} {
		t.Run("platform_"+tc[0]+tc[1], func(t *testing.T) { _, _, e := platformURL(tc[0], tc[1]); require(t, e != nil, "bad platform") })
	}
	t.Run("query_encode", func(t *testing.T) {
		u, e := officialSearch("qq", "a & b")
		mustNoErr(t, e)
		require(t, strings.Contains(u, "%26") && strings.Contains(u, "%20"), u)
	})
}
func fixtureRows() []Obj {
	return []Obj{
		{"vod_id": 101, "vod_name": "Example Journey 第二季", "vod_year": "2024", "type_name": "合成测试剧", "vod_content": "<p>测试简介</p><script>not executed</script>", "vod_play_from": "alpha$$$beta", "vod_play_url": "第01集$https://cdn.example.com/a1.mp4#第02集$https://cdn.example.com/a2.mp4$$$第1集$https://cdn.example.com/b1.mp4"},
		{"vod_id": 102, "vod_name": "Example Journey 第一季", "vod_year": "2022", "type_name": "合成测试剧", "vod_play_from": "alpha", "vod_play_url": "第1集$https://cdn.example.com/s1.mp4"},
	}
}

type fakeNetwork struct {
	mu                       sync.Mutex
	calls                    int
	failSuggest, failDetails bool
}

func (n *fakeNetwork) JSON(_ context.Context, base string, p Obj) (Obj, error) {
	n.mu.Lock()
	n.calls++
	n.mu.Unlock()
	rr := fixtureRows()
	a := []any{}
	if strings.Contains(base, "suggest") {
		if n.failSuggest {
			return nil, problem("fixture suggest failure", "network", 502)
		}
		for _, r := range rr {
			a = append(a, Obj{"id": r["vod_id"], "name": r["vod_name"]})
		}
		return Obj{"list": a}, nil
	}
	if n.failDetails {
		return nil, problem("fixture details failure", "network", 502)
	}
	ids := strings.Split(sget(p, "ids"), ",")
	for _, r := range rr {
		for _, id := range ids {
			if id == "" || id == str(r["vod_id"]) {
				a = append(a, r)
				break
			}
		}
	}
	return Obj{"list": a}, nil
}
func (n *fakeNetwork) Fetch(_ context.Context, _ string, _ int) ([]byte, Obj, error) {
	n.mu.Lock()
	n.calls++
	n.mu.Unlock()
	return []byte(`<a href="https://v.qq.com/x/cover/test/demo.html">Example Journey</a>`), Obj{"http": 200}, nil
}
func (n *fakeNetwork) Probe(_ context.Context, name, link string) Obj {
	return Obj{"name": name, "endpoint": link, "http": 200, "state": "fixture HTTP only", "playback_verified": false}
}
func fixtureService(t *testing.T) (*Service, *fakeNetwork) {
	t.Helper()
	n := &fakeNetwork{}
	s, e := newService(t.TempDir(), n)
	mustNoErr(t, e)
	attachTestFixtures(s)
	return s, n
}
func allowNetwork(t *testing.T, s *Service) {
	t.Helper()
	_, e := s.Settings(Obj{"network_consent": true})
	mustNoErr(t, e)
}
func demoPayload() Obj { return Obj{"film_id": "demo-orbit-1", "group_key": "g0", "episode_key": "e0"} }
func TestService(t *testing.T) {
	tests := map[string]func(*testing.T, *Service, *fakeNetwork){
		"offline_default": func(t *testing.T, s *Service, n *fakeNetwork) {
			require(t, s.State()["network_consent"] == false, "network default")
			_, e := s.Search(ctx, Obj{"query": "x"})
			require(t, errCode(e) == "consent_required" && n.calls == 0, "consent not respected")
		},
		"demo_search_removed": func(t *testing.T, s *Service, n *fakeNetwork) {
			_, e := s.Search(ctx, Obj{"demo": true})
			require(t, errCode(e) == "query" && n.calls == 0, "demo search still enabled")
		},
		"catalog_search": func(t *testing.T, s *Service, n *fakeNetwork) {
			allowNetwork(t, s)
			o, e := s.Search(ctx, Obj{"query": "Example Journey 第二季"})
			mustNoErr(t, e)
			require(t, sget(o["items"].([]Obj)[0], "id") == "101", "wrong order")
			f, e := s.Detail(ctx, "101")
			mustNoErr(t, e)
			require(t, len(f.Groups) == 2, "lost groups")
		},
		"fallback": func(t *testing.T, s *Service, n *fakeNetwork) {
			allowNetwork(t, s)
			n.failSuggest = true
			o, e := s.Search(ctx, Obj{"query": "Example"})
			mustNoErr(t, e)
			require(t, len(o["items"].([]Obj)) == 2, "no fallback")
		},
		"summary_on_detail_error": func(t *testing.T, s *Service, n *fakeNetwork) {
			allowNetwork(t, s)
			n.failDetails = true
			o, e := s.Search(ctx, Obj{"query": "Example"})
			mustNoErr(t, e)
			require(t, len(o["items"].([]Obj)) == 2, "no summaries")
		},
		"not_found": func(t *testing.T, s *Service, n *fakeNetwork) {
			allowNetwork(t, s)
			_, e := s.Detail(ctx, "999")
			require(t, errCode(e) == "not_found", "wrong error")
		},
		"cache": func(t *testing.T, s *Service, n *fakeNetwork) {
			allowNetwork(t, s)
			_, e := s.Search(ctx, Obj{"query": "Example"})
			mustNoErr(t, e)
			calls := n.calls
			_, e = s.Search(ctx, Obj{"query": "Example"})
			mustNoErr(t, e)
			require(t, n.calls == calls, "no cache")
		},
		"demo_history": func(t *testing.T, s *Service, n *fakeNetwork) {
			o, e := s.Resolve(ctx, demoPayload())
			mustNoErr(t, e)
			require(t, o["kind"] == "media" && o["playback_verified"] == false, "wrong resolve")
			require(t, s.store.History()[0]["evidence"] == "opened", "false evidence")
		},
		"catalog_resolve": func(t *testing.T, s *Service, n *fakeNetwork) {
			allowNetwork(t, s)
			o, e := s.Resolve(ctx, Obj{"film_id": "101", "group_key": "g0", "episode_key": "e0"})
			mustNoErr(t, e)
			require(t, o["kind"] == "page" && strings.Contains(sget(o, "url"), "%3A%2F%2F"), "bad player URL")
		},
		"binding_required": func(t *testing.T, s *Service, n *fakeNetwork) {
			allowNetwork(t, s)
			_, e := s.Resolve(ctx, Obj{"film_id": "101", "group_key": "g0", "episode_key": "e0", "source": "qilin"})
			require(t, errCode(e) == "binding_required", "no binding gate")
		},
		"hls_disabled": func(t *testing.T, s *Service, n *fakeNetwork) {
			allowNetwork(t, s)
			_, e := s.Resolve(ctx, Obj{"film_id": "101", "group_key": "g0", "episode_key": "e0", "source": "hls", "platform_url": "https://v.qq.com/x/cover/a/b.html"})
			require(t, errCode(e) == "source_disabled", "hls enabled")
		},
		"parser_encoded": func(t *testing.T, s *Service, n *fakeNetwork) {
			allowNetwork(t, s)
			o, e := s.Resolve(ctx, Obj{"film_id": "101", "group_key": "g0", "episode_key": "e0", "source": "qilin", "platform_url": "https://v.qq.com/x/cover/a/b.html?token=secret"})
			mustNoErr(t, e)
			require(t, !strings.Contains(sget(o, "url"), "secret") && strings.Contains(sget(o, "url"), "maccms"), "URL leaked")
		},
		"import_offline": func(t *testing.T, s *Service, n *fakeNetwork) {
			o, e := s.Import(Obj{"url": "https://v.qq.com/x/cover/a/b.html", "title": "测试"})
			mustNoErr(t, e)
			require(t, strings.HasPrefix(sget(o, "id"), "link-") && n.calls == 0, "import not offline")
		},
		"locator": func(t *testing.T, s *Service, n *fakeNetwork) {
			allowNetwork(t, s)
			o, e := s.Search(ctx, Obj{"query": "Example Journey", "mode": "qq"})
			mustNoErr(t, e)
			require(t, len(o["items"].([]Obj)) == 1, "locator failed")
		},
		"missing_episode": func(t *testing.T, s *Service, n *fakeNetwork) {
			_, _, _, e := s.Select(ctx, Obj{"film_id": "demo-orbit-1", "group_key": "g1", "match_name": "第02集"})
			require(t, errCode(e) == "episode_missing", "wrong auto match")
		},
		"favorite_roundtrip": func(t *testing.T, s *Service, n *fakeNetwork) {
			f, e := s.Detail(ctx, "demo-orbit-1")
			mustNoErr(t, e)
			saved, e := s.store.Toggle(f.public())
			mustNoErr(t, e)
			require(t, saved && len(s.store.Favorites()) == 1, "not saved")
			saved, e = s.store.Toggle(f.public())
			mustNoErr(t, e)
			require(t, !saved && len(s.store.Favorites()) == 0, "not removed")
		},
		"progress": func(t *testing.T, s *Service, n *fakeNetwork) {
			o, e := s.Resolve(ctx, demoPayload())
			mustNoErr(t, e)
			mustNoErr(t, s.store.Progress(sget(o, "history_id"), "browser_time_advanced", 3.2))
			require(t, s.store.History()[0]["position"] == 3.2, "progress lost")
		},
		"bad_progress": func(t *testing.T, s *Service, n *fakeNetwork) {
			for _, v := range []any{math.NaN(), math.Inf(1), -1, 604801, "3", true} {
				require(t, s.store.Progress("x", "browser_time_advanced", v) != nil, "bad progress accepted")
			}
		},
		"history_optout": func(t *testing.T, s *Service, n *fakeNetwork) {
			_, e := s.Settings(Obj{"save_history": false})
			mustNoErr(t, e)
			o, e := s.Resolve(ctx, demoPayload())
			mustNoErr(t, e)
			require(t, len(s.store.History()) == 0 && o["history_id"] == "", "history optout failed")
		},
		"settings_types": func(t *testing.T, s *Service, n *fakeNetwork) {
			_, e := s.Settings(Obj{"network_consent": "yes"})
			require(t, e != nil, "bad settings")
			_, e = s.Settings(Obj{"enabled_sources": Obj{"bad": true}})
			require(t, e != nil, "unknown source")
		},
		"diagnostics_not_playback": func(t *testing.T, s *Service, n *fakeNetwork) {
			allowNetwork(t, s)
			o, e := s.Diagnose(ctx)
			mustNoErr(t, e)
			require(t, o["playback_verified"] == false, "false playable")
			for _, r := range o["results"].([]Obj) {
				require(t, r["playback_verified"] == false, "false row")
			}
		},
		"report_redacted": func(t *testing.T, s *Service, n *fakeNetwork) {
			allowNetwork(t, s)
			_, e := s.Search(ctx, Obj{"query": "Example"})
			mustNoErr(t, e)
			b, _ := json.Marshal(s.Report())
			require(t, !strings.Contains(string(b), "a1.mp4") && !strings.Contains(string(b), "Example Journey"), "private report")
		},
		"store_restart": func(t *testing.T, s *Service, n *fakeNetwork) {
			f, _ := s.Detail(ctx, "demo-orbit-1")
			_, e := s.store.Toggle(f.public())
			mustNoErr(t, e)
			other, e := newService(s.dir, n)
			mustNoErr(t, e)
			require(t, len(other.store.Favorites()) == 1, "lost persisted favorite")
		},
		"link_restore": func(t *testing.T, s *Service, n *fakeNetwork) {
			o, e := s.Import(Obj{"url": "https://v.qq.com/x/cover/a/b.html", "title": "测试"})
			mustNoErr(t, e)
			_, e = s.store.Toggle(o)
			mustNoErr(t, e)
			other, e := newService(s.dir, n)
			mustNoErr(t, e)
			f, e := other.Detail(ctx, sget(o, "id"))
			mustNoErr(t, e)
			require(t, f.Title == "测试" && len(f.Groups) == 1, "link restore failed")
		},
		"clear_history": func(t *testing.T, s *Service, n *fakeNetwork) {
			_, e := s.Resolve(ctx, demoPayload())
			mustNoErr(t, e)
			mustNoErr(t, s.store.ClearHistory())
			require(t, len(s.store.History()) == 0, "history not empty")
		},
	}
	for name, fn := range tests {
		t.Run(name, func(t *testing.T) { s, n := fixtureService(t); fn(t, s, n) })
	}
}
func TestStorageCorruptNotOverwritten(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "library.json")
	mustNoErr(t, os.WriteFile(p, []byte("broken"), 0600))
	_, e := newStore(dir)
	require(t, e != nil, "corrupt accepted")
	b, _ := os.ReadFile(p)
	require(t, string(b) == "broken", "data overwritten")
}
func TestNoRemoteScriptsInConfig(t *testing.T) {
	dir := t.TempDir()
	raw, _ := assets.ReadFile("sources.json")
	var c Config
	json.Unmarshal(raw, &c)
	c.Parsers[0].Template = "javascript:{url}"
	b, _ := json.Marshal(c)
	os.WriteFile(filepath.Join(dir, "sources.json"), b, 0600)
	_, e := loadConfig(dir)
	require(t, e != nil, "unsafe config")
}
func TestConfigDoesNotMutateGlobal(t *testing.T) {
	s, _ := fixtureService(t)
	before := s.config.Parsers
	s.Settings(Obj{"enabled_sources": Obj{"qilin": false}})
	require(t, reflect.DeepEqual(before, s.config.Parsers), "mutated config")
}
