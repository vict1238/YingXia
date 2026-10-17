// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

type mappedNetwork struct {
	mu       sync.Mutex
	data     map[string]string
	fail     map[string]bool
	requests []RequestSpec
	handle   func(RequestSpec) ([]byte, Obj, error)
}

func (n *mappedNetwork) Request(ctx context.Context, r RequestSpec) ([]byte, Obj, error) {
	n.mu.Lock()
	n.requests = append(n.requests, r)
	bad := n.fail[r.URL]
	body, ok := n.data[r.URL]
	handler := n.handle
	n.mu.Unlock()
	if bad {
		return nil, nil, problem("测试来源超时", "timeout", 504)
	}
	if ok {
		return []byte(body), Obj{"content_type": "text/html"}, nil
	}
	if handler != nil {
		return handler(r)
	}
	return nil, nil, problem("未配置测试响应", "fixture", 502)
}
func (n *mappedNetwork) Fetch(ctx context.Context, u string, limit int) ([]byte, Obj, error) {
	return n.Request(ctx, RequestSpec{URL: u, Method: "GET", Limit: limit})
}
func (n *mappedNetwork) JSON(ctx context.Context, u string, params Obj) (Obj, error) {
	p, _ := url.Parse(u)
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
func (n *mappedNetwork) Probe(ctx context.Context, name, u string) Obj {
	_, _, e := n.Fetch(ctx, u, 100000)
	if e != nil {
		return Obj{"name": name, "state": e.Error(), "playback_verified": false}
	}
	return Obj{"name": name, "http": 200, "state": "fixture HTTP only", "playback_verified": false}
}
func ruleFixture() KazumiRule {
	return KazumiRule{API: "8", Name: "TestAnime", Version: "1", BaseURL: "https://anime.example.com/", SearchURL: "https://anime.example.com/search?wd=@keyword", SearchList: "//div[@class='item']", SearchName: "//a[@class='name']", SearchResult: "//a[@class='name']", ChapterRoads: "//ul[@class='road']", ChapterResult: "//li/a"}
}
func mappedService(t *testing.T, n *mappedNetwork) *Service {
	t.Helper()
	s, e := newService(t.TempDir(), n)
	mustNoErr(t, e)
	allowNetwork(t, s)
	return s
}

func TestMultiSource(t *testing.T) {
	n := &mappedNetwork{data: map[string]string{}, fail: map[string]bool{}}
	n.handle = func(r RequestSpec) ([]byte, Obj, error) {
		u, _ := url.Parse(r.URL)
		if u.Host == "bad.example.com" {
			return nil, nil, problem("测试来源超时", "timeout", 504)
		}
		if strings.Contains(r.URL, "suggest") {
			return []byte(`{"list":[]}`), nil, nil
		}
		return []byte(`{"list":[{"vod_id":101,"vod_name":"示例 第二季","vod_year":"2024","vod_play_url":"第1集$https://media.example.com/a.m3u8","vod_play_from":"main"}]}`), nil, nil
	}
	s := mappedService(t, n)
	r := s.manager.snapshot()
	r.Providers = []Provider{{ID: "first", Name: "来源一", Kind: "cms", URL: "https://one.example.com/api", Enabled: true, Origin: "custom"}, {ID: "second", Name: "来源二", Kind: "cms", URL: "https://two.example.com/api", Enabled: true, Origin: "custom"}, {ID: "bad", Name: "故障源", Kind: "cms", URL: "https://bad.example.com/api", Enabled: true, Origin: "custom"}}
	mustNoErr(t, s.manager.save(r))
	result, e := s.Search(ctx, Obj{"query": "示例 第二季", "mode": "all"})
	mustNoErr(t, e)
	items := result["items"].([]Obj)
	require(t, len(items) == 2, "partial success lost")
	require(t, items[0]["id"] != items[1]["id"], "cross-source ID collision")
	require(t, result["successful_sources"] == 2, "wrong source status")
	f, e := s.Detail(ctx, sget(items[0], "id"))
	mustNoErr(t, e)
	_, e = s.store.Toggle(f.public())
	mustNoErr(t, e)
	other, e := newService(s.dir, n)
	mustNoErr(t, e)
	f, e = other.Detail(ctx, f.ID)
	mustNoErr(t, e)
	require(t, f.Source == sget(items[0], "source"), "restored wrong source")
	_, e = s.Settings(Obj{"enabled_providers": Obj{"first": false}})
	mustNoErr(t, e)
	result, e = s.Search(ctx, Obj{"query": "示例 第二季", "mode": "all"})
	mustNoErr(t, e)
	require(t, len(result["items"].([]Obj)) == 1, "disabled source queried")
	_, e = s.Settings(Obj{"network_consent": false})
	mustNoErr(t, e)
	n.mu.Lock()
	before := len(n.requests)
	n.mu.Unlock()
	_, e = s.Search(ctx, Obj{"query": "示例", "mode": "all"})
	require(t, errCode(e) == "consent_required", "missing consent gate")
	n.mu.Lock()
	require(t, len(n.requests) == before, "request made without consent")
	n.mu.Unlock()
}
func TestKazumiAdapters(t *testing.T) {
	rule := ruleFixture()
	p := Provider{ID: "test", Name: "TestAnime", Kind: "kazumi", Rule: &rule, Enabled: true}
	n := &mappedNetwork{data: map[string]string{"https://anime.example.com/search?wd=test": `<div class="item"><a class="name" href="/show/123">测试番 第二季</a></div>`, "https://anime.example.com/show/123": `<h1>测试番 第二季</h1><ul class="road"><li><a href="/play/1">第01集</a></li><li><a href="/play/2">第02集</a></li></ul><ul class="road"><li><a href="/other/1">第1集</a></li></ul>`}}
	t.Run("xpath_search_and_all_roads", func(t *testing.T) {
		fs, e := searchRule(ctx, n, p, "test")
		mustNoErr(t, e)
		require(t, len(fs) == 1 && fs[0].Title == "测试番 第二季", "bad XPath result")
		_, _, ref, e := splitSourceFilmID(fs[0].ID)
		mustNoErr(t, e)
		f, e := detailRule(ctx, n, p, ref)
		mustNoErr(t, e)
		require(t, len(f.Groups) == 2 && len(f.Groups[0].Episodes) == 2 && len(f.Groups[1].Episodes) == 1, "roads leaked into each other")
		_, e = chooseEpisode(f.Groups[1], "第02集")
		require(t, errCode(e) == "episode_missing", "missing episode fallback")
	})
	t.Run("api_post_and_nested_chapters", func(t *testing.T) {
		r := ruleFixture()
		r.SearchMode = "api"
		r.ChapterMode = "api"
		r.SearchAPI = RuleSearchAPI{Request: RuleRequest{Method: "POST", URL: "https://api.example.com/search", BodyType: "json", Body: Obj{"q": "@keyword"}}, ListPath: "$[*]", NamePath: "$.title", SourcePath: "$.id"}
		r.ChapterAPI = RuleChapterAPI{Request: RuleRequest{Method: "GET", URL: "https://api.example.com/detail/@source"}, RoadsPath: "$.sources[*]", RoadNamePath: "$.name", EpisodesPath: "$.episodes[*]", EpisodeNamePath: "$.title", EpisodeURLPath: "$.id", EpisodePage: RuleEpisodePage{URL: "https://anime.example.com/anime/@source/play/@episodeUrl"}}
		mustNoErr(t, validateRule(r))
		p.Rule = &r
		n.data["https://api.example.com/search"] = `[{"id":"123","title":"测试番"}]`
		n.data["https://api.example.com/detail/123"] = `{"sources":[{"name":"line","episodes":[{"title":"第十二集","id":"ep12"}]}]}`
		fs, e := searchRule(ctx, n, p, "test & query")
		mustNoErr(t, e)
		require(t, len(fs) == 1, "no API result")
		last := n.requests[len(n.requests)-1]
		var body Obj
		mustNoErr(t, json.Unmarshal(last.Body, &body))
		require(t, body["q"] == "test & query" && last.Method == "POST", "bad API substitution")
		f, e := detailRule(ctx, n, p, "123")
		mustNoErr(t, e)
		require(t, f.Groups[0].Episodes[0].URL == "https://anime.example.com/anime/123/play/ep12", "bad episode page")
		require(t, *f.Groups[0].Episodes[0].Ordinal == 12, "Chinese episode lost")
	})
	t.Run("private_and_future_rules_rejected", func(t *testing.T) {
		r := ruleFixture()
		r.API = "9"
		require(t, errCode(validateRule(r)) == "rule_api", "unknown API accepted")
		r = ruleFixture()
		r.SearchURL = "http://127.0.0.1/?q=@keyword"
		require(t, validateRule(r) != nil, "private URL accepted")
	})
}
func TestCMSImportAtomic(t *testing.T) {
	s := mappedService(t, &mappedNetwork{})
	before := len(s.manager.providers())
	_, e := s.manager.addCMS(Obj{"items": []any{Obj{"name": "one", "url": "https://one.example.com/api"}, Obj{"name": "private", "url": "http://127.0.0.1/api"}}})
	require(t, e != nil && len(s.manager.providers()) == before, "partial invalid import applied")
	_, e = s.manager.addCMS(Obj{"items": []any{Obj{"name": "one", "url": "https://one.example.com/api"}, Obj{"name": "two", "url": "https://two.example.com/api"}}})
	mustNoErr(t, e)
	require(t, len(s.manager.providers()) == before+2, "import missing sources")
	_, e = s.manager.removeCMS("wsyzy")
	require(t, e != nil, "built-in source removed")
}

func updaterFixture(t *testing.T) (*Service, *mappedNetwork) {
	t.Helper()
	n := &mappedNetwork{data: map[string]string{}, fail: map[string]bool{}}
	s := mappedService(t, n)
	sha := strings.Repeat("a", 40)
	for _, repo := range []string{"88lin/video_vip", "Predidit/KazumiRules", "Predidit/Kazumi"} {
		n.data["https://api.github.com/repos/"+repo+"/commits/main"] = `{"sha":"` + sha + `"}`
	}
	script := `// @version 3.3.0
 const SITE='https://new.example.com'; const API='https://api.new.example.com/api.php/provide/vod/'; const PLAYER='https://new.example.com/player?url=';
 const config={videoParseList:[{"name":"麒麟解析","url":"https://free.maccms.xyz/?url="},{"name":"新增线路","url":"https://new.example.com/?url="}]};`
	n.data["https://raw.githubusercontent.com/88lin/video_vip/"+sha+"/video_vip.user.js"] = script
	n.data["https://raw.githubusercontent.com/Predidit/KazumiRules/"+sha+"/index.json"] = `[{"name":"TestAnime","version":"1"}]`
	rule := ruleFixture()
	b, _ := json.Marshal(rule)
	n.data["https://raw.githubusercontent.com/Predidit/KazumiRules/"+sha+"/TestAnime.json"] = string(b)
	n.data["https://raw.githubusercontent.com/Predidit/Kazumi/"+sha+"/pubspec.yaml"] = "version: 2.4.0+20400\n"
	n.data["https://raw.githubusercontent.com/Predidit/Kazumi/"+sha+"/lib/request/config/api_endpoints.dart"] = "static const int apiLevel = 8;"
	return s, n
}
func TestSourceUpdates(t *testing.T) {
	t.Run("apply_preserves_custom_and_switches", func(t *testing.T) {
		s, _ := updaterFixture(t)
		_, e := s.manager.addCMS(Obj{"name": "custom", "url": "https://custom.example.com/api"})
		mustNoErr(t, e)
		_, e = s.Settings(Obj{"enabled_sources": Obj{"qilin": false}, "enabled_providers": Obj{"wsyzy": false}})
		mustNoErr(t, e)
		_, e = s.manager.update(ctx)
		mustNoErr(t, e)
		r := s.manager.snapshot()
		require(t, len(r.Providers) == 3, "managed refresh removed custom")
		require(t, r.Catalog.APIURL == "https://api.new.example.com/api.php/provide/vod/", "catalog not refreshed")
		require(t, r.Upstreams["88lin"].Version == "3.3.0", "version not tracked")
		for _, p := range s.enabledSources() {
			if p.ID == "qilin" {
				require(t, !p.Enabled, "user parser switch overwritten")
			}
		}
		p, _ := s.manager.provider("wsyzy")
		require(t, !p.Enabled, "provider switch overwritten")
		before := r.LastUpdatedAt
		_, e = s.manager.update(ctx)
		mustNoErr(t, e)
		require(t, s.manager.snapshot().LastUpdatedAt == before, "unchanged commit reapplied")
	})
	t.Run("bad_bundle_keeps_previous_rules", func(t *testing.T) {
		s, n := updaterFixture(t)
		sha := strings.Repeat("a", 40)
		original := n.data["https://raw.githubusercontent.com/Predidit/KazumiRules/"+sha+"/TestAnime.json"]
		n.data["https://raw.githubusercontent.com/Predidit/KazumiRules/"+sha+"/TestAnime.json"] = `{"name":"TestAnime","version":"1","api":"9"}`
		before := len(s.manager.providers())
		_, e := s.manager.update(ctx)
		mustNoErr(t, e)
		r := s.manager.snapshot()
		require(t, len(r.Providers) == before, "bad rules replaced previous bundle")
		require(t, r.Upstreams["kazumi_rules"].NeedsAppUpdate, "missing engine upgrade signal")
		require(t, r.Upstreams["88lin"].Version == "3.3.0", "independent component was lost")
		n.data["https://raw.githubusercontent.com/Predidit/KazumiRules/"+sha+"/TestAnime.json"] = original
		_, e = s.manager.update(ctx)
		mustNoErr(t, e)
		require(t, !s.manager.snapshot().Upstreams["kazumi_rules"].NeedsAppUpdate, "resolved compatibility flag was retained")
	})
	t.Run("offline_and_failure_status", func(t *testing.T) {
		s, n := updaterFixture(t)
		_, e := s.Settings(Obj{"network_consent": false})
		mustNoErr(t, e)
		_, e = s.manager.update(ctx)
		require(t, errCode(e) == "consent_required" && len(n.requests) == 0, "offline updater contacted network")
		allowNetwork(t, s)
		n.fail["https://api.github.com/repos/88lin/video_vip/commits/main"] = true
		before := s.manager.snapshot().Catalog.APIURL
		_, e = s.manager.update(ctx)
		mustNoErr(t, e)
		r := s.manager.snapshot()
		require(t, r.Catalog.APIURL == before && r.Upstreams["88lin"].Error != "", "failed upstream overwritten")
	})
}
func TestHLSRelay(t *testing.T) {
	m := newMediaRelay(&mappedNetwork{})
	body := []byte("#EXTM3U\n#EXT-X-KEY:METHOD=AES-128,URI=\"key.bin\"\n#EXT-X-MAP:URI=\"init.mp4\"\n#EXTINF:1,\nseg.ts?token=abc\n")
	out, e := m.rewritePlaylist(body, "https://media.example.com/folder/list.m3u8", map[string]string{"Referer": "https://anime.example.com/"})
	mustNoErr(t, e)
	require(t, !strings.Contains(string(out), "key.bin") && strings.Contains(string(out), "/api/media?ticket="), "URLs not rewritten")
	require(t, len(m.grants) == 3, "missing key or map grant")
	for _, g := range m.grants {
		require(t, strings.HasPrefix(g.URL, "https://media.example.com/folder/"), "wrong relative resolution")
		require(t, g.Headers["Referer"] == "https://anime.example.com/", "referer lost")
	}
	_, e = m.rewritePlaylist([]byte("#EXTM3U\nhttp://127.0.0.1/secret\n"), "https://media.example.com/x", nil)
	require(t, e != nil, "private playlist child accepted")
	_, e = m.grant("unknown")
	require(t, errCode(e) == "media_expired", "unknown ticket allowed")
}
func TestStaticMediaExtraction(t *testing.T) {
	for _, tc := range []struct{ body, url string }{{`<video src="/clip.mp4"></video>`, "https://site.example.com/clip.mp4"}, {`var player_aaaa={"encrypt":0,"url":"https://media.example.com/a.m3u8"};`, "https://media.example.com/a.m3u8"}, {`<script>var player_aaaa={"encrypt":1,"url":"https%3A%2F%2Fmedia.example.com%2Fa.m3u8"};</script>`, "https://media.example.com/a.m3u8"}} {
		u, _ := extractPageMedia([]byte(tc.body), "https://site.example.com/page")
		require(t, u == tc.url, fmt.Sprint(u))
	}
}

func TestRegistryRecoveryPreservesGoodBackup(t *testing.T) {
	s, _ := fixtureService(t)
	m := s.manager
	mustNoErr(t, m.save(m.snapshot()))
	mustNoErr(t, m.save(m.snapshot()))
	mustNoErr(t, os.WriteFile(m.path, []byte("{broken"), 0600))
	recovered, e := newSourceManager(s.dir, s.config, s.network, s.store)
	mustNoErr(t, e)
	mustNoErr(t, recovered.save(recovered.snapshot()))
	b, e := os.ReadFile(m.path + ".previous")
	mustNoErr(t, e)
	var r SourceRegistry
	mustNoErr(t, json.Unmarshal(b, &r))
	mustNoErr(t, validateRegistry(r))
}

type mediaTestNetwork struct {
	*mappedNetwork
	open func(RequestSpec) *http.Response
}

func (n *mediaTestNetwork) Open(ctx context.Context, r RequestSpec) (*http.Response, error) {
	return n.open(r), nil
}
func TestMediaHTTPBoundary(t *testing.T) {
	srv := httpFixture(t)
	opened := 0
	n := &mediaTestNetwork{mappedNetwork: &mappedNetwork{}, open: func(spec RequestSpec) *http.Response {
		opened++
		require(t, spec.Headers["Range"] == "bytes=1-3", "range not forwarded")
		return &http.Response{StatusCode: 206, Header: http.Header{"Content-Type": []string{"video/mp4"}, "Content-Range": []string{"bytes 1-3/6"}, "Content-Length": []string{"3"}}, Body: io.NopCloser(strings.NewReader("bcd"))}
	}}
	srv.service.media = newMediaRelay(n)
	link, e := srv.service.media.register("https://media.example.com/a.mp4", nil)
	mustNoErr(t, e)
	require(t, perform(srv, link, "GET", nil, false, nil).Code == 403, "unauthenticated relay allowed")
	require(t, perform(srv, link, "GET", nil, true, nil).Code == 403, "offline relay allowed")
	require(t, opened == 0, "blocked relay reached network")
	allowNetwork(t, srv.service)
	w := perform(srv, link, "GET", nil, true, map[string]string{"Range": "bytes=1-3"})
	require(t, w.Code == 206 && w.Body.String() == "bcd" && w.Header().Get("Content-Range") == "bytes 1-3/6", "partial response lost")
	w = perform(srv, link, "HEAD", nil, true, map[string]string{"Range": "bytes=1-3"})
	require(t, w.Code == 206 && w.Body.Len() == 0, "HEAD returned body")
	require(t, perform(srv, link, "GET", nil, true, map[string]string{"Range": "bytes=0-2,4-5"}).Code == 416, "invalid range accepted")
	require(t, perform(srv, "/api/media?ticket=unknown", "GET", nil, true, nil).Code == 404, "unknown grant accepted")
}
func TestBrowserMediaSniff(t *testing.T) {
	if os.Getenv("YINGXIA_BROWSER_TEST") != "1" {
		t.Skip("manual browser check")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `<video muted autoplay></video><script>setTimeout(()=>document.querySelector('video').src='/test.m3u8',300)</script>`)
		} else if r.URL.Path == "/test.m3u8" {
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			fmt.Fprint(w, "#EXTM3U\n#EXT-X-ENDLIST\n")
		} else {
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	u, e := sniffMediaChecked(ctx, t.TempDir(), server.URL, func(ctx context.Context, u string) bool {
		return strings.HasPrefix(u, server.URL+"/") || u == server.URL
	})
	mustNoErr(t, e)
	require(t, u == server.URL+"/test.m3u8", "wrong dynamic media URL: "+u)
}
