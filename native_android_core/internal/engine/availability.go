// SPDX-License-Identifier: GPL-3.0-or-later
package engine

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Availability is a short-lived reachability observation, not decoder evidence.
// It deliberately contains no media URL or request headers.
type Availability struct {
	State      string `json:"state"`
	Label      string `json:"label"`
	CheckedAt  string `json:"checked_at"`
	LatencyMS  int64  `json:"latency_ms"`
	Error      string `json:"error,omitempty"`
	GroupKey   string `json:"group_key,omitempty"`
	EpisodeKey string `json:"episode_key,omitempty"`
}

type availabilityEntry struct {
	value   Availability
	expires time.Time
}

type AvailabilityChecker struct {
	manager *SourceManager
	mu      sync.Mutex
	cache   map[string]availabilityEntry
	slots   chan struct{}
	now     func() time.Time
}

func newAvailabilityChecker(m *SourceManager) *AvailabilityChecker {
	return &AvailabilityChecker{manager: m, cache: map[string]availabilityEntry{}, slots: make(chan struct{}, 6), now: time.Now}
}

func availabilityPriority(a Availability) int {
	switch a.State {
	case "media_ok":
		return 0
	case "page_ok":
		return 1
	case "failed":
		return 3
	default:
		return 2
	}
}

func (c *AvailabilityChecker) allowed() bool {
	return boolSetting(c.manager.store.Settings(), "network_consent", false)
}

func (c *AvailabilityChecker) cacheKey(f Film) string {
	return availabilityCacheKey(f, c.manager.snapshot(), c.manager.store.Settings())
}

// This helper does not take manager locks, so it is safe inside cache writes.
func availabilityCacheKey(f Film, r SourceRegistry, settings Obj) string {
	pid := f.ProviderID
	if pid == "" {
		_, pid, _, _ = splitSourceFilmID(f.ID)
	}
	key := f.ID + ":"
	for _, p := range r.Providers {
		if p.ID == pid {
			key += providerFingerprint(p)
			break
		}
	}
	// A legacy player's catalog URL is outside the provider object.
	if pid == "wsyzy" || pid == "" {
		b, _ := json.Marshal(r.Catalog)
		key += string(b)
	}
	b, _ := json.Marshal(settings["enabled_providers"])
	return key + string(b)
}

func (c *AvailabilityChecker) cacheDetail(ctx context.Context, key string, film Film) bool {
	c.manager.mu.Lock()
	defer c.manager.mu.Unlock()
	settings := c.manager.store.Settings()
	if ctx.Err() != nil || !boolSetting(settings, "network_consent", false) || availabilityCacheKey(film, c.manager.state, settings) != key {
		return false
	}
	// The comparison and write share the manager lock with registry replacement.
	c.manager.cache[film.ID] = filmCache{time.Now(), film}
	return true
}

