// SPDX-License-Identifier: GPL-3.0-or-later
package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestTitleIndexPinyinInitialsFuzzyAndAmbiguity(t *testing.T) {
	x := newTitleIndex(t.TempDir())
	x.observe([]Obj{{"id": "a", "title": "庆余年", "year": "2019"}, {"id": "b", "title": "庆余年第二季", "year": "2024"}, {"id": "c", "title": "流浪地球"}})
	for _, q := range []string{"qingyunian", "QYN", "qing yu nian"} {
		r := x.suggest(q)
		if r["resolved"] != true || r["resolved_query"] != "庆余年" {
			t.Fatalf("pinyin %s: %v", q, r)
		}
	}
	for _, q := range []string{"庆于年", "qingyuniab", "lldq"} {
		r := x.suggest(q)
		items := r["items"].([]Obj)
		if len(items) == 0 {
			t.Fatalf("no fuzzy result for %s", q)
		}
	}
	r := x.suggest("庆余年")
	items := r["items"].([]Obj)
	if sget(items[0], "title") != "庆余年" {
		t.Fatal("exact title did not rank first")
	}
	x.observe([]Obj{{"title": "青玉鸟"}})
	if r := x.suggest("qyn"); r["resolved"] == true {
		t.Fatalf("ambiguous initials silently resolved: %v", r)
	}
	if cappedDistance(strings.Repeat("a", 120), strings.Repeat("b", 200), 2) <= 2 {
		t.Fatal("unrelated long strings matched")
	}
}

func TestTitleIndexPersistentBoundAndClose(t *testing.T) {
	dir := t.TempDir()
	x := newTitleIndex(dir)
	items := []Obj{}
	for i := 0; i < titleIndexLimit+20; i++ {
		items = append(items, Obj{"id": fmt.Sprint(i), "title": fmt.Sprintf("合成片名%04d", i)})
	}
	x.observe(items)
	if len(x.entries) != titleIndexLimit {
		t.Fatalf("unbounded index %d", len(x.entries))
	}
	st, e := os.Stat(x.path)
	if e != nil || st.Size() > titleIndexBytes {
		t.Fatal("index file byte limit", e)
	}
	reopened := newTitleIndex(dir)
	if len(reopened.entries) != titleIndexLimit {
		t.Fatal("index did not persist")
	}
	x.close()
	x.observe([]Obj{{"title": "关闭后不得写入"}})
	if len(x.suggest("关闭后")["items"].([]Obj)) != 0 {
		t.Fatal("retired runtime mutated title index")
	}
	if e = os.WriteFile(filepath.Join(dir, "title_index.json"), []byte("broken"), 0600); e != nil {
		t.Fatal(e)
	}
	if len(newTitleIndex(dir).entries) != 0 {
		t.Fatal("corrupt optional index must fall back empty")
	}
}

func TestPublicSuggestionsConsentFilteringCacheAndAmbiguity(t *testing.T) {
	var calls atomic.Int32
	f := &managementFetcher{fetchFn: func(ctx context.Context, u string, limit int) ([]byte, Obj, error) {
		calls.Add(1)
		if !strings.HasPrefix(u, "https://suggest.video.iqiyi.com/?key=") || limit > 128<<10 {
			t.Errorf("unbounded suggestion request %s %d", u, limit)
		}
		return []byte(`{"code":"A00000","data":[{"name":"庆余年","is_known_query":true,"year":2010,"link":"https://bad.example/play"},{"name":"庆余年第3季免费全集","is_known_query":false},{"name":"不同片名","is_known_query":true}]}`), Obj{"endpoint": "https://suggest.video.iqiyi.com/?key=qyn"}, nil
	}}
	s, e := newService(t.TempDir(), f)
	if e != nil {
		t.Fatal(e)
	}
	r, e := s.SearchSuggest(context.Background(), "qyn", true)
	if e != nil || r["resolved"] == true || calls.Load() != 0 {
		t.Fatal("suggestion bypassed network consent")
	}
	if e = s.store.SetMany(Obj{"network_consent": true}); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 2; i++ {
		r, e = s.SearchSuggest(context.Background(), "qyn", true)
		if e != nil || r["resolved_query"] != "庆余年" || r["scope"] != "local_and_public_suggestions" {
			t.Fatal(r, e)
		}
	}
	if calls.Load() != 1 {
		t.Fatal("public suggestion cache not reused")
	}
	items := r["items"].([]Obj)
	if len(items) != 1 || items[0]["year"] != nil || items[0]["link"] != nil {
		t.Fatal("untrusted provider fields imported", items)
	}
	if len(s.titles.entries) != 0 {
		t.Fatal("unverified public suggestion persisted as catalog metadata")
	}
	if e = s.store.SetMany(Obj{"network_consent": false}); e != nil {
		t.Fatal(e)
	}
	if r, _ = s.SearchSuggest(context.Background(), "qyn", true); r["resolved"] == true {
		t.Fatal("revoked consent reused remote suggestion")
	}
}

