// SPDX-License-Identifier: GPL-3.0-or-later
package engine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type managementFetcher struct {
	mu      sync.Mutex
	jsonFn  func(context.Context, string, Obj) (Obj, error)
	fetchFn func(context.Context, string, int) ([]byte, Obj, error)
	calls   []string
}

func (f *managementFetcher) JSON(ctx context.Context, u string, params Obj) (Obj, error) {
	f.mu.Lock()
	f.calls = append(f.calls, u)
	fn := f.jsonFn
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx, u, params)
	}
	return Obj{"list": []any{Obj{"vod_id": "1", "vod_name": "合成目录条目"}}}, nil
}
func (f *managementFetcher) Fetch(ctx context.Context, u string, limit int) ([]byte, Obj, error) {
	if f.fetchFn != nil {
		return f.fetchFn(ctx, u, limit)
	}
	return []byte(`<html><body><div class="entry"><a href="/detail/1">合成动漫条目</a></div></body></html>`), Obj{"http": 200}, nil
}
func (f *managementFetcher) Probe(context.Context, string, string) Obj { return Obj{} }
func newManagementService(t *testing.T, f Fetcher) *Service {
	t.Helper()
	s, e := newService(t.TempDir(), f)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.store.SetMany(Obj{"network_consent": true, "auto_update_sources": false}); e != nil {
		t.Fatal(e)
	}
	return s
}
func customManagementSource(t *testing.T, s *Service, host string) Provider {
	t.Helper()
	if _, e := s.manager.addCMS(Obj{"name": host, "url": "https://" + host + "/api.php/provide/vod/"}); e != nil {
		t.Fatal(e)
	}
	for _, p := range s.manager.providers() {
		if strings.Contains(p.URL, host) {
			return p
		}
	}
	t.Fatal("custom source missing")
	return Provider{}
}
func waitSpeed(t *testing.T, s *Service, id string) Obj {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		r, e := s.sourceSpeeds.status(id)
		if e != nil {
			t.Fatal(e)
		}
		if r["state"] != "running" {
			return r
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("speed job did not finish")
	return nil
}

func TestSourceSpeedRejectsHTTP200WithoutUsableMetadata(t *testing.T) {
	for _, tc := range []struct{ name, body, status string }{
		{"valid", `{"list":[{"vod_id":1,"vod_name":"合成"}]}`, "available"},
		{"maintenance", `<html><title>just a moment</title></html>`, "unavailable"},
		{"wrong_schema", `{"code":200,"message":"success"}`, "unavailable"},
		{"empty_list", `{"list":[]}`, "unavailable"},
		{"invalid_rows", `{"list":[{"vod_id":"not-a-number","vod_name":"假条目"}]}`, "unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &managementFetcher{jsonFn: func(ctx context.Context, u string, params Obj) (Obj, error) {
				if params["ac"] != "detail" || params["pg"] != 1 || params["wd"] != nil {
					t.Errorf("not a real bounded metadata request: %#v", params)
				}
				return decodeObject([]byte(tc.body))
			}}
			s := newManagementService(t, f)
			p := customManagementSource(t, s, "fixture.example")
			h := s.manager.measureSource(context.Background(), p, s.manager.snapshot())
			if h.Status != tc.status || h.PlaybackVerified || h.CheckedAt == "" {
				t.Fatalf("unexpected health %#v", h)
			}
		})
	}
}
func TestSourceSpeedUsesKazumiSearchRule(t *testing.T) {
	f := &managementFetcher{}
	s := newManagementService(t, f)
	rule := KazumiRule{API: "4", Name: "fixture", Version: "1", BaseURL: "https://anime.example/", SearchURL: "https://anime.example/search?wd=@keyword", SearchList: "//div[@class='entry']", SearchName: "//a", SearchResult: "//a", ChapterRoads: "//div", ChapterResult: "//a"}
	p := Provider{ID: "fixture_anime", Name: "合成动漫", Kind: "kazumi", Origin: "kazumi", Enabled: true, Rule: &rule}
	if e := validateProvider(p); e != nil {
		t.Fatal(e)
	}
	h := s.manager.measureSource(context.Background(), p, s.manager.snapshot())
	if h.Status != "available" || h.CatalogCount != 1 || h.PlaybackVerified {
		t.Fatalf("rule not executed: %#v", h)
	}
	f.fetchFn = func(context.Context, string, int) ([]byte, Obj, error) {
		return []byte(`<html>请完成验证</html>`), Obj{"http": 200}, nil
	}
	h = s.manager.measureSource(context.Background(), p, s.manager.snapshot())
	if h.Status != "unavailable" {
		t.Fatal("challenge page counted as valid static rule")
	}
}
func TestKazumiAPINineSourceTemplateAndSafety(t *testing.T) {
	rule := KazumiRule{API: "9", Name: "fixture", Version: "2.5", BaseURL: "https://anime.example/", SearchMode: "api", ChapterMode: "xpath", ChapterRoads: "//ul", ChapterResult: "//a", SearchAPI: RuleSearchAPI{Request: RuleRequest{URL: "https://anime.example/suggest", Method: "GET"}, ListPath: "$.list[*]", NamePath: "$.name", SourcePath: "$.id", SourceTemplate: "https://anime.example/GV@source/"}}
	if e := validateRule(rule); e != nil {
		t.Fatal(e)
	}
	f := &managementFetcher{fetchFn: func(context.Context, string, int) ([]byte, Obj, error) {
		return []byte(`{"list":[{"id":42,"name":"合成"},{"id":42,"name":"重复"}]}`), Obj{}, nil
	}}
	p := Provider{ID: "fixture_nine", Name: "fixture", Kind: "kazumi", Rule: &rule}
	fs, e := searchRule(context.Background(), f, p, "test")
	if e != nil || len(fs) != 1 {
		t.Fatal("API9 mapping/dedup failed", fs, e)
	}
	_, _, ref, e := splitSourceFilmID(fs[0].ID)
	if e != nil || ref != "https://anime.example/GV42/" {
		t.Fatal("numeric ID did not map to detail URL", ref, e)
	}
	for _, tmpl := range []string{"https://@source/path", "https://@source.example/path", "http://127.0.0.1/@source", "file:///tmp/@source", "https://a.example/path", "https://a.example/@source/@source", "https://user:pass@a.example/@source", "https://a.example/@source#x"} {
		if e = validateRuleSourceTemplate(tmpl); e == nil {
			t.Fatal("unsafe template accepted", tmpl)
		}
	}
	rule.API = "10"
	if e = validateRule(rule); errCodeValue(e) != "rule_api" {
		t.Fatal("future API unexpectedly accepted", e)
	}
	// The 4/8 static format remains valid without a source template.
	rule.API = "8"
	rule.SearchAPI.SourceTemplate = ""
	if e = validateRule(rule); e != nil {
		t.Fatal("API8 regressed", e)
	}
}
func TestSourceSpeedAsyncProgressPersistenceAndOrdering(t *testing.T) {
	release := make(chan struct{})
	f := &managementFetcher{jsonFn: func(ctx context.Context, u string, params Obj) (Obj, error) {
		if strings.Contains(u, "slow.example") {
			select {
			case <-release:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return Obj{"list": []any{Obj{"vod_id": "1", "vod_name": "合成"}}}, nil
	}}
	s := newManagementService(t, f)
	fast := customManagementSource(t, s, "fast.example")
	slow := customManagementSource(t, s, "slow.example")
	if _, e := s.Settings(Obj{"enabled_providers": Obj{slow.ID: false}}); e != nil {
		t.Fatal(e)
	}
	job, e := s.sourceSpeeds.start(Obj{"ids": []any{fast.ID, slow.ID}})
	if e != nil {
		t.Fatal(e)
	}
	id := sget(job, "id")
	deadline := time.Now().Add(2 * time.Second)
	seenProgress := false
	for time.Now().Before(deadline) {
		r, _ := s.sourceSpeeds.status(id)
		if r["completed"] == 1 && r["total"] == 2 && r["progress"] == 50 && r["state"] == "running" {
			seenProgress = true
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	close(release)
	if !seenProgress {
		t.Fatal("in-flight progress was not observable")
	}
	result := waitSpeed(t, s, id)
	if result["state"] != "done" || result["completed"] != 2 || result["progress"] != 100 {
		t.Fatalf("bad completion %#v", result)
	}
	r := s.manager.snapshot()
	if r.Health[fast.ID].Status != "available" || r.Health[slow.ID].Status != "available" {
		t.Fatal("metrics not saved")
	}
	p, _ := s.manager.provider(slow.ID)
	if p.Enabled {
		t.Fatal("speed test enabled disabled provider")
	}
	s2, e := newService(s.dir, f)
	if e != nil {
		t.Fatal(e)
	}
	if s2.manager.snapshot().Health[fast.ID].CheckedAt == "" {
		t.Fatal("health not persisted across restart")
	}
	ps := s2.manager.providers()
	if len(ps) < 2 || ps[0].ID != fast.ID {
		t.Fatal("fast healthy source was not preferred")
	}
}
func TestSourceSpeedCancelAndConsent(t *testing.T) {
	started := make(chan struct{}, 1)
	f := &managementFetcher{jsonFn: func(ctx context.Context, u string, params Obj) (Obj, error) {
		started <- struct{}{}
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	s := newManagementService(t, f)
	p := customManagementSource(t, s, "cancel.example")
	j, e := s.sourceSpeeds.start(Obj{"ids": []any{p.ID}})
	if e != nil {
		t.Fatal(e)
	}
	<-started
	if _, e = s.Settings(Obj{"network_consent": false}); e != nil {
		t.Fatal(e)
	}
	r := waitSpeed(t, s, sget(j, "id"))
	if r["state"] != "cancelled" {
		t.Fatal(r)
	}
	if _, e = s.sourceSpeeds.start(Obj{"ids": []any{p.ID}}); errCodeValue(e) != "consent_required" {
		t.Fatal("offline job accepted", e)
	}
	if _, ok := s.manager.snapshot().Health[p.ID]; ok {
		t.Fatal("cancelled job wrote health")
	}
}
func TestSourceManagementEditsSwitchesAndUpdatePreserveState(t *testing.T) {
	f := &managementFetcher{}
	s := newManagementService(t, f)
	a := customManagementSource(t, s, "a.example")
	b := customManagementSource(t, s, "b.example")
	if _, e := s.Settings(Obj{"enabled_providers": Obj{a.ID: false}}); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Settings(Obj{"enabled_providers": Obj{b.ID: false}}); e != nil {
		t.Fatal(e)
	}
	for _, id := range []string{a.ID, b.ID} {
		p, _ := s.manager.provider(id)
		if p.Enabled {
			t.Fatal("single toggle reset another switch")
		}
	}
	job, e := s.sourceSpeeds.start(Obj{"ids": []any{a.ID}})
	if e != nil {
		t.Fatal(e)
	}
	if r := waitSpeed(t, s, sget(job, "id")); r["state"] != "done" {
		t.Fatal(r)
	}
	if _, e = s.manager.editCMS(Obj{"id": a.ID, "name": "名称已改", "url": ""}); e != nil {
		t.Fatal(e)
	}
	p, _ := s.manager.provider(a.ID)
	if p.Name != "名称已改" || p.URL != a.URL || p.Enabled {
		t.Fatal("edit failed to preserve URL/disabled choice")
	}
	if s.manager.snapshot().Health[a.ID].Status != "available" {
		t.Fatal("name-only edit lost metrics")
	}
	if _, e = s.manager.editCMS(Obj{"id": a.ID, "name": "换地址", "url": "https://new.example/vod/"}); e != nil {
		t.Fatal(e)
	}
	if _, ok := s.manager.snapshot().Health[a.ID]; ok {
		t.Fatal("URL edit retained old latency")
	}
	if _, e = s.manager.editCMS(Obj{"id": "wsyzy", "name": "内置", "url": "https://new.example/"}); e == nil {
		t.Fatal("built-in edit accepted")
	}
	for _, u := range []string{"http://127.0.0.1/", "http://10.1.2.3/", "https://user:pass@example.com/", "file:///tmp/a"} {
		if _, e = s.manager.addCMS(Obj{"name": "bad", "url": u}); e == nil {
			t.Fatal("unsafe URL accepted", u)
		}
	}
	// A real update pass with unchanged valid upstream commits must not replace
	// per-user switches, custom endpoint edits or independent health data.
	r := s.manager.snapshot()
	sha := strings.Repeat("a", 40)
	for _, key := range []string{"88lin", "kazumi_rules", "kazumi"} {
		v := r.Upstreams[key]
		v.Commit = sha
		r.Upstreams[key] = v
	}
	h := SourceHealth{Status: "available", CheckedAt: now(), Message: "目录读取正常", Fingerprint: sourceHealthFingerprint(b, r)}
	if r.Health == nil {
		r.Health = map[string]SourceHealth{}
	}
	r.Health[b.ID] = h
	if e = s.manager.save(r); e != nil {
		t.Fatal(e)
	}
	f.jsonFn = func(context.Context, string, Obj) (Obj, error) { return Obj{"sha": sha}, nil }
	if _, e = s.manager.update(context.Background()); e != nil {
		t.Fatal(e)
	}
	if s.manager.snapshot().Health[b.ID] != h {
		t.Fatal("upstream update replaced measurement")
	}
	for _, id := range []string{a.ID, b.ID} {
		p, ok := s.manager.provider(id)
		if !ok || p.Enabled {
			t.Fatal("update replaced user state")
		}
	}
	if _, e = s.manager.removeCMS(a.ID); e != nil {
		t.Fatal(e)
	}
	if _, ok := s.manager.provider(a.ID); ok {
		t.Fatal("deleted custom source remains")
	}
}
func TestSourceSpeedPrivateAPIAndNetworkBoundary(t *testing.T) {
	f := &managementFetcher{}
	r, e := newRuntime(t.TempDir(), f)
	if e != nil {
		t.Fatal(e)
	}
	defer r.Stop()
	s := r.server.service
	if e = s.store.SetMany(Obj{"network_consent": true, "auto_update_sources": false}); e != nil {
		t.Fatal(e)
	}
	p := customManagementSource(t, s, "api.example")
	status, _, _ := nativeRequest(t, r, "POST", "/api/sources/speed/start", Obj{"ids": []any{p.ID}}, false)
	if status != 403 {
		t.Fatal("anonymous speed start accepted")
	}
	status, body, _ := nativeRequest(t, r, "POST", "/api/sources/speed/start", Obj{"ids": []any{p.ID}}, true)
	if status != 200 {
		t.Fatal(body)
	}
	id := sget(body["data"].(map[string]any), "id")
	waitSpeed(t, s, id)
	status, body, _ = nativeRequest(t, r, "GET", "/api/sources/speed/status?id="+id, nil, true)
	if status != 200 || body["data"].(map[string]any)["state"] != "done" {
		t.Fatal(body)
	}
	status, _, _ = nativeRequest(t, r, "GET", "/api/sources/speed/status?id="+id, nil, false)
	if status != 403 {
		t.Fatal("anonymous status accepted")
	}
	for _, u := range []string{"http://127.0.0.1:1/", "http://169.254.169.254/latest/meta-data/", "https://[::1]/"} {
		_, _, err := newNetwork().Fetch(context.Background(), u, 1000)
		if errCodeValue(err) != "unsafe_url" {
			t.Fatal("production network allowed private target", u, err)
		}
	}
}
func TestBundledSourcesMergePreservesExistingRegistry(t *testing.T) {
	f := &managementFetcher{}
	s := newManagementService(t, f)
	p := customManagementSource(t, s, "retained.example")
	if _, e := s.Settings(Obj{"enabled_providers": Obj{p.ID: false, "wsyzy": false}}); e != nil {
		t.Fatal(e)
	}
	r := s.manager.snapshot()
	r.SeedRevision = "older"
	for i := range r.Providers {
		if r.Providers[i].ID == "kazumi_girigirilove" {
			r.Providers[i].Rule.Version = "2.4"
			r.Providers[i].Rule.API = "8"
			r.Providers[i].Rule.SearchAPI.SourceTemplate = ""
			r.Providers[i].Enabled = false
		}
		if r.Providers[i].ID == "kazumi_7sefun" {
			r.Providers[i].Rule.Version = "99.0"
		}
	}
	if r.Health == nil {
		r.Health = map[string]SourceHealth{}
	}
	r.Health[p.ID] = SourceHealth{Status: "available", CheckedAt: now(), Fingerprint: sourceHealthFingerprint(p, r)}
	before := r.Health[p.ID]
	if e := s.manager.save(r); e != nil {
		t.Fatal(e)
	}
	s2, e := newService(s.dir, f)
	if e != nil {
		t.Fatal(e)
	}
	got := s2.manager.snapshot()
	if got.SeedRevision == "older" || got.Health[p.ID] != before {
		t.Fatal("bundle migration replaced user health")
	}
	for _, id := range []string{p.ID, "wsyzy"} {
		p, ok := s2.manager.provider(id)
		if !ok || p.Enabled {
			t.Fatal("bundle migration reset switches")
		}
	}
	giri, _ := s2.manager.provider("kazumi_girigirilove")
	if giri.Rule.Version != "2.5" || giri.Rule.API != "9" || giri.Enabled {
		t.Fatal("bundled rule upgrade failed or changed disabled switch")
	}
	future, _ := s2.manager.provider("kazumi_7sefun")
	if future.Rule.Version != "99.0" {
		t.Fatal("bundled migration downgraded a newer saved rule")
	}
	data, e := os.ReadFile(filepath.Join(s.dir, "source_registry.json"))
	if e != nil {
		t.Fatal(e)
	}
	var persisted SourceRegistry
	if json.Unmarshal(data, &persisted) != nil || persisted.SeedRevision != got.SeedRevision {
		t.Fatal("bundle migration was not persisted")
	}
}
func TestKazumiAPI9UpstreamUpdatePreservesCustomAndHealth(t *testing.T) {
	f := &managementFetcher{}
	s := newManagementService(t, f)
	p := customManagementSource(t, s, "upstream-retained.example")
	r := s.manager.snapshot()
	giri, _ := s.manager.provider("kazumi_girigirilove")
	if _, e := s.Settings(Obj{"enabled_providers": Obj{giri.ID: false, p.ID: false}}); e != nil {
		t.Fatal(e)
	}
	r.Health = map[string]SourceHealth{p.ID: {Status: "available", CheckedAt: now(), Fingerprint: sourceHealthFingerprint(p, r)}}
	before := r.Health[p.ID]
	if e := s.manager.save(r); e != nil {
		t.Fatal(e)
	}
	sha := strings.Repeat("b", 40)
	f.jsonFn = func(ctx context.Context, u string, params Obj) (Obj, error) {
		if strings.Contains(u, "KazumiRules/") {
			return Obj{"sha": sha}, nil
		}
		if strings.Contains(u, "88lin/") {
			return Obj{"sha": r.Upstreams["88lin"].Commit}, nil
		}
		return Obj{"sha": r.Upstreams["kazumi"].Commit}, nil
	}
	f.fetchFn = func(ctx context.Context, u string, limit int) ([]byte, Obj, error) {
		if strings.HasSuffix(u, "index.json") {
			b, _ := json.Marshal([]Obj{{"name": giri.Name, "version": giri.Rule.Version}})
			return b, Obj{}, nil
		}
		b, _ := json.Marshal(giri.Rule)
		return b, Obj{}, nil
	}
	if _, e := s.manager.update(context.Background()); e != nil {
		t.Fatal(e)
	}
	got := s.manager.snapshot()
	if got.Upstreams["kazumi_rules"].Commit != sha || got.Health[p.ID] != before {
		t.Fatal("actual rule replacement failed or destroyed health")
	}
	for _, id := range []string{giri.ID, p.ID} {
		p, ok := s.manager.provider(id)
		if !ok || p.Enabled {
			t.Fatal("actual update lost custom/disable choices")
		}
	}
	current, _ := s.manager.provider(giri.ID)
	if current.Rule.SearchAPI.SourceTemplate == "" {
		t.Fatal("API9 sourceTemplate lost in update")
	}
}
