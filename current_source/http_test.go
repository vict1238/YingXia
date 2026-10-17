// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func httpFixture(t *testing.T) *Server {
	t.Helper()
	s, _ := fixtureService(t)
	srv, e := newServer(s, 0)
	mustNoErr(t, e)
	srv.files = fixtureFiles{srv.files}
	t.Cleanup(func() { srv.listener.Close() })
	return srv
}
func perform(s *Server, path, method string, body any, auth bool, extra map[string]string) *httptest.ResponseRecorder {
	var raw []byte
	if body != nil {
		raw, _ = json.Marshal(body)
	}
	r := httptest.NewRequest(method, s.origin+path, bytes.NewReader(raw))
	if auth {
		r.AddCookie(&http.Cookie{Name: s.cookieName, Value: s.session})
		r.Header.Set("X-YingXia-CSRF", s.csrf)
		r.Header.Set("Origin", s.origin)
	}
	if method == "POST" {
		r.Header.Set("Content-Type", "application/json")
	}
	for k, v := range extra {
		if k == "Host" {
			r.Host = v
		} else {
			r.Header.Set(k, v)
		}
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}
func TestHTTP(t *testing.T) {
	cases := []struct {
		name, path, method string
		body               any
		auth               bool
		headers            map[string]string
		status             int
	}{
		{"root_cookie", "/", "GET", nil, false, nil, 200}, {"root_csp", "/", "GET", nil, false, nil, 200}, {"api_needs_session", "/api/state", "GET", nil, false, nil, 403}, {"bootstrap_needs_session", "/bootstrap.js", "GET", nil, false, nil, 403}, {"state", "/api/state", "GET", nil, true, nil, 200}, {"host_attack", "/", "GET", nil, false, map[string]string{"Host": "attacker.example"}, 403}, {"origin_attack", "/api/state", "GET", nil, true, map[string]string{"Origin": "https://attacker.example"}, 403}, {"crosssite_fetch", "/", "GET", nil, false, map[string]string{"Sec-Fetch-Site": "cross-site"}, 403}, {"csrf_missing", "/api/search", "POST", Obj{"demo": true}, true, map[string]string{"X-YingXia-CSRF": ""}, 403}, {"demo_removed", "/api/search", "POST", Obj{"demo": true}, true, nil, 400}, {"bad_json_array", "/api/search", "POST", []int{1}, true, nil, 400}, {"traversal", "/%2e%2e/service.go", "GET", nil, true, nil, 404}, {"media_range", "/demo/clip1.webm", "GET", nil, true, map[string]string{"Range": "bytes=0-99"}, 206}, {"invalid_range", "/demo/clip1.webm", "GET", nil, true, map[string]string{"Range": "bytes=999999999-"}, 416}, {"unknown_api", "/api/nope", "GET", nil, true, nil, 404}, {"content_type", "/api/search", "POST", Obj{"demo": true}, true, map[string]string{"Content-Type": "text/plain"}, 415}, {"options", "/api/state", "OPTIONS", nil, true, nil, 405}, {"detail_demo", "/api/detail?id=demo-orbit-1", "GET", nil, true, nil, 200}, {"bindings", "/api/bindings?id=x&episode=a", "GET", nil, true, nil, 200}, {"report", "/api/report", "GET", nil, true, nil, 200}, {"ping", "/api/ping", "POST", Obj{}, true, nil, 200}, {"resolve_demo", "/api/resolve", "POST", demoPayload(), true, nil, 200}, {"favorite_demo", "/api/favorite", "POST", Obj{"film_id": "demo-orbit-1"}, true, nil, 200}, {"clear_history", "/api/clear-history", "POST", Obj{}, true, nil, 200}, {"match_missing", "/api/match", "POST", Obj{"film_id": "demo-orbit-1", "group_key": "g1", "match_name": "第02集"}, true, nil, 400}, {"consent_search", "/api/search", "POST", Obj{"query": "Example"}, true, nil, 403},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := httpFixture(t)
			w := perform(s, c.path, c.method, c.body, c.auth, c.headers)
			if w.Code != c.status {
				t.Fatalf("status=%d expected=%d body=%s", w.Code, c.status, w.Body.String())
			}
			if c.name == "root_cookie" {
				v := w.Header().Get("Set-Cookie")
				require(t, strings.Contains(v, "HttpOnly") && strings.Contains(v, "SameSite=Strict"), v)
			}
			if c.name == "root_csp" {
				v := w.Header().Get("Content-Security-Policy")
				require(t, strings.Contains(v, "frame-ancestors 'none'") && !strings.Contains(v, "unsafe-eval"), v)
			}
			if c.name == "media_range" {
				require(t, w.Body.Len() == 100 && strings.HasPrefix(w.Header().Get("Content-Range"), "bytes 0-99/"), "bad media range")
			}
		})
	}
}
func TestRealSocketStartAndShutdown(t *testing.T) {
	s := httpFixture(t)
	ch := make(chan error, 1)
	go func() { ch <- s.HTTP.Serve(s.listener) }()
	client := http.Client{Timeout: 5 * time.Second}
	r, e := client.Get(s.origin + "/")
	mustNoErr(t, e)
	io.Copy(io.Discard, r.Body)
	r.Body.Close()
	require(t, r.StatusCode == 200, "root failed")
	body := strings.NewReader("{}")
	req, _ := http.NewRequest("POST", s.origin+"/api/quit", body)
	req.AddCookie(&http.Cookie{Name: s.cookieName, Value: s.session})
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", s.origin)
	req.Header.Set("X-YingXia-CSRF", s.csrf)
	r, e = client.Do(req)
	mustNoErr(t, e)
	b, _ := io.ReadAll(r.Body)
	r.Body.Close()
	require(t, r.StatusCode == 200, string(b))
	select {
	case e := <-ch:
		require(t, e == http.ErrServerClosed, fmt.Sprint(e))
	case <-time.After(7 * time.Second):
		t.Fatal("did not shut down")
	}
}
func TestOfflineBrowserBridge(t *testing.T) {
	s := httpFixture(t)
	checks := []string{"/", "/app.js", "/app.css", "/demo/clip1.webm", "/demo/clip2.webm"}
	for _, p := range checks {
		w := perform(s, p, "GET", nil, true, nil)
		require(t, w.Code == 200 && w.Body.Len() > 0, p)
	}
	w := perform(s, "/bootstrap.js", "GET", nil, true, nil)
	require(t, w.Code == 200 && strings.Contains(w.Body.String(), s.csrf), "bootstrap failed")
}
