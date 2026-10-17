// SPDX-License-Identifier: GPL-3.0-or-later
package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Health measures the metadata operation actually used by this client. It is
// deliberately separate from per-film media availability and decode evidence.
type SourceHealth struct {
	Status           string `json:"status"`
	LatencyMS        int64  `json:"latency_ms"`
	CheckedAt        string `json:"checked_at"`
	Message          string `json:"message"`
	CatalogCount     int    `json:"catalog_count"`
	PlaybackVerified bool   `json:"playback_verified"`
	Fingerprint      string `json:"fingerprint,omitempty"`
}

type bundledSourceList struct {
	Schema    int        `json:"schema"`
	Revision  string     `json:"revision"`
	Providers []Provider `json:"providers"`
}

func providerEndpoint(p Provider, r SourceRegistry) string {
	if p.Kind == "legacy" {
		return r.Catalog.APIURL
	}
	if p.Rule != nil {
		return p.Rule.BaseURL
	}
	return p.URL
}
func sourceHealthFingerprint(p Provider, r SourceRegistry) string {
	v := Obj{"kind": p.Kind, "url": p.URL, "rule": p.Rule}
	if p.Kind == "legacy" {
		v["catalog"] = r.Catalog
	}
	b, _ := json.Marshal(v)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:16])
}
func healthPriority(status string) int {
	if status == "available" {
		return 0
	}
	if status == "unavailable" {
		return 2
	}
	return 1
}
func (m *SourceManager) healthFor(p Provider, r SourceRegistry) SourceHealth {
	h := r.Health[p.ID]
	if h.Status == "" || h.Fingerprint != sourceHealthFingerprint(p, r) {
		return SourceHealth{Status: "unknown", Message: "未测速"}
	}
	if checked, e := time.Parse(time.RFC3339, h.CheckedAt); e != nil || time.Since(checked) > 24*time.Hour {
		h.Status, h.Message = "unknown", "测速结果已过期"
	}
	return h
}

// Add a bundled revision to an existing user's registry without replacing
// upstream commits, custom sources, enable overrides or prior measurements.
func (m *SourceManager) mergeBundledSources() error {
	b, e := assets.ReadFile("reviewed_sources.json")
	if e != nil {
		return e
	}
	var list bundledSourceList
	if json.Unmarshal(b, &list) != nil || list.Schema != 1 || list.Revision == "" {
		return problem("内置来源清单格式不正确。", "source_config")
	}
	next := m.snapshot()
	if next.SeedRevision == list.Revision {
		return nil
	}
	// Refresh only older known bundled rule versions during this app migration.
	// Never roll back a newer rule that was already downloaded by the user.
	var defaults SourceRegistry
	defaultBytes, e := assets.ReadFile("providers.json")
	if e != nil || json.Unmarshal(defaultBytes, &defaults) != nil {
		return problem("内置动漫来源格式不正确。", "source_config")
	}
	for i, old := range next.Providers {
		if old.Origin != "kazumi" || old.Rule == nil {
			continue
		}
		for _, bundled := range defaults.Providers {
			if bundled.ID == old.ID && bundled.Origin == "kazumi" && bundled.Rule != nil && ruleVersionNewer(bundled.Rule.Version, old.Rule.Version) {
				bundled.Enabled = old.Enabled
				next.Providers[i] = bundled
			}
		}
	}
	for _, p := range list.Providers {
		if p.Origin == "custom" || p.Kind != "cms" {
			return problem("内置来源清单含不支持的条目。", "source_config")
		}
		if e = validateProvider(p); e != nil {
			return e
		}
		found := false
		for _, old := range next.Providers {
			if old.ID == p.ID || (old.Kind == "cms" && strings.TrimRight(old.URL, "/") == strings.TrimRight(p.URL, "/")) {
				found = true
				break
			}
		}
		if !found {
			next.Providers = append(next.Providers, p)
		}
	}
	next.SeedRevision = list.Revision
	return m.save(next)
}
func ruleVersionNewer(next, previous string) bool {
	if !re(`^\d{1,6}(?:\.\d{1,6}){0,5}$`).MatchString(next) || !re(`^\d{1,6}(?:\.\d{1,6}){0,5}$`).MatchString(previous) {
		return false
	}
	a, b := strings.Split(next, "."), strings.Split(previous, ".")
	for i := 0; i < len(a) || i < len(b); i++ {
		x, y := 0, 0
		if i < len(a) {
			x, _ = strconv.Atoi(a[i])
		}
		if i < len(b) {
			y, _ = strconv.Atoi(b[i])
		}
		if x != y {
			return x > y
		}
	}
	return false
}

