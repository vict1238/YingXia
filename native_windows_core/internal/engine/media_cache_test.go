// SPDX-License-Identifier: GPL-3.0-or-later
package engine

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type relayTestNetwork struct {
	open func(context.Context, RequestSpec) (*http.Response, error)
}

func (n *relayTestNetwork) Open(ctx context.Context, s RequestSpec) (*http.Response, error) {
	return n.open(ctx, s)
}
func (n *relayTestNetwork) Fetch(context.Context, string, int) ([]byte, Obj, error) {
	panic("unexpected Fetch")
}
func (n *relayTestNetwork) JSON(context.Context, string, Obj) (Obj, error) { panic("unexpected JSON") }
func (n *relayTestNetwork) Probe(context.Context, string, string) Obj      { panic("unexpected Probe") }
func relayResponse(s RequestSpec, status int, body []byte) *http.Response {
	u, _ := url.Parse(s.URL)
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"video/mp2t"}, "Content-Length": []string{strconv.Itoa(len(body))}}, ContentLength: int64(len(body)), Body: io.NopCloser(bytes.NewReader(body)), Request: &http.Request{URL: u}}
}
func relayGrantFor(t *testing.T, m *MediaRelay, link string) mediaGrant {
	t.Helper()
	ticket, e := m.registerMedia(link, nil, true, "")
	if e != nil {
		t.Fatal(e)
	}
	g, e := m.grant(mediaTicket(ticket))
	if e != nil {
		t.Fatal(e)
	}
	return g
}