func (c *AvailabilityChecker) Check(parent context.Context, film Film) Availability {
	started := c.now()
	finish := func(state, message string) Availability {
		labels := map[string]string{"media_ok": "媒体可访问", "page_ok": "页面可访问", "failed": "暂不可用", "unchecked": "未检测"}
		return Availability{State: state, Label: labels[state], CheckedAt: c.now().UTC().Format(time.RFC3339), LatencyMS: c.now().Sub(started).Milliseconds(), Error: message}
	}
	if parent.Err() != nil {
		return finish("unchecked", "检测已取消")
	}
	if !c.allowed() {
		return finish("unchecked", "联网已关闭")
	}
	key := c.cacheKey(film)
	c.mu.Lock()
	cached, found := c.cache[key]
	c.mu.Unlock()
	if found && c.now().Before(cached.expires) {
		return cached.value
	}
	// Queueing is included in the per-candidate budget, preventing long tails.
	ctx, cancel := context.WithTimeout(parent, 22*time.Second)
	defer cancel()
	select {
	case c.slots <- struct{}{}:
	case <-ctx.Done():
		if parent.Err() == nil && c.allowed() {
			a := finish("failed", "检测超时")
			c.remember(key, a)
			return a
		}
		return finish("unchecked", "检测超时或已取消")
	}
	defer func() { <-c.slots }()
	// Revoking network permission also aborts a request already in progress.
	go func() {
		timer := time.NewTicker(100 * time.Millisecond)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				if !c.allowed() {
					cancel()
					return
				}
			}
		}
	}()
	n := &availabilityNetwork{checker: c, underlying: c.manager.net}
	if e := n.check(ctx); e != nil {
		return finish("unchecked", e.Error())
	}
	pid := film.ProviderID
	if _, parsed, _, e := splitSourceFilmID(film.ID); e == nil {
		pid = parsed
		// Public search results intentionally omit episode URLs. Reconstruct detail
		// with a gated network, while sharing only immutable/cache snapshots.
		m := &SourceManager{state: c.manager.snapshot(), net: n, store: c.manager.store, cache: map[string]filmCache{}}
		c.manager.mu.RLock()
		if item, ok := c.manager.cache[film.ID]; ok {
			m.cache[film.ID] = item
		}
		c.manager.mu.RUnlock()
		var e error
		film, e = m.detail(ctx, film.ID)
		if e != nil {
			if parent.Err() != nil || !c.allowed() {
				return finish("unchecked", "检测超时或已取消")
			}
			if ctx.Err() != nil {
				a := finish("failed", "检测超时")
				c.remember(key, a)
				return a
			}
			a := finish("failed", e.Error())
			c.remember(key, a)
			return a
		}
		c.cacheDetail(ctx, key, film)
	}
	headers := map[string]string{}
	if p, ok := c.manager.provider(pid); ok && p.Rule != nil {
		headers = ruleHeaders(*p.Rule)
	}
	best := finish("failed", "没有可检测的正片")
	groups := 0
	for _, group := range film.Groups {
		ep, ok := availabilityEpisode(group)
		if !ok {
			continue
		}
		groups++
		state, e := probeAvailabilityEpisode(ctx, n, ep.URL, headers)
		a := finish(state, "")
		if e != nil {
			a.Error = e.Error()
		}
		a.GroupKey, a.EpisodeKey = group.Key, ep.Key
		if availabilityPriority(a) < availabilityPriority(best) || groups == 1 {
			best = a
		}
		if state == "media_ok" || groups >= 2 || ctx.Err() != nil {
			break
		}
	}
	if parent.Err() != nil || !c.allowed() {
		return finish("unchecked", "检测超时或已取消")
	}
	if ctx.Err() != nil {
		best = finish("failed", "检测超时")
	}
	best.LatencyMS = c.now().Sub(started).Milliseconds()
	if c.cacheKey(film) == key {
		c.remember(key, best)
	}
	return best
}

