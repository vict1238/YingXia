// SPDX-License-Identifier: GPL-3.0-or-later
package engine

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
)

const mediaCacheBudget = 24 * 1024 * 1024
const mediaObjectLimit = 8 * 1024 * 1024
const mediaCacheTTL = 90 * time.Second

type mediaGrant struct {
	URL       string
	Headers   map[string]string
	Expires   time.Time
	Segment   bool
	ByteRange string
}

// MediaRelay holds only a small, volatile forward buffer. Neither manifests,
// encryption keys, nor progressive movies are cached or downloaded in advance.
type MediaRelay struct {
	mu             sync.Mutex
	grants         map[string]mediaGrant
	keys           map[string]string
	net            Fetcher
	ctx            context.Context
	cancel         context.CancelFunc
	prefetchCancel context.CancelFunc
	cache          map[string]*mediaCacheEntry
	flights        map[string]*mediaFlight
	plans          map[string][]string
	slots          chan struct{}
	cacheBytes     int
	adSkip         bool
	now            func() time.Time
	grantPruned    time.Time
	stats          mediaStats
}
type mediaStats struct {
	hits, misses, retries, prefetched, skipped, playlists, deferred, failures uint64
	skippedSeconds                                                            float64
}
type mediaCacheEntry struct {
	body          []byte
	header        http.Header
	status        int
	expires, used time.Time
}
type mediaFlight struct {
	done   chan struct{}
	cancel context.CancelFunc
	refs   int
	entry  *mediaCacheEntry
	err    error
}

var errMediaOversize = errors.New("segment exceeds forward buffer")

func newMediaRelay(n Fetcher) *MediaRelay {
	ctx, cancel := context.WithCancel(context.Background())
	return &MediaRelay{grants: map[string]mediaGrant{}, keys: map[string]string{}, net: n, ctx: ctx, cancel: cancel, cache: map[string]*mediaCacheEntry{}, flights: map[string]*mediaFlight{}, plans: map[string][]string{}, slots: make(chan struct{}, 2), adSkip: true, now: time.Now}
}
func (m *MediaRelay) setAdSkip(enabled bool) { m.mu.Lock(); m.adSkip = enabled; m.mu.Unlock() }
func (m *MediaRelay) close() {
	m.cancel()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.prefetchCancel != nil {
		m.prefetchCancel()
	}
	for _, f := range m.flights {
		f.cancel()
	}
	m.cache = map[string]*mediaCacheEntry{}
	m.cacheBytes = 0
}
func (m *MediaRelay) diagnostics() Obj {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pruneLocked()
	return Obj{"skip_marked_ads": m.adSkip, "cache_bytes": m.cacheBytes, "cache_budget_bytes": mediaCacheBudget, "cache_disk_bytes": 0, "cache_object_limit_bytes": mediaObjectLimit, "cache_ttl_seconds": int(mediaCacheTTL / time.Second), "prefetch_segments": 2, "cache_hits": m.stats.hits, "cache_misses": m.stats.misses, "retry_count": m.stats.retries, "prefetched_segments": m.stats.prefetched, "skipped_ad_segments": m.stats.skipped, "skipped_ad_seconds": m.stats.skippedSeconds, "rewritten_playlists": m.stats.playlists, "ad_skip_deferred_playlists": m.stats.deferred, "fetch_failures": m.stats.failures, "inflight_segments": len(m.flights)}
}
func (m *MediaRelay) pruneLocked() {
	now := m.now()
	for k, e := range m.cache {
		if !now.Before(e.expires) {
			m.cacheBytes -= len(e.body)
			delete(m.cache, k)
		}
	}
	if !m.grantPruned.IsZero() && now.Sub(m.grantPruned) < time.Minute {
		return
	}
	m.grantPruned = now
	for id, g := range m.grants {
		if !now.Before(g.Expires) {
			delete(m.grants, id)
			delete(m.plans, id)
		}
	}
	for k, id := range m.keys {
		if _, ok := m.grants[id]; !ok {
			delete(m.keys, k)
		}
	}
}
func mediaHeaders(headers map[string]string) map[string]string {
	h := map[string]string{}
	for k, v := range headers {
		if (strings.EqualFold(k, "Referer") || strings.EqualFold(k, "User-Agent")) && !hasControl(v) && len(v) < 4096 {
			h[http.CanonicalHeaderKey(k)] = v
		}
	}
	return h
}
func mediaIdentity(g mediaGrant, byteRange string) string {
	b, _ := json.Marshal(Obj{"url": g.URL, "headers": g.Headers, "range": byteRange})
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}
func (m *MediaRelay) register(link string, headers map[string]string) (string, error) {
	return m.registerMedia(link, headers, false, "")
}
func (m *MediaRelay) registerMedia(link string, headers map[string]string, segment bool, byteRange string) (string, error) {
	clean, e := publicURL(link)
	if e != nil {
		return "", e
	}
	g := mediaGrant{URL: clean, Headers: mediaHeaders(headers), Segment: segment, ByteRange: byteRange}
	key := mediaIdentity(g, byteRange) + ":" + strconv.FormatBool(segment)
	m.mu.Lock()
	defer m.mu.Unlock()
	// Pruning at registration also bounds reverse keys for sliding live windows.
	m.pruneLocked()
	if id, ok := m.keys[key]; ok {
		old := m.grants[id]
		old.Expires = m.now().Add(2 * time.Hour)
		m.grants[id] = old
		return "/api/media?ticket=" + id, nil
	}
	if len(m.grants) >= 8192 {
		m.grantPruned = time.Time{}
		m.pruneLocked()
	}
	if len(m.grants) >= 8192 {
		return "", problem("播放清单条目过多，请重新播放。", "hls", 502)
	}
	id := token()
	g.Expires = m.now().Add(2 * time.Hour)
	m.grants[id] = g
	m.keys[key] = id
	return "/api/media?ticket=" + id, nil
}
func (m *MediaRelay) grant(id string) (mediaGrant, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	g, ok := m.grants[id]
	if !ok || !m.now().Before(g.Expires) {
		return g, problem("播放链接已过期，请重新播放。", "media_expired", 404)
	}
	g.Expires = m.now().Add(2 * time.Hour)
	m.grants[id] = g
	return g, nil
}

