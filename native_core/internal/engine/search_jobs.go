// SPDX-License-Identifier: GPL-3.0-or-later
package engine

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"
)

// One active search per local service. Polling is local and does not restart probes.
type SearchJobs struct {
	mu           sync.Mutex
	service      *Service
	checker      *AvailabilityChecker
	jobs         map[string]*searchJob
	active       string
	timeout      time.Duration
	checkTimeout time.Duration
}
type searchJob struct {
	mu                                           sync.Mutex
	id, query, phase, message                    string
	created                                      time.Time
	cancel                                       context.CancelFunc
	sourceCompleted, sourceTotal, checked, total int
	items                                        []Obj
	result                                       Obj
}

func newSearchJobs(s *Service) *SearchJobs {
	return &SearchJobs{service: s, checker: newAvailabilityChecker(s.manager), jobs: map[string]*searchJob{}, timeout: 150 * time.Second, checkTimeout: 110 * time.Second}
}
func (jobs *SearchJobs) start(d Obj) (Obj, error) {
	q, ok := d["query"].(string)
	q = strings.TrimSpace(q)
	if !ok || len([]rune(q)) < 1 || len([]rune(q)) > 120 {
		return nil, problem("请输入 1–120 个字符的片名。", "query")
	}
	if e := jobs.service.requireNetwork(); e != nil {
		return nil, e
	}
	mode := defaultStr(d, "mode", "all")
	if mode != "all" && mode != "anime" && mode != "catalog" && !strings.HasPrefix(mode, "provider:") {
		if _, ok := platforms[mode]; !ok {
			return nil, problem("搜索来源无效。", "source_config")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), jobs.timeout)
	j := &searchJob{id: token(), query: q, phase: "searching", created: time.Now(), cancel: cancel, sourceTotal: 1, items: []Obj{}, result: Obj{}}
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
		var oldest *searchJob
		for _, old := range jobs.jobs {
			if oldest == nil || old.created.Before(oldest.created) {
				oldest = old
			}
		}
		oldest.stop()
		delete(jobs.jobs, oldest.id)
	}
	jobs.jobs[j.id] = j
	jobs.active = j.id
	jobs.mu.Unlock()
	go jobs.run(ctx, j, mode)
	return j.snapshot(), nil
}
func (jobs *SearchJobs) status(id string) (Obj, error) {
	jobs.mu.Lock()
	j := jobs.jobs[id]
	jobs.mu.Unlock()
	if j == nil {
		return nil, problem("搜索任务已过期，请重新搜索。", "search_missing", 404)
	}
	return j.snapshot(), nil
}
func (jobs *SearchJobs) cancel(id string) (Obj, error) {
	jobs.mu.Lock()
	j := jobs.jobs[id]
	jobs.mu.Unlock()
	if j == nil {
		return nil, problem("搜索任务已过期。", "search_missing", 404)
	}
	j.stop()
	return j.snapshot(), nil
}
func (jobs *SearchJobs) cancelActive() {
	jobs.mu.Lock()
	j := jobs.jobs[jobs.active]
	jobs.mu.Unlock()
	if j != nil {
		j.stop()
	}
}
func (j *searchJob) stop() {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.phase == "searching" || j.phase == "checking" {
		j.phase = "cancelled"
		j.message = "已取消"
		j.cancel()
	}
}
func (j *searchJob) snapshot() Obj {
	j.mu.Lock()
	defer j.mu.Unlock()
	r := Obj{}
	for k, v := range j.result {
		r[k] = v
	}
	percent := 0
	if j.sourceTotal > 0 {
		percent = 30 * j.sourceCompleted / j.sourceTotal
	}
	if j.phase == "checking" && j.total > 0 {
		percent = 30 + 69*j.checked/j.total
	}
	if j.phase == "done" {
		percent = 100
	}
	available := 0
	items := append([]Obj{}, j.items...)
	sort.SliceStable(items, func(i, k int) bool {
		a, _ := items[i]["availability"].(Availability)
		b, _ := items[k]["availability"].(Availability)
		if availabilityPriority(a) != availabilityPriority(b) {
			return availabilityPriority(a) < availabilityPriority(b)
		}
		x, y := ranking(j.query, sget(items[i], "title"), sget(items[i], "year")), ranking(j.query, sget(items[k], "title"), sget(items[k], "year"))
		if x != y {
			return x > y
		}
		return sget(items[i], "source") < sget(items[k], "source")
	})
	for _, v := range items {
		a, _ := v["availability"].(Availability)
		if a.State == "media_ok" || a.State == "page_ok" {
			available++
		}
	}
	r["id"] = j.id
	r["phase"] = j.phase
	r["message"] = j.message
	r["percent"] = percent
	r["source_completed"] = j.sourceCompleted
	r["source_total"] = j.sourceTotal
	r["checked_count"] = j.checked
	r["check_total"] = j.total
	r["available_count"] = available
	r["items"] = items
	// Deep copy under the lock: workers may replace individual availability values.
	b, _ := json.Marshal(r)
	var out Obj
	_ = json.Unmarshal(b, &out)
	return out
}
func (jobs *SearchJobs) run(ctx context.Context, j *searchJob, mode string) {
	defer j.cancel()
	var r Obj
	var e error
	if mode == "all" || mode == "anime" || strings.HasPrefix(mode, "provider:") {
		r, e = jobs.service.manager.searchWithProgress(ctx, j.query, mode, func(done, total int) { j.mu.Lock(); j.sourceCompleted = done; j.sourceTotal = total; j.mu.Unlock() })
	} else {
		r, e = jobs.service.Search(ctx, Obj{"query": j.query, "mode": mode})
	}
	if e != nil {
		j.mu.Lock()
		if j.phase != "cancelled" {
			j.phase = "failed"
			j.message = e.Error()
		}
		j.mu.Unlock()
		return
	}
	items, _ := r["items"].([]Obj)
	// Cards do not need thousands of episode names on every progress poll.
	for i, item := range items {
		compact := Obj{}
		for _, key := range []string{"id", "title", "year", "kind", "area", "remarks", "source", "provider_id", "provider_kind", "demo"} {
			if value, ok := item[key]; ok {
				compact[key] = value
			}
		}
		items[i] = compact
	}
	j.mu.Lock()
	if j.phase == "cancelled" {
		j.mu.Unlock()
		return
	}
	j.result = r
	j.sourceCompleted = j.sourceTotal
	j.total = len(items)
	j.phase = "checking"
	for _, item := range items {
		item["availability"] = Availability{State: "pending", Label: "检测中"}
	}
	j.items = items
	j.mu.Unlock()
	checkCtx, cancel := context.WithTimeout(ctx, jobs.checkTimeout)
	defer cancel()
	queue := make(chan int)
	var wg sync.WaitGroup
	for worker := 0; worker < 6; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range queue {
				var f Film
				b, _ := json.Marshal(items[i])
				_ = json.Unmarshal(b, &f)
				var a Availability
				if f.ProviderID == "" {
					var de error
					f, de = jobs.service.Detail(checkCtx, f.ID)
					if de != nil {
						a = Availability{State: "failed", Label: "暂不可用", Error: de.Error(), CheckedAt: now()}
						if checkCtx.Err() != nil {
							a.State, a.Label = "unchecked", "未检测"
						}
					}
				}
				if a.State == "" {
					a = jobs.checker.Check(checkCtx, f)
				}
				j.mu.Lock()
				if j.phase == "checking" {
					items[i]["availability"] = a
					if a.State != "unchecked" {
						j.checked++
					}
				}
				j.mu.Unlock()
			}
		}()
	}
feed:
	for i := range items {
		select {
		case queue <- i:
		case <-checkCtx.Done():
			break feed
		}
	}
	close(queue)
	wg.Wait()
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.phase == "cancelled" {
		return
	}
	for _, item := range items {
		a, _ := item["availability"].(Availability)
		if a.State == "pending" {
			item["availability"] = Availability{State: "unchecked", Label: "未检测"}
		}
	}
	j.phase = "done"
	if j.checked < j.total {
		j.message = "本轮检测结束，剩余结果排在后面"
	}
}