// CheckEpisode probes precisely the matched episode. A successful film-card
// probe (which may have checked episode 1) is never reused for source switching.
func (c *AvailabilityChecker) CheckEpisode(parent context.Context, film Film, groupKey, episodeKey string) Availability {
	started := c.now()
	finish := func(state, message string) Availability {
		label := map[string]string{"media_ok": "媒体可访问", "failed": "暂不可用", "unchecked": "未检测"}[state]
		return Availability{State: state, Label: label, Error: message, CheckedAt: now(), LatencyMS: c.now().Sub(started).Milliseconds(), GroupKey: groupKey, EpisodeKey: episodeKey}
	}
	if parent.Err() != nil || !c.allowed() {
		return finish("unchecked", "检测已取消或联网已关闭")
	}
	if p, ok := c.manager.provider(film.ProviderID); ok && !p.Enabled {
		return finish("unchecked", "来源已关闭")
	}
	var chosen Episode
	for _, g := range film.Groups {
		if g.Key == groupKey {
			for _, ep := range g.Episodes {
				if ep.Key == episodeKey {
					chosen = ep
					break
				}
			}
		}
	}
	if chosen.URL == "" {
		return finish("failed", "对应集数不存在")
	}
	baseKey := c.cacheKey(film)
	key := baseKey + ":episode:" + groupKey + ":" + episodeKey + ":" + chosen.URL
	c.mu.Lock()
	cached, ok := c.cache[key]
	c.mu.Unlock()
	if ok && c.now().Before(cached.expires) {
		return cached.value
	}
	ctx, cancel := context.WithTimeout(parent, 22*time.Second)
	defer cancel()
	select {
	case c.slots <- struct{}{}:
	case <-ctx.Done():
		return finish("unchecked", "检测已取消或超时")
	}
	defer func() { <-c.slots }()
	go func() {
		timer := time.NewTicker(100 * time.Millisecond)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				if !c.allowed() {
					cancel()
					return
				}
			}
		}
	}()
	n := &availabilityNetwork{checker: c, underlying: c.manager.net}
	headers := map[string]string{}
	if p, ok := c.manager.provider(film.ProviderID); ok && p.Rule != nil {
		headers = ruleHeaders(*p.Rule)
		if headers["Referer"] == "" {
			headers["Referer"] = chosen.URL
		}
	}
	state, e := probeAvailabilityEpisode(ctx, n, chosen.URL, headers)
	a := finish(state, "")
	if e != nil {
		a.Error = e.Error()
	}
	if parent.Err() != nil || !c.allowed() || c.cacheKey(film) != baseKey {
		return finish("unchecked", "检测已取消或来源发生变化")
	}
	if ctx.Err() != nil {
		a = finish("failed", "检测超时")
	}
	c.remember(key, a)
	return a
}

func (c *AvailabilityChecker) remember(key string, a Availability) {
	if a.State == "unchecked" || !c.allowed() {
		return
	}
	ttl := 3 * time.Minute
	if a.State == "failed" {
		ttl = 45 * time.Second
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.cache) > 4000 {
		for k, item := range c.cache {
			if !c.now().Before(item.expires) {
				delete(c.cache, k)
			}
		}
		if len(c.cache) > 4000 {
			c.cache = map[string]availabilityEntry{}
		}
	}
	c.cache[key] = availabilityEntry{a, c.now().Add(ttl)}
}

func availabilityEpisode(g Group) (Episode, bool) {
	var chosen Episode
	found := false
	chosenSpecial := true
	for _, ep := range g.Episodes {
		label := strings.ToLower(ep.Name)
		if ep.URL == "" || strings.Contains(label, "预告") || strings.Contains(label, "花絮") || strings.Contains(label, "特典") || strings.Contains(label, "trailer") || strings.Contains(label, "pv") || strings.Contains(label, "teaser") {
			continue
		}
		special := strings.Contains(label, "特别") || strings.Contains(label, "番外") || strings.Contains(label, "special") || strings.HasPrefix(label, "sp")
		if !found || (chosenSpecial && !special) || (chosenSpecial == special && ep.Ordinal != nil && *ep.Ordinal > 0 && (chosen.Ordinal == nil || *chosen.Ordinal <= 0 || *ep.Ordinal < *chosen.Ordinal)) {
			chosen, found = ep, true
			chosenSpecial = special
		}
	}
	return chosen, found
}

// availabilityNetwork gates every upstream request, including rule API calls.
type availabilityNetwork struct {
	checker    *AvailabilityChecker
	underlying Fetcher
}

