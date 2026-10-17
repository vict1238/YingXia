// SPDX-License-Identifier: GPL-3.0-or-later
package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func nativeRequest(t *testing.T, r *Runtime, method, path string, body Obj, auth bool) (int, Obj, http.Header) {
	t.Helper()
	var raw []byte
	if body != nil {
		raw, _ = json.Marshal(body)
	}
	req, e := http.NewRequest(method, r.server.origin+path, bytes.NewReader(raw))
	if e != nil {
		t.Fatal(e)
	}
	if auth {
		req.Header.Set("Cookie", r.server.cookieName+"="+r.server.session)
		req.Header.Set("Origin", r.server.origin)
		req.Header.Set("X-YingXia-CSRF", r.server.csrf)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, e := http.DefaultClient.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	defer res.Body.Close()
	raw, e = io.ReadAll(res.Body)
	if e != nil {
		t.Fatal(e)
	}
	var decoded Obj
	if e = json.Unmarshal(raw, &decoded); e != nil {
		t.Fatalf("invalid response: %s", raw)
	}
	return res.StatusCode, decoded, res.Header
}

func TestNativePrivateAPIAndClearHistory(t *testing.T) {
	dir := t.TempDir()
	r, e := NewRuntime(dir)
	if e != nil {
		t.Fatal(e)
	}
	defer r.Stop()
	var descriptor Obj
	if e = json.Unmarshal([]byte(r.Bootstrap()), &descriptor); e != nil {
		t.Fatal(e)
	}
	if descriptor["version"] != version || !strings.HasPrefix(sget(descriptor, "origin"), "http://127.0.0.1:") || sget(descriptor, "cookie") == "" || sget(descriptor, "csrf") == "" {
		t.Fatal("invalid native bootstrap")
	}
	for _, path := range []string{"/", "/bootstrap.js", "/api/state", "/api/library"} {
		status, _, headers := nativeRequest(t, r, "GET", path, nil, false)
		if status != 403 || headers.Get("Set-Cookie") != "" {
			t.Fatalf("unauthenticated %s exposes session: %d", path, status)
		}
	}
	status, _, _ := nativeRequest(t, r, "GET", "/", nil, true)
	if status != 404 {
		t.Fatal("native server unexpectedly exposes HTML UI")
	}
	store := r.server.service.store
	if _, e = store.Toggle(Obj{"id": "favorite-one", "title": "收藏保留"}); e != nil {
		t.Fatal(e)
	}
	historyID, e := store.Opened("film-one", "测试记录", "第 1 集", "fixture", "media")
	if e != nil {
		t.Fatal(e)
	}
	status, body, _ := nativeRequest(t, r, "POST", "/api/clear-history", Obj{}, false)
	if status != 403 || body["error"] != "session" || len(store.History()) != 1 {
		t.Fatal("clear requires authentication")
	}
	req, _ := http.NewRequest("POST", r.server.origin+"/api/clear-history", strings.NewReader("{}"))
	req.Header.Set("Cookie", r.server.cookieName+"="+r.server.session)
	req.Header.Set("Content-Type", "application/json")
	res, e := http.DefaultClient.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	res.Body.Close()
	if res.StatusCode != 403 || len(store.History()) != 1 {
		t.Fatal("clear requires CSRF")
	}
	status, body, _ = nativeRequest(t, r, "POST", "/api/clear-history", Obj{}, true)
	if status != 200 || body["ok"] != true {
		t.Fatal("authenticated clear failed")
	}
	status, body, _ = nativeRequest(t, r, "GET", "/api/library", nil, true)
	data := body["data"].(map[string]any)
	if status != 200 || len(data["history"].([]any)) != 0 || len(data["favorites"].([]any)) != 1 {
		t.Fatal("clear affected library incorrectly")
	}
	status, body, _ = nativeRequest(t, r, "POST", "/api/progress", Obj{"history_id": historyID, "evidence": "browser_time_advanced", "position": 12}, true)
	if status != 200 || body["ok"] != true || len(store.History()) != 0 {
		t.Fatal("late player progress resurrected deleted history")
	}
	r.Stop()
	if r.Running() {
		t.Fatal("runtime did not stop")
	}
	r2, e := NewRuntime(dir)
	if e != nil {
		t.Fatal(e)
	}
	defer r2.Stop()
	if len(r2.server.service.store.History()) != 0 || len(r2.server.service.store.Favorites()) != 1 {
		t.Fatal("clear did not persist across native restart")
	}
}

func TestNativeClearHistoryWriteFailureRollsBack(t *testing.T) {
	dir := t.TempDir()
	s, e := newStore(dir)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Opened("a", "a", "1", "fixture", "media"); e != nil {
		t.Fatal(e)
	}
	original, e := os.ReadFile(s.path)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.Remove(s.path); e != nil {
		t.Fatal(e)
	}
	if e = os.Mkdir(s.path, 0700); e != nil {
		t.Fatal(e)
	}
	if e = s.ClearHistory(); e == nil || len(s.History()) != 1 {
		t.Fatal("storage failure lost in-memory history")
	}
	if s.HistoryGeneration() != 0 {
		t.Fatal("failed clear invalidated pending playback")
	}
	if e = os.Remove(s.path); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(dir, "library.json"), original, 0600); e != nil {
		t.Fatal(e)
	}
	reopened, e := newStore(dir)
	if e != nil || len(reopened.History()) != 1 {
		t.Fatal("storage failure lost disk history")
	}
}

type nativeBlockingNetwork struct {
	nativeFixtureFetcher
	started chan struct{}
	release chan struct{}
}

func (n *nativeBlockingNetwork) Fetch(ctx context.Context, _ string, _ int) ([]byte, Obj, error) {
	close(n.started)
	select {
	case <-n.release:
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	}
	return []byte(`<video src="https://media.example.org/video.webm"></video>`), Obj{"content_type": "text/html"}, nil
}

