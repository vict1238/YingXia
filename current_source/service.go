// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

//go:embed web/* sources.json providers.json LICENSE.txt
var assets embed.FS

const version = "1.3.2-exe"

func intPtr(n int) *int { return &n }

type Service struct {
	dir           string
	store         *Store
	config        Config
	network       Fetcher
	catalog       *Catalog
	manager       *SourceManager
	media         *MediaRelay
	searches      *SearchJobs
	updateTrigger chan struct{}
	mu            sync.Mutex
	links         map[string]Film
	diagnostics   []Obj
}

func loadConfig(dir string) (Config, error) {
	var c Config
	b, e := os.ReadFile(filepath.Join(dir, "sources.json"))
	if os.IsNotExist(e) {
		b, e = assets.ReadFile("sources.json")
	}
	if e != nil {
		return c, e
	}
	e = json.Unmarshal([]byte(strings.TrimPrefix(string(b), "\ufeff")), &c)
	if e != nil {
		return c, problem("sources.json 格式不正确。", "config")
	}
	for _, s := range []string{c.Catalog.SuggestURL, c.Catalog.APIURL} {
		if _, e = publicURL(s); e != nil {
			return c, e
		}
	}
	templates := []string{c.Catalog.PlayerURL}
	ids := map[string]bool{}
	for _, p := range c.Parsers {
		if !re(`^[a-z0-9_]{1,30}$`).MatchString(p.ID) || ids[p.ID] {
			return c, problem("来源编号无效或重复。", "config")
		}
		ids[p.ID] = true
		templates = append(templates, p.Template)
	}
	for _, t := range templates {
		if strings.Count(t, "{url}") != 1 || strings.ContainsAny(strings.ReplaceAll(t, "{url}", ""), "{}") {
			return c, problem("播放网址模板不正确。", "config")
		}
		if _, e = publicURL(strings.ReplaceAll(t, "{url}", "https%3A%2F%2Fexample.com%2F")); e != nil {
			return c, e
		}
	}
	return c, nil
}
func newService(dir string, n Fetcher) (*Service, error) {
	store, e := newStore(dir)
	if e != nil {
		return nil, e
	}
	config, e := loadConfig(dir)
	if e != nil {
		return nil, e
	}
	if n == nil {
		n = newNetwork()
	}
	manager, e := newSourceManager(dir, config, n, store)
	if e != nil {
		return nil, e
	}
	s := &Service{dir: dir, store: store, config: config, network: n, catalog: newCatalog(config.Catalog, n), manager: manager, media: newMediaRelay(n), updateTrigger: make(chan struct{}, 1), links: map[string]Film{}, diagnostics: []Obj{}}
	s.searches = newSearchJobs(s)
	return s, nil
}
func (s *Service) requireNetwork() error {
	if !boolSetting(s.store.Settings(), "network_consent", false) {
		return problem("请先允许联网。", "consent_required", 403)
	}
	return nil
}
func (s *Service) enabledSources() []ParserConfig {
	out := append([]ParserConfig{}, s.manager.snapshot().Parsers...)
	o, _ := s.store.Settings()["enabled_sources"].(map[string]any)
	for i, p := range out {
		if v, ok := o[p.ID].(bool); ok {
			out[i].Enabled = v
		}
	}
	return out
}
func (s *Service) State() Obj {
	settings := s.store.Settings()
	ps := Obj{}
	for k, v := range platforms {
		ps[k] = Obj{"name": v.Name}
	}
	ids := []string{}
	for _, f := range s.store.Favorites() {
		ids = append(ids, sget(f, "id"))
	}
	s.mu.Lock()
	diag := cloneList(s.diagnostics)
	s.mu.Unlock()
	return Obj{"version": version, "network_consent": boolSetting(settings, "network_consent", false), "save_history": boolSetting(settings, "save_history", true), "platforms": ps, "sources": s.enabledSources(), "catalog": Obj{"name": s.config.Catalog.Name, "status": s.config.Catalog.Status}, "data_dir": s.dir, "favorite_ids": ids, "last_diagnostics": diag, "source_registry": s.manager.publicState(), "auto_next": boolSetting(settings, "auto_next", true)}
}
func (s *Service) Settings(d Obj) (Obj, error) {
	values := Obj{}
	for _, k := range []string{"network_consent", "save_history", "auto_update_sources", "auto_next"} {
		if v, ok := d[k]; ok {
			if _, ok = v.(bool); !ok {
				return nil, problem("设置值应为布尔类型。", "settings")
			}
			values[k] = v
		}
	}
	if v, ok := d["enabled_sources"]; ok {
		o, ok := v.(map[string]any)
		if !ok {
			return nil, problem("来源设置不正确。", "settings")
		}
		known := map[string]bool{}
		for _, p := range s.manager.snapshot().Parsers {
			known[p.ID] = true
		}
		for k, v := range o {
			_, isBool := v.(bool)
			if !known[k] || !isBool {
				return nil, problem("来源设置不正确。", "settings")
			}
		}
		values["enabled_sources"] = o
	}
	if v, ok := d["enabled_providers"]; ok {
		o, ok := v.(map[string]any)
		if !ok {
			return nil, problem("来源设置不正确。", "settings")
		}
		for id, value := range o {
			if _, exists := s.manager.provider(id); !exists {
				return nil, problem("来源不存在。", "settings")
			}
			if _, ok := value.(bool); !ok {
				return nil, problem("来源设置不正确。", "settings")
			}
		}
		values["enabled_providers"] = o
	}
	if e := s.store.SetMany(values); e != nil {
		return nil, e
	}
	if values["network_consent"] == false {
		s.searches.cancelActive()
	}
	if values["network_consent"] == true || values["auto_update_sources"] == true {
		select {
		case s.updateTrigger <- struct{}{}:
		default:
		}
	}
	return s.State(), nil
}
func (s *Service) Search(ctx context.Context, d Obj) (Obj, error) {
	q, ok := d["query"].(string)
	q = strings.TrimSpace(q)
	if !ok || len([]rune(q)) < 1 || len([]rune(q)) > 120 {
		return nil, problem("请输入 1–120 个字符的片名。", "query")
	}
	if e := s.requireNetwork(); e != nil {
		return nil, e
	}
	mode := defaultStr(d, "mode", "catalog")
	if mode == "all" || mode == "anime" || strings.HasPrefix(mode, "provider:") {
		return s.manager.search(ctx, q, mode)
	}
	items := []Obj{}
	if mode == "catalog" {
		fs, w, e := s.catalog.Search(ctx, q)
		if e != nil {
			return nil, e
		}
		for _, f := range fs {
			items = append(items, f.public())
		}
		return Obj{"items": items, "warnings": append(w, "目录名称相同不代表年份、季数和版本相同，请核对后选择。"), "demo": false}, nil
	}
	link, e := officialSearch(mode, q)
	if e != nil {
		return nil, e
	}
	links, _, e := locatePlatform(ctx, s.network, mode, q)
	warning := "平台公开搜索页静态检索；候选版本和集数需人工核对，不读取登录账号。"
	if e != nil || len(links) == 0 {
		warning = "未取得可确认的静态播放链接。请打开官方搜索、进入具体播放页后粘贴网址。"
		if e != nil {
			warning += " " + e.Error()
		}
	}
	for _, l := range links {
		f, fe := s.makeLinkFilm(sget(l, "url"), sget(l, "title"), mode)
		if fe == nil {
			items = append(items, f.public())
		}
	}
	return Obj{"items": items, "warnings": []string{warning}, "official_search_url": link, "demo": false}, nil
}
func (s *Service) makeLinkFilm(link, title, platform string) (Film, error) {
	sum := sha256.Sum256([]byte(link))
	key := "link-" + hex.EncodeToString(sum[:])[:20]
	title = text(title, 200)
	if title == "" {
		title = "用户提供的平台播放页"
	}
	pf := platforms[platform]
	f := Film{ID: key, Title: title, Kind: pf.Name, Source: pf.Name + "播放页", Groups: []Group{{"official", pf.Name + "页面", []Episode{{"page", "当前播放页（集数未核验）", link, nil}}}}}
	if e := s.store.Bind(key, f.Groups[0].Episodes[0].Name, platform, link); e != nil {
		return Film{}, e
	}
	s.mu.Lock()
	s.links[key] = f
	s.mu.Unlock()
	return f, nil
}
func (s *Service) Import(d Obj) (Obj, error) {
	p, u, e := platformURL(sget(d, "url"), sget(d, "platform"))
	if e != nil {
		return nil, e
	}
	f, e := s.makeLinkFilm(u, defaultStr(d, "title", "用户提供的平台播放页"), p)
	if e != nil {
		return nil, e
	}
	return f.public(), nil
}
func (s *Service) Detail(ctx context.Context, id string) (Film, error) {
	s.mu.Lock()
	f, ok := s.links[id]
	s.mu.Unlock()
	if ok {
		return f, nil
	}
	if strings.HasPrefix(id, "link-") {
		binds := s.store.Bindings(id, "当前播放页（集数未核验）")
		if len(binds) > 0 {
			title := "已保存的平台播放页"
			for _, saved := range s.store.Favorites() {
				if sget(saved, "id") == id {
					title = sget(saved, "title")
					break
				}
			}
			p, u, e := platformURL(sget(binds[0], "url"), sget(binds[0], "platform"))
			if e != nil {
				return Film{}, e
			}
			return s.makeLinkFilm(u, title, p)
		}
		return Film{}, problem("平台候选已过期，请重新搜索影片。", "not_found", 404)
	}
	if e := s.requireNetwork(); e != nil {
		return Film{}, e
	}
	if strings.HasPrefix(id, "vod:") || strings.HasPrefix(id, "rule:") {
		return s.manager.detail(ctx, id)
	}
	return s.catalog.Detail(ctx, id)
}
func (s *Service) DetailPublic(ctx context.Context, id string) (Obj, error) {
	f, e := s.Detail(ctx, id)
	if e != nil {
		return nil, e
	}
	d := f.public()
	if len(f.Groups) > 0 && len(f.Groups[0].Episodes) > 0 {
		d["bindings"] = s.store.Bindings(id, f.Groups[0].Episodes[0].Name)
	}
	return d, nil
}
func (s *Service) Bind(ctx context.Context, d Obj) (Obj, error) {
	f, e := s.Detail(ctx, sget(d, "film_id"))
	if e != nil {
		return nil, e
	}
	name := text(d["episode"], 120)
	if name == "" {
		return nil, problem("请先选择对应集数。", "episode")
	}
	p, u, e := platformURL(sget(d, "url"), sget(d, "platform"))
	if e != nil {
		return nil, e
	}
	if e = s.store.Bind(f.ID, name, p, u); e != nil {
		return nil, e
	}
	return Obj{"platform": p, "url": u, "note": "用户绑定；未自动验证同一作品/集数。"}, nil
}
func (s *Service) Select(ctx context.Context, d Obj) (Film, Group, Episode, error) {
	f, e := s.Detail(ctx, sget(d, "film_id"))
	if e != nil {
		return Film{}, Group{}, Episode{}, e
	}
	for _, g := range f.Groups {
		if g.Key == sget(d, "group_key") {
			if name, ok := d["match_name"]; ok {
				ep, e := chooseEpisode(g, str(name))
				return f, g, ep, e
			}
			for _, ep := range g.Episodes {
				if ep.Key == sget(d, "episode_key") {
					return f, g, ep, nil
				}
			}
			return f, g, Episode{}, problem("未找到这一集，请重新选择。", "episode_missing")
		}
	}
	return f, Group{}, Episode{}, problem("请先选择有效的来源组。", "bad_group")
}
func (s *Service) Resolve(ctx context.Context, d Obj) (Obj, error) {
	historyGeneration := s.store.HistoryGeneration()
	f, g, ep, e := s.Select(ctx, d)
	if e != nil {
		return nil, e
	}
	mode := defaultStr(d, "source", "catalog")
	kind, link, source := "", "", ""
	if f.Demo {
		kind, link, source = "media", ep.URL, "本机合成演示 / "+g.Name
	} else {
		if e = s.requireNetwork(); e != nil {
			return nil, e
		}
		if mode == "web" && g.Key != "official" {
			clean, e := publicURL(ep.URL)
			if e != nil {
				return nil, e
			}
			kind, link, source = "page", clean, f.Source+" / "+g.Name
		} else if (mode == "catalog" || mode == "native") && g.Key != "official" {
			clean, e := publicURL(ep.URL)
			if e != nil {
				return nil, e
			}
			if f.ProviderKind == "kazumi" {
				provider, ok := s.manager.provider(f.ProviderID)
				if !ok || provider.Rule == nil {
					return nil, problem("来源已移除。", "source_missing")
				}
				media, resolveErr := resolveRuleMedia(ctx, s.network, clean, *provider.Rule)
				if resolveErr != nil {
					if _, real := s.network.(*Network); real {
						media, _ = sniffMedia(ctx, s.dir, clean)
					}
				}
				if media != "" {
					kind, link, source = "media", media, provider.Name+" / "+g.Name
				} else {
					kind, link, source = "page", clean, provider.Name+" / "+g.Name
				}
			} else if mode == "native" {
				kind, link, source = "media", clean, "目录媒体直连 / "+g.Name
			} else {
				kind, link, source = "page", strings.ReplaceAll(s.manager.snapshot().Catalog.PlayerURL, "{url}", quote(clean)), "网页播放器 / "+g.Name
			}
		} else {
			provided := sget(d, "platform_url")
			if provided == "" {
				b := s.store.Bindings(f.ID, ep.Name)
				if len(b) > 0 {
					provided = sget(b[0], "url")
				} else if g.Key == "official" {
					provided = ep.URL
				}
			}
			if provided == "" {
				return nil, problem("此线路需要官方播放页，请先绑定这一集的平台网址。", "binding_required")
			}
			p, official, e := platformURL(provided, sget(d, "platform"))
			if e != nil {
				return nil, e
			}
			if mode == "official" || (mode == "catalog" && g.Key == "official") {
				kind, link, source = "external", official, platforms[p].Name+"官方页面"
			} else {
				var chosen *ParserConfig
				for _, entry := range s.enabledSources() {
					if entry.ID == mode && entry.Enabled {
						x := entry
						chosen = &x
						break
					}
				}
				if chosen == nil {
					return nil, problem("该解析来源未启用，请到来源设置检查。", "source_disabled")
				}
				kind, link, source = "page", strings.ReplaceAll(chosen.Template, "{url}", quote(official)), chosen.Name+"（第三方） / 定位平台："+platforms[p].Name
			}
		}
	}
	external := link
	format := ""
	if kind == "media" && strings.Contains(strings.ToLower(link), "m3u8") {
		format = "hls"
	}
	if kind == "media" && !f.Demo {
		headers := map[string]string{}
		if p, ok := s.manager.provider(f.ProviderID); ok && p.Rule != nil {
			headers = ruleHeaders(*p.Rule)
			if headers["Referer"] == "" {
				headers["Referer"] = ep.URL
			}
		}
		link, e = s.media.register(link, headers)
		if e != nil {
			return nil, e
		}
	}
	id, e := s.store.OpenedOnGeneration(historyGeneration, f.ID, f.Title, ep.Name, source, kind)
	if e != nil {
		return nil, e
	}
	return Obj{"kind": kind, "url": link, "external_url": external, "format": format, "source_label": source, "title": f.Title, "episode": ep.Name, "history_id": id, "film_id": f.ID, "group_key": g.Key, "episode_key": ep.Key, "demo": f.Demo, "playback_verified": false}, nil
}
func (s *Service) Diagnose(ctx context.Context) (Obj, error) {
	if e := s.requireNetwork(); e != nil {
		return nil, e
	}
	jobs := [][2]string{{"网络对照 python.org", "https://www.python.org/"}, {"无损云目录接口", s.config.Catalog.APIURL}, {"无损云网页播放器", strings.Split(s.config.Catalog.PlayerURL, "?")[0]}}
	for _, p := range s.manager.providers() {
		if !p.Enabled || p.Kind == "legacy" {
			continue
		}
		link := p.URL
		if p.Rule != nil {
			link = p.Rule.BaseURL
		}
		jobs = append(jobs, [2]string{p.Name, link})
	}
	for _, p := range s.enabledSources() {
		if p.Enabled {
			jobs = append(jobs, [2]string{p.Name, strings.Split(p.Template, "?")[0]})
		}
	}
	results := make([]Obj, len(jobs))
	sem := make(chan struct{}, 3)
	var wg sync.WaitGroup
	for i, j := range jobs {
		wg.Add(1)
		go func(i int, j [2]string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i] = s.network.Probe(ctx, j[0], j[1])
		}(i, j)
	}
	wg.Wait()
	s.mu.Lock()
	s.diagnostics = results
	s.mu.Unlock()
	return Obj{"results": results, "scope": "仅 HTTP / JSON 预检，不执行远程脚本，也不解码视频。", "playback_verified": false}, nil
}
func (s *Service) Report() Obj {
	s.mu.Lock()
	diag := cloneList(s.diagnostics)
	s.mu.Unlock()
	return Obj{"app": "YingXia", "version": version, "created_at": now(), "scope": "HTTP / JSON 连接检测", "diagnostics": diag, "config_provenance": s.config.Provenance, "source_registry": s.manager.publicState()}
}