func (n *availabilityNetwork) check(ctx context.Context) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	if !n.checker.allowed() {
		return problem("联网已关闭", "consent_required", 403)
	}
	return nil
}
func (n *availabilityNetwork) Fetch(ctx context.Context, link string, limit int) ([]byte, Obj, error) {
	return n.Request(ctx, RequestSpec{URL: link, Method: "GET", Limit: limit})
}
func (n *availabilityNetwork) Request(ctx context.Context, r RequestSpec) ([]byte, Obj, error) {
	if e := n.check(ctx); e != nil {
		return nil, nil, e
	}
	if _, e := publicURL(r.URL); e != nil {
		return nil, nil, e
	}
	b, meta, e := fetchRequest(ctx, n.underlying, r)
	if gate := n.check(ctx); gate != nil {
		return nil, nil, gate
	}
	if e == nil && meta != nil {
		status, _ := strconv.Atoi(str(meta["http"]))
		if status != 0 && (status < 200 || status >= 300) {
			e = problem("来源返回 HTTP "+str(status), "http", 502)
		}
	}
	return b, meta, e
}
func (n *availabilityNetwork) JSON(ctx context.Context, link string, params Obj) (Obj, error) {
	if e := n.check(ctx); e != nil {
		return nil, e
	}
	if _, e := publicURL(link); e != nil {
		return nil, e
	}
	o, e := n.underlying.JSON(ctx, link, params)
	if gate := n.check(ctx); gate != nil {
		return nil, gate
	}
	return o, e
}
func (n *availabilityNetwork) Probe(ctx context.Context, name, link string) Obj {
	if e := n.check(ctx); e != nil {
		return Obj{"name": name, "error": e.Error()}
	}
	return n.underlying.Probe(ctx, name, link)
}

const availabilitySampleBytes = 192 * 1024

// sample reads at most limit bytes even when an origin ignores the Range header.
// It closes streaming responses immediately; this is never a whole-film fetch.
func (n *availabilityNetwork) sample(ctx context.Context, link string, headers map[string]string, limit int, ranged bool) ([]byte, string, error) {
	if e := n.check(ctx); e != nil {
		return nil, "", e
	}
	if _, e := publicURL(link); e != nil {
		return nil, "", e
	}
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	h := map[string]string{}
	for k, v := range headers {
		h[k] = v
	}
	if ranged {
		if h["Range"] == "" {
			h["Range"] = fmt.Sprintf("bytes=0-%d", limit-1)
		}
	}
	r := RequestSpec{URL: link, Method: "GET", Headers: h, Limit: limit}
	base := link
	var b []byte
	var e error
	if opener, ok := n.underlying.(mediaOpener); ok {
		var res *http.Response
		res, e = opener.Open(ctx, r)
		if e == nil {
			defer res.Body.Close()
			if res.StatusCode < 200 || res.StatusCode >= 300 {
				return nil, "", problem("媒体返回 HTTP "+str(res.StatusCode), "http", 502)
			}
			if ranged && !strings.HasPrefix(h["Range"], "bytes=0-") && res.StatusCode != http.StatusPartialContent {
				return nil, "", problem("媒体服务器不支持所需分段范围", "range", 502)
			}
			if res.Request != nil && res.Request.URL != nil {
				base, e = publicURL(res.Request.URL.String())
			}
			if e == nil {
				// Read one extra byte for metadata to detect truncated playlists.
				count := limit
				if !ranged {
					count++
				}
				b, e = io.ReadAll(io.LimitReader(res.Body, int64(count)))
			}
		}
	} else {
		b, _, e = n.Request(ctx, r)
	}
	if gate := n.check(ctx); gate != nil {
		return nil, "", gate
	}
	if e != nil {
		return nil, "", e
	}
	if len(b) > limit {
		if !ranged {
			return nil, "", problem("播放清单或页面过大", "too_large", 502)
		}
		b = b[:limit]
	}
	return b, base, nil
}