type mediaOpener interface {
	Open(context.Context, RequestSpec) (*http.Response, error)
}

func transientMediaError(e error) bool {
	if e == nil || errors.Is(e, context.Canceled) || errors.Is(e, context.DeadlineExceeded) {
		return false
	}
	var a *AppError
	if errors.As(e, &a) {
		if a.Code == "network" || a.Code == "timeout" || a.Code == "dns" {
			return true
		}
		if a.Code == "http" {
			for _, code := range []string{"408", "429", "500", "502", "503", "504"} {
				if strings.HasSuffix(a.Message, "HTTP "+code) {
					return true
				}
			}
		}
		return false
	}
	var networkError net.Error
	if errors.As(e, &networkError) {
		return true
	}
	return errors.Is(e, io.ErrUnexpectedEOF) || errors.Is(e, io.EOF)
}
func (m *MediaRelay) retryPause(ctx context.Context, attempt int) error {
	t := time.NewTimer(time.Duration(attempt+1) * 120 * time.Millisecond)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
	}
	m.mu.Lock()
	m.stats.retries++
	m.mu.Unlock()
	return nil
}
func (m *MediaRelay) openOnce(ctx context.Context, g mediaGrant, byteRange string) (*http.Response, error) {
	h := mediaHeaders(g.Headers)
	if byteRange != "" {
		h["Range"] = byteRange
	}
	spec := RequestSpec{URL: g.URL, Method: "GET", Headers: h, Limit: 32000000}
	if o, ok := m.net.(mediaOpener); ok {
		return o.Open(ctx, spec)
	}
	b, meta, e := fetchRequest(ctx, m.net, spec)
	if e != nil {
		return nil, e
	}
	u, _ := url.Parse(g.URL)
	res := &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(b)), ContentLength: int64(len(b)), Request: &http.Request{URL: u}}
	res.Header.Set("Content-Type", sget(meta, "content_type"))
	res.Header.Set("Content-Length", strconv.Itoa(len(b)))
	return res, nil
}
func (m *MediaRelay) open(ctx context.Context, g mediaGrant, byteRange string) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		res, e := m.openOnce(ctx, g, byteRange)
		if e == nil && (res == nil || res.Body == nil) {
			e = problem("媒体响应为空。", "network", 502)
		}
		if e == nil && res != nil && (res.StatusCode < 200 || res.StatusCode >= 300) {
			res.Body.Close()
			e = problem("来源返回 HTTP "+strconv.Itoa(res.StatusCode), "http", 502)
		}
		if e == nil {
			return res, nil
		}
		if attempt >= 2 || !transientMediaError(e) {
			return nil, e
		}
		if e = m.retryPause(ctx, attempt); e != nil {
			return nil, e
		}
	}
}
func validMediaRange(raw string) bool {
	if !strings.HasPrefix(raw, "bytes=") {
		return false
	}
	a, b, ok := strings.Cut(strings.TrimPrefix(raw, "bytes="), "-")
	if !ok || (a == "" && b == "") {
		return false
	}
	parse := func(s string) (int64, bool) {
		if s == "" {
			return 0, true
		}
		for _, c := range s {
			if c < '0' || c > '9' {
				return 0, false
			}
		}
		n, e := strconv.ParseInt(s, 10, 64)
		return n, e == nil
	}
	x, xok := parse(a)
	y, yok := parse(b)
	return xok && yok && (a == "" && y > 0 || a != "" && (b == "" || x <= y))
}
func (m *MediaRelay) serve(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("ticket")
	g, e := m.grant(id)
	if e != nil {
		sendError(w, e)
		return
	}
	ctx, cancel := context.WithCancel(r.Context())
	stop := context.AfterFunc(m.ctx, cancel)
	defer stop()
	defer cancel()
	r = r.WithContext(ctx)
	// The authenticated media path may stream for longer than the API's
	// write timeout. Body inactivity is bounded separately below.
	_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
	byteRange := r.Header.Get("Range")
	if byteRange == "" && g.ByteRange != "" {
		byteRange = g.ByteRange
	}
	if byteRange != "" && !validMediaRange(byteRange) {
		sendError(w, problem("分段请求无效。", "range", 416))
		return
	}
	if g.Segment && r.Method != "HEAD" {
		entry, err := m.cached(r.Context(), g, byteRange)
		if err == nil {
			m.prefetch(id)
			m.writeMedia(w, r, entry.header, entry.status, bytes.NewReader(entry.body))
			return
		}
		if !errors.Is(err, errMediaOversize) {
			sendError(w, mediaReadError(err))
			return
		}
	}
	// A progressive response has no absolute 85-second/512-MiB cutoff. The
	// transport still bounds connection/header setup; client cancellation ends it.
	res, e := m.open(r.Context(), g, byteRange)
	if e != nil {
		sendError(w, mediaReadError(e))
		return
	}
	res.Body = idleMediaBody(res.Body)
	defer res.Body.Close()
	reader := bufio.NewReader(res.Body)
	prefix, _ := reader.Peek(16)
	ct := strings.ToLower(res.Header.Get("Content-Type"))
	u, _ := url.Parse(g.URL)
	if strings.Contains(ct, "mpegurl") || strings.EqualFold(path.Ext(u.Path), ".m3u8") || strings.HasPrefix(strings.TrimSpace(string(prefix)), "#EXTM3U") {
		body, e := io.ReadAll(io.LimitReader(reader, 4000001))
		if e != nil {
			sendError(w, mediaReadError(e))
			return
		}
		base := g.URL
		if res.Request != nil && res.Request.URL != nil && res.Request.URL.Host != "" {
			base = res.Request.URL.String()
		}
		body, e = m.rewritePlaylist(body, base, g.Headers)
		if e != nil {
			sendError(w, e)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(200)
		if r.Method != "HEAD" {
			_, _ = w.Write(body)
		}
		return
	}
	if mediaHTML(ct, prefix) {
		sendError(w, problem("来源返回网页，无法作为媒体播放。", "not_media", 502))
		return
	}
	if res.StatusCode == 206 && res.ContentLength >= 0 && !validMediaContentRange(res.Header.Get("Content-Range"), byteRange, res.ContentLength) {
		sendError(w, problem("来源分段响应无效。", "range", 502))
		return
	}
	m.writeMedia(w, r, res.Header, res.StatusCode, reader)
}
func mediaHTML(ct string, prefix []byte) bool {
	s := strings.ToLower(strings.TrimSpace(string(prefix)))
	return strings.Contains(ct, "text/html") || strings.HasPrefix(s, "<!doctype") || strings.HasPrefix(s, "<html")
}
func mediaReadError(e error) error {
	var a *AppError
	if errors.As(e, &a) {
		return e
	}
	return problem("媒体读取中断，请重试或换来源。", "network", 502)
}
func (m *MediaRelay) writeMedia(w http.ResponseWriter, r *http.Request, h http.Header, status int, body io.Reader) {
	ct := strings.ToLower(h.Get("Content-Type"))
	if !strings.HasPrefix(ct, "video/") && !strings.HasPrefix(ct, "audio/") {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	for _, key := range []string{"Content-Length", "Content-Range", "Accept-Ranges"} {
		if v := h.Get(key); v != "" {
			w.Header().Set(key, v)
		}
	}
	w.WriteHeader(status)
	if r.Method != "HEAD" {
		_, err := io.Copy(w, body)
		if err != nil && r.Context().Err() == nil {
			m.mu.Lock()
			m.stats.failures++
			m.mu.Unlock()
		}
	}
}