func (m *SourceManager) editCMS(d Obj) (Obj, error) {
	m.updateMu.Lock()
	defer m.updateMu.Unlock()
	next := m.snapshot()
	id := sget(d, "id")
	name := text(d["name"], 80)
	u := strings.TrimSpace(sget(d, "url"))
	var e error
	if u != "" {
		u, e = publicURL(u)
		if e != nil {
			return nil, e
		}
	}
	if name == "" {
		return nil, problem("请输入来源名称。", "source_config")
	}
	found := false
	for i, p := range next.Providers {
		if p.ID != id {
			continue
		}
		if p.Origin != "custom" || p.Kind != "cms" {
			return nil, problem("只能编辑自定义来源。", "source_config")
		}
		found = true
		if u == "" {
			u = p.URL
		}
		if p.URL != u {
			delete(next.Health, id)
		}
		p.Name, p.URL = name, u
		next.Providers[i] = p
	}
	if !found {
		return nil, problem("来源不存在。", "not_found", 404)
	}
	if e = m.save(next); e != nil {
		return nil, e
	}
	return m.publicState(), nil
}

func (m *SourceManager) measureSource(ctx context.Context, p Provider, registry SourceRegistry) SourceHealth {
	start := time.Now()
	h := SourceHealth{Status: "unavailable", CheckedAt: now(), Fingerprint: sourceHealthFingerprint(p, registry)}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	var fs []Film
	var e error
	if p.Kind == "kazumi" {
		// Run the supported static/API search rule; a homepage HTTP 200 or a
		// challenge page is not a successful metadata operation.
		fs, e = searchRule(ctx, m.net, p, "海贼王")
	} else {
		endpoint := providerEndpoint(p, registry)
		var d Obj
		d, e = m.net.JSON(ctx, endpoint, Obj{"ac": "detail", "pg": 1})
		if e == nil {
			var rs []Obj
			rs, e = rows(d)
			for _, row := range rs {
				if f, fe := filmFromVOD(row); fe == nil && f.Title != "" {
					fs = append(fs, f)
				}
			}
		}
	}
	h.LatencyMS = time.Since(start).Milliseconds()
	if e != nil {
		h.Message = e.Error()
	} else if len(fs) == 0 {
		h.Message = "未返回可解析的影片条目"
	} else {
		h.Status, h.Message, h.CatalogCount = "available", "目录读取正常", len(fs)
	}
	return h
}

type SourceSpeedJobs struct {
	mu      sync.Mutex
	service *Service
	jobs    map[string]*sourceSpeedJob
	active  string
}
type sourceSpeedJob struct {
	mu        sync.Mutex
	id        string
	state     string
	message   string
	created   time.Time
	completed int
	total     int
	results   []Obj
	cancel    context.CancelFunc
}