func probeAvailabilityEpisode(ctx context.Context, n *availabilityNetwork, link string, headers map[string]string) (string, error) {
	if _, e := publicURL(link); e != nil {
		return "failed", e
	}
	if isMediaURL(link) {
		if e := probeAvailabilityMedia(ctx, n, link, headers, 0); e != nil {
			return "failed", e
		}
		return "media_ok", nil
	}
	b, base, e := n.sample(ctx, link, headers, 1000000, false)
	if e != nil {
		return "failed", e
	}
	// Some sources use extensionless stream endpoints.
	if strings.HasPrefix(strings.TrimSpace(string(b)), "#EXTM3U") {
		if e := probeAvailabilityPlaylist(ctx, n, b, base, headers, 0); e != nil {
			return "failed", e
		}
		return "media_ok", nil
	}
	if availabilityMediaSignature(b) {
		return "media_ok", nil
	}
	if !availabilityReadablePage(b) {
		return "failed", problem("播放入口未返回有效页面或媒体", "not_media", 502)
	}
	media, frames := extractPageMedia(b, base)
	if media != "" {
		if e := probeAvailabilityMedia(ctx, n, media, headers, 0); e != nil {
			return "failed", e
		}
		return "media_ok", nil
	}
	frameReadable := false
	var frameError error
	var frameMediaError error
	for i, frame := range frames {
		if i >= 2 {
			break
		}
		raw, frameBase, err := n.sample(ctx, frame, headers, 700000, false)
		if err != nil || !availabilityReadablePage(raw) {
			if err != nil {
				frameError = err
			}
			continue
		}
		frameReadable = true
		if media, _ := extractPageMedia(raw, frameBase); media != "" {
			if e := probeAvailabilityMedia(ctx, n, media, headers, 0); e == nil {
				return "media_ok", nil
			} else {
				frameMediaError = e
			}
		}
	}
	if frameMediaError != nil {
		return "failed", frameMediaError
	}
	if len(frames) > 0 && !frameReadable {
		if frameError == nil {
			frameError = problem("网页播放器未返回有效页面", "page_player", 502)
		}
		return "failed", frameError
	}
	// The native player has no HTML/JavaScript engine. A reachable page without
	// a statically recoverable stream is not a playable native candidate.
	return "failed", problem("此来源需要网页播放器，请换一个来源", "page_player", 502)
}

func probeAvailabilityMedia(ctx context.Context, n *availabilityNetwork, link string, headers map[string]string, depth int) error {
	if depth > 3 {
		return problem("播放清单层级过多", "hls", 502)
	}
	ranged := !strings.Contains(strings.ToLower(strings.Split(link, "?")[0]), ".m3u8")
	limit := availabilitySampleBytes
	if !ranged {
		limit = 500000
	}
	b, base, e := n.sample(ctx, link, headers, limit, ranged)
	if e != nil {
		return e
	}
	if strings.HasPrefix(strings.TrimSpace(string(b)), "#EXTM3U") {
		return probeAvailabilityPlaylist(ctx, n, b, base, headers, depth)
	}
	if !availabilityMediaSignature(b) {
		return problem("媒体响应不是有效的视频或音频数据", "not_media", 502)
	}
	return nil
}

var availabilityHLSAttribute = re(`([A-Z0-9-]+)=(?:"([^"]*)"|([^,]*))`)

func availabilityHLSAttrs(line string) map[string]string {
	out := map[string]string{}
	if _, tail, ok := strings.Cut(line, ":"); ok {
		for _, item := range availabilityHLSAttribute.FindAllStringSubmatch(tail, -1) {
			v := item[2]
			if v == "" {
				v = strings.TrimSpace(item[3])
			}
			out[item[1]] = v
		}
	}
	return out
}

