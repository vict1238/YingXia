// SPDX-License-Identifier: GPL-3.0-or-later
package engine

import (
	"context"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"
)

func (m *MediaRelay) cached(ctx context.Context, g mediaGrant, byteRange string) (*mediaCacheEntry, error) {
	entry, err := m.cachedOnce(ctx, g, byteRange)
	// A newly arriving demand may briefly join a prefetch whose final waiter
	// just canceled. Retry that abandoned flight once with the demand context.
	if errors.Is(err, context.Canceled) && ctx.Err() == nil && m.ctx.Err() == nil {
		return m.cachedOnce(ctx, g, byteRange)
	}
	return entry, err
}
func (m *MediaRelay) cachedOnce(ctx context.Context, g mediaGrant, byteRange string) (*mediaCacheEntry, error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	if e := m.ctx.Err(); e != nil {
		return nil, e
	}
	key := mediaIdentity(g, byteRange)
	m.mu.Lock()
	m.pruneLocked()
	if entry := m.cache[key]; entry != nil {
		entry.used = m.now()
		m.stats.hits++
		m.mu.Unlock()
		return entry, nil
	}
	f := m.flights[key]
	if f == nil {
		// Independent callers can share a fetch; the last departing waiter
		// cancels it. Prefetch cannot keep an abandoned request alive forever.
		loadCtx, cancel := context.WithTimeout(m.ctx, 35*time.Second)
		f = &mediaFlight{done: make(chan struct{}), cancel: cancel}
		m.flights[key] = f
		m.stats.misses++
		go m.loadFlight(loadCtx, key, g, byteRange, f)
	}
	f.refs++
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		f.refs--
		if f.refs == 0 {
			f.cancel()
		}
		m.mu.Unlock()
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-f.done:
		return f.entry, f.err
	}
}
func (m *MediaRelay) loadFlight(ctx context.Context, key string, g mediaGrant, byteRange string, f *mediaFlight) {
	defer f.cancel()
	var entry *mediaCacheEntry
	var err error
	select {
	case m.slots <- struct{}{}:
		entry, err = m.fetchBuffered(ctx, g, byteRange)
		<-m.slots
	case <-ctx.Done():
		err = ctx.Err()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err == nil && entry != nil && ctx.Err() == nil && !strings.Contains(strings.ToLower(entry.header.Get("Cache-Control")), "no-store") {
		m.pruneLocked()
		for m.cacheBytes+len(entry.body) > mediaCacheBudget {
			oldest := ""
			var when time.Time
			for k, e := range m.cache {
				if oldest == "" || e.used.Before(when) {
					oldest = k
					when = e.used
				}
			}
			if oldest == "" {
				break
			}
			m.cacheBytes -= len(m.cache[oldest].body)
			delete(m.cache, oldest)
		}
		entry.expires = m.now().Add(mediaCacheTTL)
		entry.used = m.now()
		m.cache[key] = entry
		m.cacheBytes += len(entry.body)
	}
	if err != nil && !errors.Is(err, errMediaOversize) && !errors.Is(err, context.Canceled) {
		m.stats.failures++
	}
	f.entry, f.err = entry, err
	delete(m.flights, key)
	close(f.done)
}
func (m *MediaRelay) fetchBuffered(ctx context.Context, g mediaGrant, byteRange string) (*mediaCacheEntry, error) {
	// All three attempts include the full body, so an incomplete segment is
	// never published or cached. No bytes have reached the decoder yet.
	for attempt := 0; ; attempt++ {
		res, err := m.openOnce(ctx, g, byteRange)
		if err == nil && (res == nil || res.Body == nil) {
			err = problem("媒体响应为空。", "network", 502)
		}
		var entry *mediaCacheEntry
		if err == nil && res != nil {
			if res.StatusCode < 200 || res.StatusCode >= 300 {
				err = problem("来源返回 HTTP "+strconv.Itoa(res.StatusCode), "http", 502)
			} else if res.ContentLength > mediaObjectLimit {
				err = errMediaOversize
			} else {
				var body []byte
				body, err = io.ReadAll(io.LimitReader(res.Body, mediaObjectLimit+1))
				if err == nil && len(body) > mediaObjectLimit {
					err = errMediaOversize
				}
				if err == nil {
					if declared := res.Header.Get("Content-Length"); declared != "" {
						n, e := strconv.ParseInt(declared, 10, 64)
						if e != nil || n != int64(len(body)) {
							err = io.ErrUnexpectedEOF
						}
					}
					if res.ContentLength >= 0 && res.ContentLength != int64(len(body)) {
						err = io.ErrUnexpectedEOF
					}
					if res.StatusCode == 206 && !validMediaContentRange(res.Header.Get("Content-Range"), byteRange, int64(len(body))) {
						err = problem("来源分段响应无效。", "range", 502)
					}
					if mediaHTML(strings.ToLower(res.Header.Get("Content-Type")), body[:min(len(body), 32)]) {
						err = problem("来源返回网页，无法作为媒体播放。", "not_media", 502)
					}
					if err == nil {
						h := res.Header.Clone()
						h.Set("Content-Length", strconv.Itoa(len(body)))
						entry = &mediaCacheEntry{body: body, header: h, status: res.StatusCode}
					}
				}
			}
			res.Body.Close()
		}
		if err == nil {
			return entry, nil
		}
		if attempt >= 2 || !transientMediaError(err) {
			return nil, err
		}
		if err = m.retryPause(ctx, attempt); err != nil {
			return nil, err
		}
	}
}
func validMediaContentRange(raw, requested string, size int64) bool {
	if !strings.HasPrefix(raw, "bytes ") {
		return false
	}
	p, total, ok := strings.Cut(strings.TrimPrefix(raw, "bytes "), "/")
	if !ok {
		return false
	}
	first, last, ok := strings.Cut(p, "-")
	if !ok {
		return false
	}
	a, e := strconv.ParseInt(first, 10, 64)
	b, f := strconv.ParseInt(last, 10, 64)
	if e != nil || f != nil || a < 0 || b < a || b-a+1 != size {
		return false
	}
	if total != "*" {
		n, e := strconv.ParseInt(total, 10, 64)
		if e != nil || n <= b {
			return false
		}
	}
	if requested == "" {
		return false
	}
	x, y, ok := strings.Cut(strings.TrimPrefix(requested, "bytes="), "-")
	if !ok {
		return false
	}
	if x != "" {
		n, e := strconv.ParseInt(x, 10, 64)
		if e != nil || n != a {
			return false
		}
		if y != "" {
			end, e := strconv.ParseInt(y, 10, 64)
			if e != nil || b > end {
				return false
			}
		}
	} else if y != "" {
		n, e := strconv.ParseInt(y, 10, 64)
		if e != nil || size > n {
			return false
		}
		if total != "*" {
			length, _ := strconv.ParseInt(total, 10, 64)
			if b != length-1 {
				return false
			}
		}
	}
	return true
}
func mediaTicket(link string) string { _, id, _ := strings.Cut(link, "ticket="); return id }
func (m *MediaRelay) prefetch(id string) {
	m.mu.Lock()
	if m.prefetchCancel != nil {
		m.prefetchCancel()
	}
	ctx, cancel := context.WithTimeout(m.ctx, 12*time.Second)
	m.prefetchCancel = cancel
	ids := append([]string(nil), m.plans[id]...)
	m.mu.Unlock()
	if len(ids) == 0 {
		cancel()
		return
	}
	// Exactly two successors at most, only after an actual media demand.
	// Prefetch itself never advances this window.
	for _, next := range ids {
		go func(next string) {
			g, e := m.grant(next)
			if e != nil {
				return
			}
			if _, e = m.cached(ctx, g, g.ByteRange); e == nil {
				m.mu.Lock()
				m.stats.prefetched++
				m.mu.Unlock()
			}
		}(next)
	}
}

// A stalled progressive body is closed after idle time, while a healthy long
// transfer may run for its natural duration. No attempt replays delivered bytes.
type mediaIdleBody struct {
	io.ReadCloser
	timer *time.Timer
}

func idleMediaBody(body io.ReadCloser) *mediaIdleBody {
	return &mediaIdleBody{ReadCloser: body, timer: time.AfterFunc(30*time.Second, func() { _ = body.Close() })}
}
func (b *mediaIdleBody) Read(p []byte) (int, error) {
	n, e := b.ReadCloser.Read(p)
	if n > 0 {
		b.timer.Reset(30 * time.Second)
	}
	return n, e
}
func (b *mediaIdleBody) Close() error { b.timer.Stop(); return b.ReadCloser.Close() }