func TestRelayCacheDedupCancellationAndIsolation(t *testing.T) {
	var opens atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	n := &relayTestNetwork{open: func(ctx context.Context, s RequestSpec) (*http.Response, error) {
		if opens.Add(1) == 1 {
			close(started)
		}
		select {
		case <-release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return relayResponse(s, 200, []byte("a complete segment")), nil
	}}
	m := newMediaRelay(n)
	defer m.close()
	g := relayGrantFor(t, m, "https://media.example.org/a.ts?signature=a")
	results := make(chan error, 2)
	go func() { _, e := m.cached(context.Background(), g, ""); results <- e }()
	<-started
	go func() { _, e := m.cached(context.Background(), g, ""); results <- e }()
	deadline := time.Now().Add(time.Second)
	for {
		m.mu.Lock()
		refs := m.flights[mediaIdentity(g, "")].refs
		m.mu.Unlock()
		if refs == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("second reader did not join")
		}
		time.Sleep(time.Millisecond)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := m.cached(ctx, g, ""); !errors.Is(e, context.Canceled) {
		t.Fatal("cancel ignored")
	}
	close(release)
	for i := 0; i < 2; i++ {
		if e := <-results; e != nil {
			t.Fatal(e)
		}
	}
	if opens.Load() != 1 {
		t.Fatalf("duplicate loads %d", opens.Load())
	}
	if _, e := m.cached(context.Background(), g, ""); e != nil || opens.Load() != 1 {
		t.Fatal("cache miss after complete load")
	}
	copyGrant := g
	copyGrant.Headers = map[string]string{"Referer": "https://other.example.org/"}
	if _, e := m.cached(context.Background(), copyGrant, ""); e != nil {
		t.Fatal(e)
	}
	copyGrant = g
	copyGrant.URL = "https://media.example.org/a.ts?signature=b"
	if _, e := m.cached(context.Background(), copyGrant, ""); e != nil {
		t.Fatal(e)
	}
	if opens.Load() != 3 {
		t.Fatal("URL credentials or headers leaked across cache identities")
	}
}

type relayContextBody struct {
	ctx    context.Context
	closed chan struct{}
	once   sync.Once
}

func (b *relayContextBody) Read([]byte) (int, error) { <-b.ctx.Done(); return 0, b.ctx.Err() }
func (b *relayContextBody) Close() error             { b.once.Do(func() { close(b.closed) }); return nil }
func TestRelayCancelStopsUnusedFlight(t *testing.T) {
	started := make(chan struct{})
	closed := make(chan struct{})
	n := &relayTestNetwork{open: func(ctx context.Context, s RequestSpec) (*http.Response, error) {
		close(started)
		r := relayResponse(s, 200, nil)
		r.ContentLength = -1
		r.Header.Del("Content-Length")
		r.Body = &relayContextBody{ctx: ctx, closed: closed}
		return r, nil
	}}
	m := newMediaRelay(n)
	defer m.close()
	g := relayGrantFor(t, m, "https://media.example.org/a.ts")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, e := m.cached(ctx, g, ""); done <- e }()
	<-started
	cancel()
	if e := <-done; !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("orphan fetch was not canceled")
	}
}
func TestRelayRetriesWholeSegmentsBeforePublishing(t *testing.T) {
	var attempts atomic.Int32
	n := &relayTestNetwork{open: func(ctx context.Context, s RequestSpec) (*http.Response, error) {
		a := attempts.Add(1)
		if a == 1 {
			return relayResponse(s, 503, []byte("busy")), nil
		}
		r := relayResponse(s, 200, []byte("complete"))
		if a == 2 {
			r.Body = io.NopCloser(strings.NewReader("part"))
		}
		return r, nil
	}}
	m := newMediaRelay(n)
	defer m.close()
	g := relayGrantFor(t, m, "https://media.example.org/a.ts")
	entry, e := m.cached(context.Background(), g, "")
	if e != nil || string(entry.body) != "complete" || attempts.Load() != 3 {
		t.Fatalf("attempts %d entry %v error %v", attempts.Load(), entry, e)
	}
	if m.stats.retries != 2 {
		t.Fatalf("retry diagnostics %d", m.stats.retries)
	}
	attempts.Store(0)
	n.open = func(ctx context.Context, s RequestSpec) (*http.Response, error) {
		attempts.Add(1)
		return relayResponse(s, 404, []byte("gone")), nil
	}
	g.URL = "https://media.example.org/b.ts"
	if _, e = m.cached(context.Background(), g, ""); e == nil || attempts.Load() != 1 {
		t.Fatal("permanent HTTP failure retried")
	}
}
func TestRelayCacheBudgetTTLAndNoStore(t *testing.T) {
	var opens atomic.Int32
	n := &relayTestNetwork{open: func(ctx context.Context, s RequestSpec) (*http.Response, error) {
		opens.Add(1)
		return relayResponse(s, 200, make([]byte, mediaObjectLimit)), nil
	}}
	m := newMediaRelay(n)
	defer m.close()
	now := time.Now()
	m.now = func() time.Time { return now }
	for i := 0; i < 4; i++ {
		g := relayGrantFor(t, m, "https://media.example.org/"+strconv.Itoa(i)+".ts")
		if _, e := m.cached(context.Background(), g, ""); e != nil {
			t.Fatal(e)
		}
		now = now.Add(time.Millisecond)
	}
	if m.cacheBytes > mediaCacheBudget || len(m.cache) != 3 {
		t.Fatalf("cache exceeded budget: %d %d", m.cacheBytes, len(m.cache))
	}
	now = now.Add(mediaCacheTTL + time.Second)
	d := m.diagnostics()
	if d["cache_bytes"] != 0 || len(m.cache) != 0 {
		t.Fatal("expired segments retained")
	}
	n.open = func(ctx context.Context, s RequestSpec) (*http.Response, error) {
		r := relayResponse(s, 200, []byte("secret"))
		r.Header.Set("Cache-Control", "private, no-store")
		return r, nil
	}
	g := relayGrantFor(t, m, "https://media.example.org/private.ts")
	if _, e := m.cached(context.Background(), g, ""); e != nil || m.cacheBytes != 0 {
		t.Fatal("no-store response retained")
	}
}
func TestRelayRangeAndHeadResponses(t *testing.T) {
	var opens atomic.Int32
	n := &relayTestNetwork{open: func(ctx context.Context, s RequestSpec) (*http.Response, error) {
		opens.Add(1)
		r := relayResponse(s, 206, []byte("2345"))
		if s.Headers["Range"] != "bytes=2-5" {
			t.Errorf("range not forwarded: %v", s.Headers)
		}
		r.Header.Set("Content-Range", "bytes 2-5/10")
		r.Header.Set("Accept-Ranges", "bytes")
		return r, nil
	}}
	m := newMediaRelay(n)
	defer m.close()
	ticket, e := m.registerMedia("https://media.example.org/movie.ts", nil, true, "bytes=2-5")
	if e != nil {
		t.Fatal(e)
	}
	for _, method := range []string{"GET", "GET", "HEAD"} {
		req := httptest.NewRequest(method, ticket, nil)
		w := httptest.NewRecorder()
		m.serve(w, req)
		if w.Code != 206 || w.Header().Get("Content-Range") != "bytes 2-5/10" || w.Header().Get("Content-Length") != "4" {
			t.Fatalf("bad range response: %d %v %s", w.Code, w.Header(), w.Body.String())
		}
		if method == "HEAD" && w.Body.Len() != 0 {
			t.Fatal("HEAD carried a body")
		}
		if method == "GET" && w.Body.String() != "2345" {
			t.Fatal("range body changed")
		}
	}
	if opens.Load() != 2 {
		t.Fatalf("range cache/HEAD opens: %d", opens.Load())
	}
	for _, raw := range []string{"bytes=-", "bytes=5-2", "bytes=0-1,4-5", "bytes=-0", "bytes=9223372036854775809-"} {
		req := httptest.NewRequest("GET", ticket, nil)
		req.Header.Set("Range", raw)
		w := httptest.NewRecorder()
		m.serve(w, req)
		if w.Code != 416 {
			t.Fatalf("invalid range accepted: %s", raw)
		}
	}
	if validMediaContentRange("bytes 3-6/10", "bytes=2-5", 4) || validMediaContentRange("bytes 2-5/5", "bytes=2-5", 4) {
		t.Fatal("mismatched Content-Range accepted")
	}
}
func TestRelayPrefetchIsOnlyDemandDrivenWindow(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]int{}
	n := &relayTestNetwork{open: func(ctx context.Context, s RequestSpec) (*http.Response, error) {
		mu.Lock()
		seen[s.URL]++
		mu.Unlock()
		return relayResponse(s, 200, []byte("segment")), nil
	}}
	m := newMediaRelay(n)
	defer m.close()
	body, e := m.rewritePlaylist([]byte("#EXTM3U\n#EXTINF:5,\na.ts\n#EXTINF:5,\nb.ts\n#EXTINF:5,\nc.ts\n#EXTINF:5,\nd.ts\n#EXT-X-ENDLIST"), "https://media.example.org/index.m3u8", nil)
	if e != nil {
		t.Fatal(e)
	}
	mu.Lock()
	count := len(seen)
	mu.Unlock()
	if count != 0 {
		t.Fatal("manifest download triggered media prefetch")
	}
	first := ""
	for _, line := range strings.Split(string(body), "\n") {
		if strings.HasPrefix(line, "/api/media?") {
			first = line
			break
		}
	}
	w := httptest.NewRecorder()
	m.serve(w, httptest.NewRequest("GET", first, nil))
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	deadline := time.Now().Add(time.Second)
	for {
		mu.Lock()
		count = len(seen)
		mu.Unlock()
		if count == 3 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("forward prefetch count %d", count)
		}
		time.Sleep(time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if seen["https://media.example.org/d.ts"] != 0 {
		t.Fatal("prefetch recursively downloaded beyond window")
	}
}
func TestRelayOversizeSegmentDoesNotEnterCache(t *testing.T) {
	var opens atomic.Int32
	n := &relayTestNetwork{open: func(ctx context.Context, s RequestSpec) (*http.Response, error) {
		opens.Add(1)
		r := relayResponse(s, 200, nil)
		r.ContentLength = mediaObjectLimit + 1
		r.Header.Set("Content-Length", strconv.Itoa(mediaObjectLimit+1))
		return r, nil
	}}
	m := newMediaRelay(n)
	defer m.close()
	g := relayGrantFor(t, m, "https://media.example.org/large.ts")
	if _, e := m.cached(context.Background(), g, ""); !errors.Is(e, errMediaOversize) || m.cacheBytes != 0 || opens.Load() != 1 {
		t.Fatal("oversized media buffered/retried")
	}
}