func probeAvailabilityPlaylist(ctx context.Context, n *availabilityNetwork, body []byte, base string, headers map[string]string, depth int) error {
	if depth > 3 || len(body) > 500000 || !strings.HasPrefix(strings.TrimSpace(string(body)), "#EXTM3U") {
		return problem("播放清单无效", "hls", 502)
	}
	lines := strings.Split(strings.ReplaceAll(string(body), "\r\n", "\n"), "\n")
	if len(lines) > 10000 {
		return problem("播放清单条目过多", "hls", 502)
	}
	segment, variant, initURL := "", "", ""
	segmentRange, initRange, nextRange := "", "", ""
	currentInitURL, currentInitRange := "", ""
	keyAttrs := map[string]string{}
	segmentKey := map[string]string{}
	initKey := map[string]string{}
	currentInitKey := map[string]string{}
	sequence := uint64(0)
	segmentSequence := uint64(0)
	nextVariant := false
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			attrs := availabilityHLSAttrs(line)
			// Validate every referenced URI, not just the selected rendition.
			if uri := attrs["URI"]; uri != "" {
				if _, e := ruleURL(base, uri); e != nil {
					return e
				}
			}
			switch {
			case strings.HasPrefix(line, "#EXT-X-STREAM-INF:"):
				nextVariant = true
			case strings.HasPrefix(line, "#EXT-X-MEDIA-SEQUENCE:"):
				sequence, _ = strconv.ParseUint(strings.TrimSpace(strings.TrimPrefix(line, "#EXT-X-MEDIA-SEQUENCE:")), 10, 64)
			case strings.HasPrefix(line, "#EXT-X-KEY:"):
				keyAttrs = attrs
			case strings.HasPrefix(line, "#EXT-X-BYTERANGE:"):
				nextRange = strings.TrimSpace(strings.TrimPrefix(line, "#EXT-X-BYTERANGE:"))
			case strings.HasPrefix(line, "#EXT-X-MAP:"):
				if attrs["URI"] == "" {
					return problem("初始化分片地址无效", "hls", 502)
				}
				currentInitURL, _ = ruleURL(base, attrs["URI"])
				currentInitRange = attrs["BYTERANGE"]
				currentInitKey = keyAttrs
			}
			continue
		}
		u, e := ruleURL(base, line)
		if e != nil {
			return e
		}
		if nextVariant {
			if variant == "" {
				variant = u
			}
			nextVariant = false
		} else {
			if segment == "" {
				segment, segmentKey, segmentSequence = u, keyAttrs, sequence
				segmentRange = nextRange
				initURL, initRange, initKey = currentInitURL, currentInitRange, currentInitKey
			}
			sequence++
			nextRange = ""
		}
	}
	if variant != "" {
		return probeAvailabilityMedia(ctx, n, variant, headers, depth+1)
	}
	if segment == "" {
		return problem("播放清单没有分片", "hls", 502)
	}
	if initURL != "" {
		h, limit, e := availabilityRange(headers, initRange, true)
		if e != nil {
			return e
		}
		b, _, e := n.sample(ctx, initURL, h, limit, true)
		if e != nil {
			return e
		}
		b, e = availabilityDecryptSample(ctx, n, b, initKey, base, headers, segmentSequence)
		if e != nil {
			return e
		}
		if !availabilityMediaSignature(b) {
			return problem("初始化分片无效", "not_media", 502)
		}
	}
	h, limit, e := availabilityRange(headers, segmentRange, false)
	if e != nil {
		return e
	}
	b, _, e := n.sample(ctx, segment, h, limit, true)
	if e != nil {
		return e
	}
	b, e = availabilityDecryptSample(ctx, n, b, segmentKey, base, headers, segmentSequence)
	if e != nil {
		return e
	}
	if !availabilityMediaSignature(b) {
		return problem("媒体分片不是有效的视频或音频数据", "not_media", 502)
	}
	return nil
}

func availabilityRange(headers map[string]string, raw string, isMap bool) (map[string]string, int, error) {
	if raw == "" {
		return headers, availabilitySampleBytes, nil
	}
	length, offset, hasOffset := strings.Cut(raw, "@")
	n, e := strconv.ParseInt(length, 10, 64)
	if e != nil || n <= 0 || n > 1<<50 || (!hasOffset && !isMap) {
		return nil, 0, problem("分片字节范围无效", "hls", 502)
	}
	start := int64(0)
	if hasOffset {
		start, e = strconv.ParseInt(offset, 10, 64)
		if e != nil || start < 0 || start > 1<<50 {
			return nil, 0, problem("分片字节范围无效", "hls", 502)
		}
	}
	if n > availabilitySampleBytes {
		n = availabilitySampleBytes
	}
	h := map[string]string{}
	for k, v := range headers {
		h[k] = v
	}
	h["Range"] = fmt.Sprintf("bytes=%d-%d", start, start+n-1)
	return h, int(n), nil
}

