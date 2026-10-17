// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Provider struct {
	ID      string      `json:"id"`
	Name    string      `json:"name"`
	Kind    string      `json:"kind"`
	URL     string      `json:"url,omitempty"`
	Origin  string      `json:"origin"`
	Enabled bool        `json:"enabled"`
	Rule    *KazumiRule `json:"rule,omitempty"`
}
type UpstreamState struct {
	Repository     string `json:"repository"`
	Commit         string `json:"commit"`
	Version        string `json:"version"`
	CheckedAt      string `json:"checked_at,omitempty"`
	AppliedAt      string `json:"applied_at,omitempty"`
	Status         string `json:"status"`
	Error          string `json:"error,omitempty"`
	NeedsAppUpdate bool   `json:"needs_app_update"`
}
type SourceRegistry struct {
	Schema        int                      `json:"schema"`
	Providers     []Provider               `json:"providers"`
	Catalog       CatalogConfig            `json:"catalog"`
	Parsers       []ParserConfig           `json:"parsers"`
	Upstreams     map[string]UpstreamState `json:"upstreams"`
	LastCheckedAt string                   `json:"last_checked_at,omitempty"`
	LastUpdatedAt string                   `json:"last_updated_at,omitempty"`
	Changes       []string                 `json:"changes"`
}
type SourceManager struct {
	mu       sync.RWMutex
	updateMu sync.Mutex
	state    SourceRegistry
	path     string
	net      Fetcher
	store    *Store
	cache    map[string]filmCache
}

func newSourceManager(dir string, c Config, n Fetcher, store *Store) (*SourceManager, error) {
	m := &SourceManager{path: filepath.Join(dir, "source_registry.json"), net: n, store: store, cache: map[string]filmCache{}}
	b, e := assets.ReadFile("providers.json")
	if e != nil {
		return nil, e
	}
	if e = json.Unmarshal(b, &m.state); e != nil {
		return nil, e
	}
	m.state.Catalog = c.Catalog
	m.state.Parsers = c.Parsers
	if b, e = os.ReadFile(m.path); e == nil {
		var saved SourceRegistry
		if json.Unmarshal(b, &saved) == nil && validateRegistry(saved) == nil {
			m.state = saved
		} else {
			if previous, err := os.ReadFile(m.path + ".previous"); err == nil && json.Unmarshal(previous, &saved) == nil && validateRegistry(saved) == nil {
				m.state = saved
			}
			m.state.Changes = []string{"来源文件损坏，已恢复可用配置"}
		}
	} else if !os.IsNotExist(e) {
		return nil, e
	}
	if e = validateRegistry(m.state); e != nil {
		return nil, e
	}
	return m, nil
}
func cloneRegistry(r SourceRegistry) SourceRegistry {
	b, _ := json.Marshal(r)
	var v SourceRegistry
	_ = json.Unmarshal(b, &v)
	return v
}
func (m *SourceManager) snapshot() SourceRegistry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return cloneRegistry(m.state)
}
func validateRegistry(r SourceRegistry) error {
	if r.Schema != 1 || len(r.Providers) > 100 || len(r.Providers) == 0 {
		return problem("来源配置格式不支持。", "source_config")
	}
	seen := map[string]bool{}
	for _, p := range r.Providers {
		if seen[p.ID] {
			return problem("来源编号重复。", "source_config")
		}
		seen[p.ID] = true
		if e := validateProvider(p); e != nil {
			return e
		}
	}
	if _, e := publicURL(r.Catalog.APIURL); e != nil {
		return e
	}
	if _, e := publicURL(r.Catalog.SuggestURL); e != nil {
		return e
	}
	if e := validateTemplate(r.Catalog.PlayerURL); e != nil {
		return e
	}
	seen = map[string]bool{}
	for _, p := range r.Parsers {
		if !providerIDRE.MatchString(p.ID) || seen[p.ID] {
			return problem("解析线路编号无效。", "source_config")
		}
		seen[p.ID] = true
		if e := validateTemplate(p.Template); e != nil {
			return e
		}
	}
	return nil
}

var providerIDRE = re(`^[a-z0-9_]{1,50}$`)

