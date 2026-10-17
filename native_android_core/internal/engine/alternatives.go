// SPDX-License-Identifier: GPL-3.0-or-later
package engine

import (
	"context"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

// Suggestions are opt-in playback actions: neither a job nor a successful probe
// resolves media, writes history, or switches the active player.
type AlternativeJobs struct {
	mu      sync.Mutex
	service *Service
	jobs    map[string]*alternativeJob
	active  string
	timeout time.Duration
}
type alternativeJob struct {
	mu                                           sync.Mutex
	id, phase, message                           string
	created                                      time.Time
	cancel                                       context.CancelFunc
	items                                        []Obj
	position                                     float64
	sourceCompleted, sourceTotal, checked, total int
}

func newAlternativeJobs(s *Service) *AlternativeJobs {
	return &AlternativeJobs{service: s, jobs: map[string]*alternativeJob{}, timeout: 55 * time.Second}
}
func (jobs *AlternativeJobs) start(d Obj) (Obj, error) {
	if e := jobs.service.requireNetwork(); e != nil {
		return nil, e
	}
	if !boolSetting(jobs.service.store.Settings(), "smart_source_switch", true) {
		return nil, problem("换源建议已关闭。", "switch_disabled")
	}
	for _, key := range []string{"film_id", "group_key", "episode_key"} {
		if sget(d, key) == "" || len(sget(d, key)) > 6100 {
			return nil, problem("请选择当前影片与集数。", "episode")
		}
	}
	position := float64(0)
	if v, ok := d["position"]; ok {
		var valid bool
		position, valid = numValue(v)
		if !valid || math.IsNaN(position) || math.IsInf(position, 0) || position < 0 || position > 604800 {
			return nil, problem("无效播放进度。", "position")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), jobs.timeout)
	j := &alternativeJob{id: token(), phase: "searching", created: time.Now(), cancel: cancel, items: []Obj{}, position: position}
	jobs.mu.Lock()
	if previous := jobs.jobs[jobs.active]; previous != nil {
		previous.stop()
	}
	for id, old := range jobs.jobs {
		if time.Since(old.created) > 5*time.Minute {
			old.stop()
			delete(jobs.jobs, id)
		}
	}
	if len(jobs.jobs) >= 4 {
		var oldest *alternativeJob
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
	request := cloneObj(d)
	go jobs.run(ctx, j, request)
	return j.snapshot(), nil
}
func (jobs *AlternativeJobs) status(id string) (Obj, error) {
	jobs.mu.Lock()
	j := jobs.jobs[id]
	jobs.mu.Unlock()
	if j == nil {
		return nil, problem("换源任务已过期。", "switch_missing", 404)
	}
	return j.snapshot(), nil
}
func (jobs *AlternativeJobs) cancel(id string) (Obj, error) {
	jobs.mu.Lock()
	j := jobs.jobs[id]
	jobs.mu.Unlock()
	if j == nil {
		return nil, problem("换源任务已过期。", "switch_missing", 404)
	}
	j.stop()
	return j.snapshot(), nil
}
func (jobs *AlternativeJobs) cancelActive() {
	jobs.mu.Lock()
	j := jobs.jobs[jobs.active]
	jobs.mu.Unlock()
	if j != nil {
		j.stop()
	}
}
func (j *alternativeJob) stop() {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.phase == "searching" || j.phase == "checking" {
		j.phase, j.message = "cancelled", "已取消"
		j.cancel()
	}
}
func (j *alternativeJob) snapshot() Obj {
	j.mu.Lock()
	defer j.mu.Unlock()
	percent := 0
	if j.sourceTotal > 0 {
		percent = 40 * j.sourceCompleted / j.sourceTotal
	}
	if j.phase == "checking" && j.total > 0 {
		percent = 40 + 59*j.checked/j.total
	}
	if j.phase == "done" {
		percent = 100
	}
	items := cloneList(j.items)
	return Obj{"id": j.id, "phase": j.phase, "message": j.message, "items": items, "candidates": items, "position": j.position, "checked_count": j.checked, "check_total": j.total, "source_completed": j.sourceCompleted, "source_total": j.sourceTotal, "percent": percent}
}

func sameAlternativeIdentity(a, b Film) bool {
	if a.ID == b.ID {
		return true
	}
	if a.Demo || b.Demo {
		return false
	}
	sa, sb := season(a.Title), season(b.Title)
	if (sa == nil) != (sb == nil) || sa != nil && *sa != *sb {
		return false
	}
	base := func(title string) string {
		title = seasonCN.ReplaceAllString(title, "")
		title = seasonEN.ReplaceAllString(title, "")
		return searchNorm(title)
	}
	if base(a.Title) == "" || base(a.Title) != base(b.Title) {
		return false
	}
	// Missing years cannot rule out remakes or unrelated same-name works.
	ya, yb := yearRE.FindString(a.Year), yearRE.FindString(b.Year)
	return ya != "" && ya == yb
}

func matchingAlternativeEpisode(current Film, selected Episode, candidate Film, g Group) (Episode, error) {
	if !sameAlternativeIdentity(current, candidate) {
		return Episode{}, problem("作品版本不同。", "identity")
	}
	// Film entries named 正片/HD can match another unambiguous single-film entry.
	if selected.Ordinal == nil && !ambiguousEp.MatchString(selected.Name) && len(g.Episodes) == 1 && g.Episodes[0].Ordinal == nil && !ambiguousEp.MatchString(g.Episodes[0].Name) {
		movie := strings.Contains(current.Kind, "电影") || strings.Contains(strings.ToLower(current.Kind), "movie")
		if movie {
			return g.Episodes[0], nil
		}
	}
	return chooseEpisode(g, selected.Name)
}

func (jobs *AlternativeJobs) run(ctx context.Context, j *alternativeJob, d Obj) {
	defer j.cancel()
	s := jobs.service
	f, g, ep, e := s.Select(ctx, d)
	if e != nil {
		j.mu.Lock()
		if j.phase != "cancelled" {
			j.phase, j.message = "failed", e.Error()
		}
		j.mu.Unlock()
		return
	}
	films := []Film{f}
	providers := []Provider{}
	for _, p := range s.manager.providers() {
		if p.Enabled && p.ID != f.ProviderID {
			providers = append(providers, p)
			if len(providers) >= 12 {
				break
			}
		}
	}
	j.mu.Lock()
	j.sourceTotal = len(providers)
	j.mu.Unlock()
	// A small priority-ordered provider set and shared deadline bound extra work
	// while an existing video is stalled. The regular search remains available.
	searchCtx, cancelSearch := context.WithTimeout(ctx, 25*time.Second)
	queue := make(chan int)
	results := make([][]Film, len(providers))
	var wg sync.WaitGroup
	for worker := 0; worker < 3; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range queue {
				pctx, cancel := context.WithTimeout(searchCtx, 8*time.Second)
				fs, err := s.manager.searchProvider(pctx, providers[i], f.Title)
				cancel()
				if err == nil {
					for _, candidate := range fs {
						if sameAlternativeIdentity(f, candidate) {
							results[i] = append(results[i], candidate)
							if len(results[i]) >= 3 {
								break
							}
						}
					}
				}
				j.mu.Lock()
				j.sourceCompleted++
				j.mu.Unlock()
			}
		}()
	}
feed:
	for i := range providers {
		select {
		case queue <- i:
		case <-searchCtx.Done():
			break feed
		}
	}
	close(queue)
	wg.Wait()
	cancelSearch()
	for _, fs := range results {
		films = append(films, fs...)
	}
	type choice struct {
		film    Film
		group   Group
		episode Episode
	}
	choices := []choice{}
	for _, candidate := range films {
		if ctx.Err() != nil {
			break
		}
		if candidate.ID != f.ID && len(candidate.Groups) == 0 {
			detailCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			candidate, e = s.Detail(detailCtx, candidate.ID)
			cancel()
			if e != nil || !sameAlternativeIdentity(f, candidate) {
				continue
			}
		}
		for _, group := range candidate.Groups {
			if candidate.ID == f.ID && group.Key == g.Key {
				continue
			}
			matched, err := matchingAlternativeEpisode(f, ep, candidate, group)
			if err == nil {
				choices = append(choices, choice{candidate, group, matched})
				if len(choices) >= 12 {
					break
				}
			}
		}
		if len(choices) >= 12 {
			break
		}
	}
	j.mu.Lock()
	if j.phase == "cancelled" {
		j.mu.Unlock()
		return
	}
	j.phase, j.total = "checking", len(choices)
	j.mu.Unlock()
	checkQueue := make(chan int)
	verified := make([]Obj, len(choices))
	for worker := 0; worker < 3; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range checkQueue {
				choice := choices[i]
				a := s.searches.checker.CheckEpisode(ctx, choice.film, choice.group.Key, choice.episode.Key)
				if a.State == "media_ok" {
					verified[i] = Obj{"film_id": choice.film.ID, "title": choice.film.Title, "year": choice.film.Year, "source": choice.film.Source, "provider_id": choice.film.ProviderID, "group_key": choice.group.Key, "episode_key": choice.episode.Key, "episode": choice.episode.Name, "position": j.position, "available": true, "availability": a, "playback_verified": false}
				}
				j.mu.Lock()
				j.checked++
				j.mu.Unlock()
			}
		}()
	}
checkFeed:
	for i := range choices {
		select {
		case checkQueue <- i:
		case <-ctx.Done():
			break checkFeed
		}
	}
	close(checkQueue)
	wg.Wait()
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.phase == "cancelled" {
		return
	}
	if s.requireNetwork() != nil {
		j.phase, j.message = "cancelled", "联网已关闭"
		return
	}
	for _, item := range verified {
		if item != nil {
			j.items = append(j.items, item)
		}
	}
	sort.SliceStable(j.items, func(i, k int) bool {
		a, _ := j.items[i]["availability"].(Availability)
		b, _ := j.items[k]["availability"].(Availability)
		if a.LatencyMS != b.LatencyMS {
			return a.LatencyMS < b.LatencyMS
		}
		return sget(j.items[i], "source") < sget(j.items[k], "source")
	})
	j.phase = "done"
	if len(j.items) == 0 {
		j.message = "未找到版本、集数一致且媒体可访问的替代来源"
	} else {
		j.message = "请选择换源；将保留当前集数与播放位置"
	}
	if ctx.Err() != nil {
		j.message += "（本轮检测已到时限）"
	}
}