// Standard HLS AES-128 is sampled only with a server-provided identity key.
// Other encryption/key systems are not interpreted or marked as media_ok.
func availabilityDecryptSample(ctx context.Context, n *availabilityNetwork, b []byte, attrs map[string]string, base string, headers map[string]string, sequence uint64) ([]byte, error) {
	method := attrs["METHOD"]
	if method == "" || method == "NONE" {
		return b, nil
	}
	if method != "AES-128" || (attrs["KEYFORMAT"] != "" && attrs["KEYFORMAT"] != "identity") || attrs["URI"] == "" {
		return nil, problem("此加密清单无法完成媒体抽样", "hls_key", 502)
	}
	u, e := ruleURL(base, attrs["URI"])
	if e != nil {
		return nil, e
	}
	key, _, e := n.sample(ctx, u, headers, 16, false)
	if e != nil || len(key) != 16 {
		return nil, problem("媒体密钥不可达或格式无效", "hls_key", 502)
	}
	iv := make([]byte, 16)
	if raw := attrs["IV"]; raw != "" {
		raw = strings.TrimPrefix(strings.TrimPrefix(raw, "0x"), "0X")
		if len(raw) > 32 {
			return nil, problem("媒体 IV 无效", "hls_key", 502)
		}
		raw = strings.Repeat("0", 32-len(raw)) + raw
		iv, e = hex.DecodeString(raw)
		if e != nil {
			return nil, problem("媒体 IV 无效", "hls_key", 502)
		}
	} else {
		binary.BigEndian.PutUint64(iv[8:], sequence)
	}
	if len(b) < 16 {
		return nil, problem("加密分片无效", "hls_key", 502)
	}
	b = append([]byte(nil), b[:len(b)/16*16]...)
	block, _ := aes.NewCipher(key)
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(b, b)
	return b, nil
}

func availabilityMediaSignature(b []byte) bool {
	if len(b) < 8 {
		return false
	}
	trimmed := bytes.TrimSpace(b)
	if len(trimmed) == 0 || trimmed[0] == '<' || trimmed[0] == '{' || trimmed[0] == '[' {
		return false
	}
	if bytes.Equal(b[:4], []byte{0x1a, 0x45, 0xdf, 0xa3}) || bytes.Equal(b[:4], []byte("OggS")) {
		return true
	}
	if bytes.Equal(b[:3], []byte("ID3")) || (b[0] == 0xff && b[1]&0xf6 == 0xf0) {
		return true
	}
	if len(b) >= 12 {
		switch string(b[4:8]) {
		case "ftyp", "styp", "moof", "moov":
			return binary.BigEndian.Uint32(b[:4]) >= 8
		}
	}
	// Three transport-stream sync bytes distinguish TS from arbitrary payloads.
	for offset := 0; offset < 188 && offset+376 < len(b); offset++ {
		if b[offset] == 0x47 && b[offset+188] == 0x47 && b[offset+376] == 0x47 {
			return true
		}
	}
	return false
}

func availabilityReadablePage(b []byte) bool {
	if len(b) < 32 {
		return false
	}
	s := strings.ToLower(string(b))
	for _, bad := range []string{"cf-chl-", "/cdn-cgi/challenge", "<title>just a moment", "<title>access denied", "404 not found", "502 bad gateway", "503 service unavailable", "人机验证", "访问被拒绝", "本站已关闭", "页面不存在", "请求过于频繁"} {
		if strings.Contains(s, bad) {
			return false
		}
	}
	return (strings.Contains(s, "<html") || strings.Contains(s, "<video") || strings.Contains(s, "<iframe")) && (len([]rune(text(string(b), 200))) >= 8 || strings.Contains(s, "<video") || strings.Contains(s, "<iframe") || strings.Contains(s, "player_"))
}