func TestNativeClearWhileResolveIsPending(t *testing.T) {
	n := &nativeBlockingNetwork{started: make(chan struct{}), release: make(chan struct{})}
	s, e := newService(t.TempDir(), n)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.store.SetMany(Obj{"network_consent": true}); e != nil {
		t.Fatal(e)
	}
	s.links["pending-film"] = Film{ID: "pending-film", Title: "pending resolve", Source: "fixture", Groups: []Group{{Key: "g0", Name: "fixture", Episodes: []Episode{{Key: "e0", Name: "1", URL: "https://player.example.org/pending"}}}}}
	result := make(chan Obj, 1)
	failure := make(chan error, 1)
	go func() {
		v, err := s.Resolve(context.Background(), Obj{"film_id": "pending-film", "group_key": "g0", "episode_key": "e0", "source": "native"})
		result <- v
		failure <- err
	}()
	select {
	case <-n.started:
	case <-time.After(time.Second):
		t.Fatal("resolve did not enter blocked network request")
	}
	if e = s.store.ClearHistory(); e != nil {
		t.Fatal(e)
	}
	close(n.release)
	select {
	case v := <-result:
		if sget(v, "history_id") != "" {
			t.Fatal("stale resolve has history id")
		}
	case <-time.After(time.Second):
		t.Fatal("resolve did not finish")
	}
	if e = <-failure; e != nil {
		t.Fatal(e)
	}
	if len(s.store.History()) != 0 {
		t.Fatal("pending playback resurrected history after clear")
	}
	if _, e = s.store.OpenedOnGeneration(s.store.HistoryGeneration(), "new", "new", "1", "fixture", "media"); e != nil || len(s.store.History()) != 1 {
		t.Fatal("new playback cannot save history after clear")
	}
}

func TestNativeRetiredStoreCannotWrite(t *testing.T) {
	dir := t.TempDir()
	s, e := newStore(dir)
	if e != nil {
		t.Fatal(e)
	}
	generation := s.HistoryGeneration()
	s.Close()
	s2, e := newStore(dir)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s2.Opened("new", "new", "1", "fixture", "media"); e != nil {
		t.Fatal(e)
	}
	if id, err := s.OpenedOnGeneration(generation, "old", "old", "1", "fixture", "media"); err != nil || id != "" {
		t.Fatal("retired resolve saved history")
	}
	mutations := []func() error{
		func() error { return s.SetMany(Obj{"save_history": false}) },
		func() error { _, err := s.Toggle(Obj{"id": "stale"}); return err },
		func() error { _, err := s.Opened("old", "old", "1", "fixture", "media"); return err },
		func() error { return s.Progress("old", "browser_time_advanced", 12) },
		func() error { return s.ClearHistory() },
		func() error { return s.Bind("old", "1", "fixture", "https://example.org") },
	}
	for i, mutate := range mutations {
		if mutate() == nil {
			t.Fatalf("retired mutation %d accepted", i)
		}
	}
	reopened, e := newStore(dir)
	if e != nil || len(reopened.History()) != 1 || sget(reopened.History()[0], "film_id") != "new" {
		t.Fatal("retired handler overwrote replacement data")
	}
}

type nativeFixtureFetcher map[string]string

func (f nativeFixtureFetcher) Fetch(_ context.Context, u string, _ int) ([]byte, Obj, error) {
	if b, ok := f[u]; ok {
		return []byte(b), Obj{"content_type": "text/html"}, nil
	}
	return nil, nil, problem("fixture missing", "fixture", 502)
}
func (f nativeFixtureFetcher) JSON(context.Context, string, Obj) (Obj, error) {
	return nil, problem("fixture unused", "fixture", 502)
}
func (f nativeFixtureFetcher) Probe(context.Context, string, string) Obj { return Obj{} }

func TestNativeStaticPlaybackAndDynamicRanking(t *testing.T) {
	ctx := context.Background()
	f := nativeFixtureFetcher{
		"https://player.example.org/static":    `<html><video src="https://media.example.org/video.webm"></video></html>`,
		"https://player.example.org/frame":     `<html><iframe src="/static"></iframe></html>`,
		"https://player.example.org/dynamic":   `<html><title>动态播放器</title><body>播放器仅在脚本运行后获得视频</body></html>`,
		"https://media.example.org/video.webm": string([]byte{0x1a, 0x45, 0xdf, 0xa3, 1, 2, 3, 4, 5, 6, 7, 8}),
	}
	for _, page := range []string{"static", "frame"} {
		media, e := resolveRuleMedia(ctx, f, "https://player.example.org/"+page, KazumiRule{})
		if e != nil || media != "https://media.example.org/video.webm" {
			t.Fatalf("static resolver failed %s: %v", page, e)
		}
	}
	if media, e := resolveRuleMedia(ctx, f, "https://player.example.org/dynamic", KazumiRule{}); e == nil || media != "" {
		t.Fatal("dynamic page falsely marked as a stream")
	}
	s, e := newService(t.TempDir(), f)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.store.SetMany(Obj{"network_consent": true}); e != nil {
		t.Fatal(e)
	}
	n := &availabilityNetwork{checker: newAvailabilityChecker(s.manager), underlying: f}
	state, e := probeAvailabilityEpisode(ctx, n, "https://player.example.org/static", nil)
	if e != nil || state != "media_ok" {
		t.Fatalf("static media rejected: %s %v", state, e)
	}
	state, e = probeAvailabilityEpisode(ctx, n, "https://player.example.org/dynamic", nil)
	if e == nil || state != "failed" {
		t.Fatal("page-only source ranks as playable in native app")
	}
}