type relayZeroReader struct{ remaining int64 }

func (r *relayZeroReader) Read(p []byte) (int, error) {
	if r.remaining == 0 {
		return 0, io.EOF
	}
	n := min(int64(len(p)), r.remaining)
	clear(p[:int(n)])
	r.remaining -= n
	return int(n), nil
}

type relayCountingWriter struct {
	header http.Header
	count  int64
	status int
}

func (w *relayCountingWriter) Header() http.Header { return w.header }
func (w *relayCountingWriter) WriteHeader(s int)   { w.status = s }
func (w *relayCountingWriter) Write(p []byte) (int, error) {
	w.count += int64(len(p))
	return len(p), nil
}
func TestRelayProgressiveStreamingDoesNotTruncate512MiB(t *testing.T) {
	const size int64 = 512*1024*1024 + 123
	n := &relayTestNetwork{open: func(ctx context.Context, s RequestSpec) (*http.Response, error) {
		r := relayResponse(s, 200, nil)
		r.ContentLength = size
		r.Header.Set("Content-Length", strconv.FormatInt(size, 10))
		r.Body = io.NopCloser(&relayZeroReader{remaining: size})
		return r, nil
	}}
	m := newMediaRelay(n)
	defer m.close()
	ticket, e := m.register("https://media.example.org/long.mp4", nil)
	if e != nil {
		t.Fatal(e)
	}
	w := &relayCountingWriter{header: http.Header{}}
	m.serve(w, httptest.NewRequest("GET", ticket, nil))
	if w.count != size || w.status != 200 || w.Header().Get("Content-Length") != strconv.FormatInt(size, 10) || m.cacheBytes != 0 {
		t.Fatalf("progressive transfer truncated: %d / %d, %v", w.count, size, w.Header())
	}
}
func TestRelayTransientRetryCapAndShutdown(t *testing.T) {
	var attempts atomic.Int32
	n := &relayTestNetwork{open: func(ctx context.Context, s RequestSpec) (*http.Response, error) {
		attempts.Add(1)
		return relayResponse(s, 503, []byte("busy")), nil
	}}
	m := newMediaRelay(n)
	g := relayGrantFor(t, m, "https://media.example.org/a.ts")
	if _, e := m.cached(context.Background(), g, ""); e == nil || attempts.Load() != 3 {
		t.Fatal("transient retry cap not enforced")
	}
	m.close()
	if _, e := m.cached(context.Background(), g, ""); !errors.Is(e, context.Canceled) || attempts.Load() != 3 {
		t.Fatal("shutdown permitted media fetch")
	}
}
