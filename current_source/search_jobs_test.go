// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

type jobTestNetwork struct {
	*fakeNetwork
	rows      []Obj
	block     chan struct{}
	mu        sync.Mutex
	cancelled int
}

func (n *jobTestNetwork) JSON(ctx context.Context, link string, p Obj) (Obj, error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	list := []any{}
	for _, r := range n.rows {
		if ids := sget(p, "ids"); ids == "" || ids == str(r["vod_id"]) {
			list = append(list, r)
		}
	}
	return Obj{"list": list}, nil
}
func (n *jobTestNetwork) Request(ctx context.Context, r RequestSpec) ([]byte, Obj, error) {
	if strings.Contains(r.URL, "slow") && n.block != nil {
		select {
		case <-ctx.Done():
			n.mu.Lock()
			n.cancelled++
			n.mu.Unlock()
			return nil, nil, ctx.Err()
		case <-n.block:
		}
	}
	if strings.Contains(r.URL, "bad") {
		return nil, nil, problem("来源返回 HTTP 503", "http", 502)
	}
	return []byte(availabilityMP4()), Obj{"http": 206}, nil
}
func (n *jobTestNetwork) Fetch(ctx context.Context, u string, limit int) ([]byte, Obj, error) {
	return n.Request(ctx, RequestSpec{URL: u, Method: "GET", Limit: limit})
}
func jobService(t *testing.T, slow bool) (*Service, *jobTestNetwork) {
	t.Helper()
	path := "good.mp4"
	if slow {
		path = "slow.mp4"
	}
	n := &jobTestNetwork{fakeNetwork: &fakeNetwork{}, rows: []Obj{
		{"vod_id": 1, "vod_name": "样本", "vod_play_from": "坏线路", "vod_play_url": "第01集$https://media.example.com/bad.mp4"},
		{"vod_id": 2, "vod_name": "样本 正常", "vod_play_from": "正常", "vod_play_url": "第01集$https://media.example.com/" + path},
		{"vod_id": 3, "vod_name": "样本 备用", "vod_play_from": "坏线路$$$好线路", "vod_play_url": "第01集$https://media.example.com/bad.mp4$$$第01集$https://media.example.com/" + path},
	}}
	if slow {
		n.block = make(chan struct{})
	}
	s, e := newService(t.TempDir(), n)
	mustNoErr(t, e)
	r := s.manager.snapshot()
	r.Providers = []Provider{{ID: "test", Name: "测试源", Kind: "cms", URL: "https://cms.example.com/api.php/provide/vod/", Enabled: true, Origin: "custom"}}
	mustNoErr(t, s.manager.save(r))
	allowNetwork(t, s)
	t.Cleanup(s.searches.cancelActive)
	return s, n
}
func waitJob(t *testing.T, s *Service, id, phase string) Obj {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		v, e := s.searches.status(id)
		mustNoErr(t, e)
		if sget(v, "phase") == phase {
			return v
		}
		time.Sleep(2 * time.Millisecond)
	}
	v, _ := s.searches.status(id)
	t.Fatalf("job never %s: %v", phase, v)
	return nil
}
func TestSearchJobs(t *testing.T) {
	t.Run("sorts_actual_availability_and_prefers_working_group", func(t *testing.T) {
		s, _ := jobService(t, false)
		start, e := s.searches.start(Obj{"query": "样本", "mode": "all"})
		mustNoErr(t, e)
		r := waitJob(t, s, sget(start, "id"), "done")
		require(t, r["percent"] == float64(100) && r["checked_count"] == float64(3) && r["available_count"] == float64(2), "bad final progress")
		items := r["items"].([]any)
		for i, raw := range items {
			v := raw.(map[string]any)
			a := v["availability"].(map[string]any)
			if i < 2 {
				require(t, a["state"] == "media_ok", "bad candidate ahead of sampled media")
			} else {
				require(t, a["state"] == "failed", "bad candidate missing")
			}
			if strings.Contains(sget(v, "title"), "备用") {
				require(t, a["group_key"] == "g1", "failed group selected")
			}
		}
		b, _ := json.Marshal(r)
		require(t, !strings.Contains(string(b), "media.example.com") && !strings.Contains(string(b), "episodes"), "raw URLs or unnecessary episodes in status")
		items[0].(map[string]any)["title"] = "mutated"
		again, _ := s.searches.status(sget(start, "id"))
		require(t, !strings.Contains(sget(again, "items"), "mutated"), "mutable snapshot escaped")
	})
	t.Run("replacement_cancels_previous_job", func(t *testing.T) {
		s, n := jobService(t, true)
		first, e := s.searches.start(Obj{"query": "样本", "mode": "all"})
		mustNoErr(t, e)
		waitJob(t, s, sget(first, "id"), "checking")
		second, e := s.searches.start(Obj{"query": "新样本", "mode": "all"})
		mustNoErr(t, e)
		waitJob(t, s, sget(first, "id"), "cancelled")
		close(n.block)
		waitJob(t, s, sget(second, "id"), "done")
		old, _ := s.searches.status(sget(first, "id"))
		require(t, old["phase"] == "cancelled", "old work resurrected")
	})
	t.Run("revoking_network_cancels_checks", func(t *testing.T) {
		s, _ := jobService(t, true)
		first, e := s.searches.start(Obj{"query": "样本", "mode": "all"})
		mustNoErr(t, e)
		waitJob(t, s, sget(first, "id"), "checking")
		_, e = s.Settings(Obj{"network_consent": false})
		mustNoErr(t, e)
		waitJob(t, s, sget(first, "id"), "cancelled")
		_, e = s.searches.start(Obj{"query": "样本"})
		require(t, errCode(e) == "consent_required", "offline job launched")
	})
	t.Run("budget_preserves_untested_candidates", func(t *testing.T) {
		s, _ := jobService(t, true)
		s.searches.checkTimeout = 30 * time.Millisecond
		first, e := s.searches.start(Obj{"query": "样本", "mode": "all"})
		mustNoErr(t, e)
		r := waitJob(t, s, sget(first, "id"), "done")
		require(t, r["checked_count"].(float64) < r["check_total"].(float64), "budget not represented")
		for _, raw := range r["items"].([]any) {
			a := raw.(map[string]any)["availability"].(map[string]any)
			require(t, a["state"] != "pending", "pending result left enabled after completion")
		}
	})
	t.Run("input_validation_and_missing_tasks", func(t *testing.T) {
		s, _ := jobService(t, false)
		for _, d := range []Obj{{"query": ""}, {"query": "ok", "mode": "invalid"}} {
			_, e := s.searches.start(d)
			require(t, e != nil, "invalid task accepted")
		}
		_, e := s.searches.status("unknown")
		require(t, errCode(e) == "search_missing", "unknown job accessible")
		_, e = s.searches.cancel("unknown")
		require(t, errCode(e) == "search_missing", "unknown cancellation accepted")
	})
}
func TestSearchJobsHTTP(t *testing.T) {
	s, _ := jobService(t, false)
	srv, e := newServer(s, 0)
	mustNoErr(t, e)
	defer srv.listener.Close()
	w := perform(srv, "/api/search/start", "POST", Obj{"query": "样本", "mode": "all"}, true, nil)
	require(t, w.Code == 200, w.Body.String())
	var body Obj
	mustNoErr(t, json.Unmarshal(w.Body.Bytes(), &body))
	id := sget(body["data"].(map[string]any), "id")
	require(t, perform(srv, "/api/search/status?id="+id, "GET", nil, false, nil).Code == 403, "anonymous status allowed")
	require(t, perform(srv, "/api/search/cancel", "POST", Obj{"id": id}, true, map[string]string{"X-YingXia-CSRF": "wrong"}).Code == 403, "cancel without csrf allowed")
	require(t, perform(srv, "/api/search/status?id="+id, "GET", nil, true, nil).Code == 200, "authenticated status failed")
}
func TestWebResultUsesActualPlaybackPage(t *testing.T) {
	s, _ := fixtureService(t)
	allowNetwork(t, s)
	s.links["web-fixture"] = availabilityFilm("web-fixture", "https://video.example.com/play/1")
	r, e := s.Resolve(context.Background(), Obj{"film_id": "web-fixture", "group_key": "g0", "episode_key": "e0", "source": "web"})
	mustNoErr(t, e)
	require(t, r["kind"] == "page" && r["url"] == "https://video.example.com/play/1", "readable page treated as native media")
	s.links["web-private"] = availabilityFilm("web-private", "http://127.0.0.1/private")
	_, e = s.Resolve(context.Background(), Obj{"film_id": "web-private", "group_key": "g0", "episode_key": "e0", "source": "web"})
	require(t, errCode(e) == "unsafe_url", "private webpage accepted")
}