func validateTemplate(t string) error {
	if strings.Count(t, "{url}") != 1 {
		return problem("解析地址模板无效。", "source_config")
	}
	_, e := publicURL(strings.ReplaceAll(t, "{url}", "https%3A%2F%2Fexample.com%2F"))
	return e
}
func validateProvider(p Provider) error {
	if !providerIDRE.MatchString(p.ID) || p.Name == "" || len(p.Name) > 200 {
		return problem("来源编号或名称无效。", "source_config")
	}
	switch p.Kind {
	case "legacy":
		if p.ID != "wsyzy" {
			return problem("目录配置无效。", "source_config")
		}
	case "cms":
		_, e := publicURL(p.URL)
		return e
	case "kazumi":
		if p.Rule == nil {
			return problem("缺少动漫规则。", "rule")
		}
		return validateRule(*p.Rule)
	default:
		return problem("不支持此来源类型。", "source_config")
	}
	return nil
}
func (m *SourceManager) save(next SourceRegistry) error {
	if e := validateRegistry(next); e != nil {
		return e
	}
	b, e := json.MarshalIndent(next, "", "  ")
	if e != nil {
		return e
	}
	if old, e := os.ReadFile(m.path); e == nil {
		var previous SourceRegistry
		if json.Unmarshal(old, &previous) == nil && validateRegistry(previous) == nil {
			if e = atomicBytes(m.path+".previous", old); e != nil {
				return e
			}
		}
	}
	if e = atomicBytes(m.path, b); e != nil {
		return e
	}
	m.mu.Lock()
	m.state = cloneRegistry(next)
	m.cache = map[string]filmCache{}
	m.mu.Unlock()
	return nil
}
func atomicBytes(path string, b []byte) error {
	f, e := os.CreateTemp(filepath.Dir(path), ".sources-*.tmp")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	return replaceFile(name, path)
}
func (m *SourceManager) providers() []Provider {
	ps := m.snapshot().Providers
	o, _ := m.store.Settings()["enabled_providers"].(map[string]any)
	for i, p := range ps {
		if v, ok := o[p.ID].(bool); ok {
			ps[i].Enabled = v
		}
	}
	return ps
}
func (m *SourceManager) provider(id string) (Provider, bool) {
	for _, p := range m.providers() {
		if p.ID == id {
			return p, true
		}
	}
	return Provider{}, false
}
func (m *SourceManager) publicState() Obj {
	r := m.snapshot()
	ps := []Obj{}
	for _, p := range m.providers() {
		o := Obj{"id": p.ID, "name": p.Name, "kind": p.Kind, "enabled": p.Enabled, "origin": p.Origin}
		if p.Kind == "cms" {
			o["url"] = redactedURL(p.URL)
		}
		if p.Rule != nil {
			o["rule_version"] = p.Rule.Version
		}
		ps = append(ps, o)
	}
	return Obj{"providers": ps, "upstreams": r.Upstreams, "last_checked_at": r.LastCheckedAt, "last_updated_at": r.LastUpdatedAt, "changes": r.Changes, "auto_update": boolSetting(m.store.Settings(), "auto_update_sources", true)}
}
func sourceFilmID(kind, id, ref string) string {
	return kind + ":" + id + ":" + base64.RawURLEncoding.EncodeToString([]byte(ref))
}
func splitSourceFilmID(id string) (string, string, string, error) {
	p := strings.SplitN(id, ":", 3)
	if len(p) != 3 || len(p[2]) > 6000 {
		return "", "", "", problem("影片编号无效。", "bad_id")
	}
	b, e := base64.RawURLEncoding.DecodeString(p[2])
	if e != nil || len(b) > 4096 || len(b) == 0 {
		return "", "", "", problem("影片编号无效。", "bad_id")
	}
	return p[0], p[1], string(b), nil
}
func providerFingerprint(p Provider) string {
	b, _ := json.Marshal(p)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:8])
}
func (m *SourceManager) search(ctx context.Context, q, mode string) (Obj, error) {
	return m.searchWithProgress(ctx, q, mode, nil)
}
func (m *SourceManager) searchWithProgress(ctx context.Context, q, mode string, progress func(int, int)) (Obj, error) {
	ctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	ps := []Provider{}
	for _, p := range m.providers() {
		if p.Enabled && (mode == "all" || (mode == "anime" && p.Kind == "kazumi") || mode == "provider:"+p.ID) {
			ps = append(ps, p)
		}
	}
	if len(ps) == 0 {
		return nil, problem("请启用至少一个来源。", "sources_disabled")
	}
	type outcome struct {
		films  []Film
		status Obj
	}
	all := make([]outcome, len(ps))
	sem := make(chan struct{}, 6)
	var wg sync.WaitGroup
	var progressMu sync.Mutex
	completed := 0
	if progress != nil {
		progress(0, len(ps))
	}
	for i, p := range ps {
		wg.Add(1)
		go func(i int, p Provider) {
			defer wg.Done()
			defer func() {
				progressMu.Lock()
				defer progressMu.Unlock()
				completed++
				if progress != nil {
					progress(completed, len(ps))
				}
			}()
			start := time.Now()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				all[i].status = Obj{"id": p.ID, "name": p.Name, "ok": false, "count": 0, "error": "搜索超时"}
				return
			}
			defer func() { <-sem }()
			fs, e := m.searchProvider(ctx, p, q)
			st := Obj{"id": p.ID, "name": p.Name, "ok": e == nil, "count": len(fs), "elapsed_ms": time.Since(start).Milliseconds()}
			if e != nil {
				st["error"] = e.Error()
			}
			all[i] = outcome{fs, st}
		}(i, p)
	}
	wg.Wait()
	films := []Film{}
	statuses := []Obj{}
	success := 0
	for _, o := range all {
		films = append(films, o.films...)
		statuses = append(statuses, o.status)
		if o.status["ok"] == true {
			success++
		}
	}
	sort.SliceStable(films, func(i, j int) bool {
		a, b := ranking(q, films[i].Title, films[i].Year), ranking(q, films[j].Title, films[j].Year)
		if a != b {
			return a > b
		}
		return films[i].Source < films[j].Source
	})
	if len(films) > 400 {
		films = films[:400]
	}
	items := []Obj{}
	for _, f := range films {
		items = append(items, f.public())
	}
	return Obj{"items": items, "warnings": []string{}, "source_results": statuses, "successful_sources": success, "source_count": len(ps), "demo": false}, nil
}
func (m *SourceManager) searchProvider(ctx context.Context, p Provider, q string) ([]Film, error) {
	if !boolSetting(m.store.Settings(), "network_consent", false) {
		return nil, problem("联网已关闭。", "consent_required", 403)
	}
	var films []Film
	var e error
	switch p.Kind {
	case "legacy":
		r := m.snapshot()
		films, _, e = newCatalog(r.Catalog, m.net).Search(ctx, q)
	case "cms":
		var d Obj
		d, e = m.net.JSON(ctx, p.URL, Obj{"ac": "detail", "wd": q, "pg": 1})
		if e == nil {
			var rs []Obj
			rs, e = rows(d)
			for i, r := range rs {
				if i >= 60 {
					break
				}
				f, fe := filmFromVOD(r)
				if fe == nil {
					films = append(films, f)
				}
			}
		}
	case "kazumi":
		films, e = searchRule(ctx, m.net, p, q)
	}
	if e != nil {
		return nil, e
	}
	for i := range films {
		f := &films[i]
		if p.Kind != "kazumi" {
			f.ID = sourceFilmID("vod", p.ID, f.ID)
		}
		f.ProviderID = p.ID
		f.ProviderKind = p.Kind
		f.Source = p.Name
		m.mu.Lock()
		if len(m.cache) > 3000 {
			m.cache = map[string]filmCache{}
		}
		m.cache[f.ID] = filmCache{time.Now(), *f}
		m.mu.Unlock()
	}
	return films, nil
}
func (m *SourceManager) detail(ctx context.Context, id string) (Film, error) {
	kind, pid, ref, e := splitSourceFilmID(id)
	if e != nil {
		return Film{}, e
	}
	p, ok := m.provider(pid)
	if !ok {
		return Film{}, problem("该来源已移除，请重新搜索。", "source_missing")
	}
	if !p.Enabled {
		return Film{}, problem("此来源已关闭。", "sources_disabled")
	}
	m.mu.RLock()
	cached, ok := m.cache[id]
	m.mu.RUnlock()
	if ok && len(cached.Film.Groups) > 0 && time.Since(cached.When) < 3*time.Minute {
		return cached.Film, nil
	}
	var f Film
	if kind == "rule" && p.Kind == "kazumi" {
		f, e = detailRule(ctx, m.net, p, ref)
	} else if kind == "vod" && p.Kind == "legacy" {
		f, e = newCatalog(m.snapshot().Catalog, m.net).Detail(ctx, ref)
	} else if kind == "vod" && p.Kind == "cms" {
		var d Obj
		d, e = m.net.JSON(ctx, p.URL, Obj{"ac": "detail", "ids": ref})
		if e == nil {
			var rs []Obj
			rs, e = rows(d)
			found := false
			for _, r := range rs {
				x, fe := filmFromVOD(r)
				if fe == nil && x.ID == ref {
					f = x
					found = true
					break
				}
			}
			if !found && e == nil {
				e = problem("未找到所选影片。", "not_found", 404)
			}
		}
	} else {
		e = problem("来源类型不匹配。", "bad_id")
	}
	if e != nil {
		return Film{}, e
	}
	f.ID = id
	f.ProviderID = p.ID
	f.ProviderKind = p.Kind
	f.Source = p.Name
	if ok && p.Kind == "kazumi" && cached.Film.Title != "" {
		f.Title = cached.Film.Title
	}
	if f.Title == "" {
		if ok {
			f.Title = cached.Film.Title
		} else {
			for _, saved := range m.store.Favorites() {
				if sget(saved, "id") == id {
					f.Title = sget(saved, "title")
					break
				}
			}
		}
		if f.Title == "" {
			f.Title = p.Name + "影片"
		}
	}
	m.mu.Lock()
	m.cache[id] = filmCache{time.Now(), f}
	m.mu.Unlock()
	return f, nil
}
func (m *SourceManager) addCMS(d Obj) (Obj, error) {
	m.updateMu.Lock()
	defer m.updateMu.Unlock()
	entries := []Obj{d}
	if raw, ok := d["items"]; ok {
		a, ok := raw.([]any)
		if !ok || len(a) == 0 || len(a) > 50 {
			return nil, problem("每次可导入 1–50 个来源。", "source_config")
		}
		entries = []Obj{}
		for _, v := range a {
			o, ok := v.(map[string]any)
			if !ok {
				return nil, problem("来源 JSON 格式不正确。", "source_config")
			}
			entries = append(entries, o)
		}
	}
	next := m.snapshot()
	for _, entry := range entries {
		u, e := publicURL(sget(entry, "url"))
		if e != nil {
			return nil, e
		}
		name := text(entry["name"], 80)
		if name == "" {
			return nil, problem("请输入来源名称。", "source_config")
		}
		sum := sha256.Sum256([]byte(strings.TrimRight(u, "/")))
		id := fmt.Sprintf("custom_%x", sum[:8])
		p := Provider{ID: id, Name: name, Kind: "cms", URL: u, Enabled: true, Origin: "custom"}
		found := false
		for i, x := range next.Providers {
			if x.ID == id {
				next.Providers[i] = p
				found = true
			}
		}
		if !found {
			next.Providers = append(next.Providers, p)
		}
	}
	if e := m.save(next); e != nil {
		return nil, e
	}
	return m.publicState(), nil
}
func (m *SourceManager) removeCMS(id string) (Obj, error) {
	m.updateMu.Lock()
	defer m.updateMu.Unlock()
	next := m.snapshot()
	out := []Provider{}
	found := false
	for _, p := range next.Providers {
		if p.ID == id {
			if p.Origin != "custom" {
				return nil, problem("内置来源请使用开关。", "source_config")
			}
			found = true
		} else {
			out = append(out, p)
		}
	}
	if !found {
		return nil, problem("来源不存在。", "not_found", 404)
	}
	next.Providers = out
	if e := m.save(next); e != nil {
		return nil, e
	}
	return m.publicState(), nil
}