func newSourceSpeedJobs(s *Service) *SourceSpeedJobs {
	return &SourceSpeedJobs{service: s, jobs: map[string]*sourceSpeedJob{}}
}
func (j *sourceSpeedJob) stop() {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.state == "running" {
		j.state, j.message = "cancelled", "已取消，本轮结果未保存"
		j.cancel()
	}
}
func (jobs *SourceSpeedJobs) cancelActive() {
	jobs.mu.Lock()
	j := jobs.jobs[jobs.active]
	jobs.mu.Unlock()
	if j != nil {
		j.stop()
	}
}
func (jobs *SourceSpeedJobs) start(d Obj) (Obj, error) {
	if e := jobs.service.requireNetwork(); e != nil {
		return nil, e
	}
	ps := jobs.service.manager.providers()
	wanted := map[string]bool{}
	if raw, ok := d["ids"]; ok {
		ids, ok := raw.([]any)
		if !ok || len(ids) == 0 || len(ids) > 400 {
			return nil, problem("请选择有效的来源。", "source_config")
		}
		for _, id := range ids {
			v, ok := id.(string)
			if !ok || wanted[v] {
				return nil, problem("来源编号无效或重复。", "source_config")
			}
			wanted[v] = true
		}
		selected := []Provider{}
		for _, p := range ps {
			if wanted[p.ID] {
				selected = append(selected, p)
			}
		}
		if len(selected) != len(wanted) {
			return nil, problem("来源不存在。", "not_found", 404)
		}
		ps = selected
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	j := &sourceSpeedJob{id: token(), state: "running", created: time.Now(), total: len(ps), results: []Obj{}, cancel: cancel}
	jobs.mu.Lock()
	if old := jobs.jobs[jobs.active]; old != nil {
		old.stop()
	}
	for id, old := range jobs.jobs {
		if time.Since(old.created) > 5*time.Minute {
			old.stop()
			delete(jobs.jobs, id)
		}
	}
	if len(jobs.jobs) >= 4 {
		var oldest *sourceSpeedJob
		for _, old := range jobs.jobs {
			if oldest == nil || old.created.Before(oldest.created) {
				oldest = old
			}
		}
		oldest.stop()
		delete(jobs.jobs, oldest.id)
	}
	jobs.jobs[j.id], jobs.active = j, j.id
	jobs.mu.Unlock()
	go jobs.run(ctx, j, ps)
	return jobs.snapshot(j), nil
}
func (jobs *SourceSpeedJobs) snapshot(j *sourceSpeedJob) Obj {
	j.mu.Lock()
	percent := 0
	if j.total > 0 {
		percent = 100 * j.completed / j.total
	}
	r := Obj{"id": j.id, "state": j.state, "completed": j.completed, "total": j.total, "progress": percent, "percent": percent, "results": cloneList(j.results), "message": j.message}
	j.mu.Unlock()
	r["sources"] = jobs.service.manager.publicState()
	return r
}
func (jobs *SourceSpeedJobs) status(id string) (Obj, error) {
	jobs.mu.Lock()
	j := jobs.jobs[id]
	jobs.mu.Unlock()
	if j == nil {
		return nil, problem("测速任务已过期。", "source_speed_missing", 404)
	}
	return jobs.snapshot(j), nil
}
func (jobs *SourceSpeedJobs) cancel(id string) (Obj, error) {
	jobs.mu.Lock()
	j := jobs.jobs[id]
	jobs.mu.Unlock()
	if j == nil {
		return nil, problem("测速任务已过期。", "source_speed_missing", 404)
	}
	j.stop()
	return jobs.snapshot(j), nil
}
func (jobs *SourceSpeedJobs) run(ctx context.Context, j *sourceSpeedJob, ps []Provider) {
	defer j.cancel()
	m := jobs.service.manager
	registry := m.snapshot()
	measurements := make(map[string]SourceHealth)
	sem := make(chan struct{}, 6)
	var wg sync.WaitGroup
	for _, p := range ps {
		wg.Add(1)
		go func(p Provider) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()
			if ctx.Err() != nil {
				return
			}
			h := m.measureSource(ctx, p, registry)
			b, _ := json.Marshal(h)
			var result Obj
			_ = json.Unmarshal(b, &result)
			delete(result, "fingerprint")
			result["id"], result["name"] = p.ID, p.Name
			j.mu.Lock()
			if j.state == "running" {
				measurements[p.ID] = h
				j.completed++
				j.results = append(j.results, result)
			}
			j.mu.Unlock()
		}(p)
	}
	wg.Wait()
	if ctx.Err() != nil {
		j.mu.Lock()
		if j.state == "running" {
			j.state, j.message = "failed", "测速超时，请缩小范围后重试"
		}
		j.mu.Unlock()
		return
	}
	// Updates and editing serialize registry writes. Wait cancellably rather
	// than blocking local progress polling behind an upstream network request.
	for !m.updateMu.TryLock() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(20 * time.Millisecond):
		}
	}
	defer m.updateMu.Unlock()
	if ctx.Err() != nil {
		return
	}
	next := m.snapshot()
	if next.Health == nil {
		next.Health = map[string]SourceHealth{}
	}
	for _, p := range next.Providers {
		if h, ok := measurements[p.ID]; ok && h.Fingerprint == sourceHealthFingerprint(p, next) {
			next.Health[p.ID] = h
		}
	}
	e := m.save(next)
	j.mu.Lock()
	if j.state == "running" {
		j.state = "done"
		if e != nil {
			j.state, j.message = "failed", "测速完成，但结果未能保存"
		}
	}
	j.mu.Unlock()
}