func TestResumeLatestEpisodeEndPolicyAndClear(t *testing.T) {
	s, e := newStore(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	f := Film{ID: "film", Title: "测试"}
	g := Group{Key: "g"}
	one := Episode{Key: "e1", Name: "第1集"}
	two := Episode{Key: "e2", Name: "第2集"}
	id, e := s.OpenedSelection(s.HistoryGeneration(), f, g, one, "source", "media")
	if e != nil {
		t.Fatal(e)
	}
	if e = s.ProgressDetails(id, "native_time_advanced", 42, 100, false); e != nil {
		t.Fatal(e)
	}
	id, e = s.OpenedSelection(s.HistoryGeneration(), f, g, two, "source", "media")
	if e != nil {
		t.Fatal(e)
	}
	if e = s.ProgressDetails(id, "native_time_advanced", 56, 200, false); e != nil {
		t.Fatal(e)
	}
	r := s.Resume(f.ID)
	if r["episode_key"] != "e2" || r["position"] != float64(56) || r["policy"] != "resume" {
		t.Fatal(r)
	}
	if _, e = s.OpenedSelection(s.HistoryGeneration(), f, g, two, "source", "media"); e != nil {
		t.Fatal(e)
	}
	if s.Resume(f.ID)["position"] != float64(56) {
		t.Fatal("resolve retry erased position")
	}
	for _, value := range []any{math.NaN(), math.Inf(1), -1, 700000} {
		if e = s.Progress(id, "native_time_advanced", value); e == nil {
			t.Fatal("accepted invalid position", value)
		}
	}
	if e = s.ProgressDetails(id, "native_time_advanced", 210, 200, false); e == nil {
		t.Fatal("accepted position after duration")
	}
	// Mark newest entry completed; EOF restarts this episode and never silently
	// advances into a different episode.
	latest := s.History()[0]
	id = sget(latest, "id")
	if e = s.ProgressDetails(id, "native_time_advanced", 195, 200, true); e != nil {
		t.Fatal(e)
	}
	r = s.Resume(f.ID)
	if r["position"] != 0 || r["policy"] != "restart" || r["episode_key"] != "e2" {
		t.Fatal(r)
	}
	generation := s.HistoryGeneration()
	if e = s.ClearHistory(); e != nil {
		t.Fatal(e)
	}
	if e = s.ProgressDetails(id, "native_time_advanced", 60, 200, false); e != nil {
		t.Fatal(e)
	}
	if stale, e := s.OpenedSelection(generation, f, g, two, "source", "media"); e != nil || stale != "" {
		t.Fatal("pending resolve restored history")
	}
	if s.Resume(f.ID)["found"] != false || len(s.History()) != 0 {
		t.Fatal("clear restored resume")
	}
}

func TestResumeLegacySelectionAndInvalidPosition(t *testing.T) {
	s := newManagementService(t, &managementFetcher{})
	f := Film{ID: "legacy", Title: "测试", Groups: []Group{{Key: "g", Episodes: []Episode{{Key: "e", Name: "第2集", Ordinal: intPtr(2)}}}}}
	s.links[f.ID] = f
	id, e := s.store.Opened(f.ID, f.Title, "2", "source", "media")
	if e != nil {
		t.Fatal(e)
	}
	if e = s.store.Progress(id, "browser_time_advanced", 12); e != nil {
		t.Fatal(e)
	}
	r, e := s.Resume(context.Background(), f.ID)
	if e != nil || r["group_key"] != "g" || r["episode_key"] != "e" {
		t.Fatal(r, e)
	}
	s.store.mu.Lock()
	s.store.data.History[0]["position"] = "NaN"
	s.store.mu.Unlock()
	if s.store.Resume(f.ID)["position"] != 0 {
		t.Fatal("invalid saved position used")
	}
}

func TestAlternativesIdentitySeasonAndEpisodeSafety(t *testing.T) {
	a := Film{ID: "a", Title: "庆余年 第二季", Year: "2024", Kind: "电视剧"}
	b := a
	b.ID = "b"
	if !sameAlternativeIdentity(a, b) {
		t.Fatal("equal identity rejected")
	}
	for _, mutate := range []func(*Film){func(f *Film) { f.Year = "2019" }, func(f *Film) { f.Year = "" }, func(f *Film) { f.Title = "庆余年 第三季" }, func(f *Film) { f.Title = "庆余年" }, func(f *Film) { f.Title = "庆余年 第二季 花絮" }} {
		c := b
		mutate(&c)
		if sameAlternativeIdentity(a, c) {
			t.Fatalf("unsafe identity accepted %+v", c)
		}
	}
	selected := Episode{Key: "a2", Name: "第2集", Ordinal: intPtr(2)}
	g := Group{Key: "g", Episodes: []Episode{{Key: "e1", Name: "第1集", Ordinal: intPtr(1)}, {Key: "e2", Name: "2", Ordinal: intPtr(2)}}}
	if ep, e := matchingAlternativeEpisode(a, selected, b, g); e != nil || ep.Key != "e2" {
		t.Fatal("ordinal mapping failed", ep, e)
	}
	g.Episodes = append(g.Episodes, Episode{Key: "e2b", Name: "第02话", Ordinal: intPtr(2)})
	if _, e := matchingAlternativeEpisode(a, selected, b, g); e == nil {
		t.Fatal("ambiguous episode mapping accepted")
	}
	if _, e := matchingAlternativeEpisode(a, Episode{Name: "第2集上", Ordinal: intPtr(2)}, b, g); e == nil {
		t.Fatal("split episode mapped to full episode")
	}
}

func TestExactEpisodeProbeAndAlternativeJob(t *testing.T) {
	good := "https://media.example.org/good.webm"
	bad := "https://media.example.org/bad.webm"
	n := nativeFixtureFetcher{good: string([]byte{0x1a, 0x45, 0xdf, 0xa3, 1, 2, 3, 4, 5, 6, 7, 8}), bad: "<html>not a video</html>"}
	s := newManagementService(t, n)
	// Avoid external catalogs in this deterministic same-film-group case.
	s.manager.mu.Lock()
	for i := range s.manager.state.Providers {
		s.manager.state.Providers[i].Enabled = false
	}
	s.manager.mu.Unlock()
	f := Film{ID: "exact", Title: "合成剧", Year: "2026", Groups: []Group{{Key: "old", Episodes: []Episode{{Key: "e1", Name: "第1集", Ordinal: intPtr(1), URL: good}, {Key: "e2", Name: "第2集", Ordinal: intPtr(2), URL: bad}}}, {Key: "new", Episodes: []Episode{{Key: "n1", Name: "第1集", Ordinal: intPtr(1), URL: bad}, {Key: "n2", Name: "第2集", Ordinal: intPtr(2), URL: good}}}}}
	s.links[f.ID] = f
	if a := s.searches.checker.CheckEpisode(context.Background(), f, "old", "e1"); a.State != "media_ok" {
		t.Fatal(a)
	}
	if a := s.searches.checker.CheckEpisode(context.Background(), f, "old", "e2"); a.State != "failed" {
		t.Fatal("episode 1 cached success leaked into episode 2", a)
	}
	j, e := s.alternatives.start(Obj{"film_id": f.ID, "group_key": "old", "episode_key": "e2", "position": 37})
	if e != nil {
		t.Fatal(e)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		j, e = s.alternatives.status(sget(j, "id"))
		if e != nil {
			t.Fatal(e)
		}
		if j["phase"] == "done" {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if j["phase"] != "done" {
		t.Fatal(j)
	}
	items := j["items"].([]Obj)
	if len(items) != 1 || items[0]["episode_key"] != "n2" || items[0]["position"] != float64(37) {
		t.Fatal(items)
	}
	if len(s.store.History()) != 0 {
		t.Fatal("suggestion created playback history")
	}
	raw, _ := json.Marshal(j)
	if strings.Contains(string(raw), good) || strings.Contains(string(raw), bad) {
		t.Fatal("suggestion leaked media URL")
	}
}

func TestV23PrivateRoutesAndSettings(t *testing.T) {
	r, e := newRuntime(t.TempDir(), &managementFetcher{})
	if e != nil {
		t.Fatal(e)
	}
	defer r.Stop()
	for _, path := range []string{"/api/search/suggest?q=qyn&online=0", "/api/resume?film_id=missing", "/api/playback/alternatives/status?id=missing"} {
		code, _, _ := nativeRequest(t, r, "GET", path, nil, false)
		if code != 403 {
			t.Fatal("new API lacks authentication", path, code)
		}
	}
	code, result, _ := nativeRequest(t, r, "GET", "/api/search/suggest?q=qyn&online=0", nil, true)
	if code != 200 {
		t.Fatal(result)
	}
	code, result, _ = nativeRequest(t, r, "POST", "/api/settings", Obj{"smart_source_switch": false, "skip_marked_ads": false, "auto_update_sources": false}, true)
	if code != 200 {
		t.Fatal(result)
	}
	state := result["data"].(map[string]any)
	if state["smart_source_switch"] != false || state["skip_marked_ads"] != false {
		t.Fatal("v23 settings absent", state)
	}
	code, result, _ = nativeRequest(t, r, "POST", "/api/progress", Obj{"history_id": "missing", "evidence": "native_time_advanced", "position": 1, "duration": 2, "completed": false}, true)
	if code != 200 {
		t.Fatal(result)
	}
	code, result, _ = nativeRequest(t, r, "POST", "/api/playback/alternatives", Obj{"film_id": "f", "group_key": "g", "episode_key": "e"}, true)
	if code != 403 {
		t.Fatal("alternative lookup bypassed consent", result)
	}
}
