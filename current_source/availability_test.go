// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func availabilityMP4() string {
	return string([]byte{0, 0, 0, 24}) + "ftypisom" + strings.Repeat("\x00", 80)
}
func availabilityTS() string {
	b := make([]byte, 188*3)
	b[0], b[188], b[376] = 0x47, 0x47, 0x47
	return string(b)
}
func availabilityFilm(id, link string) Film {
	return Film{ID: id, Title: "正片测试", Groups: []Group{{Key: "g0", Episodes: []Episode{{Key: "e0", Name: "第1集", URL: link}}}}}
}
func availabilityTestChecker(t *testing.T, n Fetcher) *AvailabilityChecker {
	t.Helper()
	s, e := newService(t.TempDir(), n)
	mustNoErr(t, e)
	allowNetwork(t, s)
	return newAvailabilityChecker(s.manager)
}

func TestAvailabilitySamples(t *testing.T) {
	for _, tc := range []struct{ name, url, body, state string }{
		{"mp4", "https://media.example.com/one.mp4", availabilityMP4(), "media_ok"},
		{"extensionless transport", "https://media.example.com/stream", availabilityTS(), "media_ok"},
		{"mp4 is html", "https://media.example.com/one.mp4", "<html><body>媒体服务器暂时没有该资源，请稍后重试。</body></html>", "failed"},
		{"readable player", "https://anime.example.com/play/1", "<html><body>测试影片第一集，网页播放器加载中。<script src='dynamic.js'></script></body></html>", "page_ok"},
		{"verification page", "https://anime.example.com/play/1", "<html><title>Just a moment</title><body>Human verification needed</body></html>", "failed"},
		{"empty page", "https://anime.example.com/play/1", "<html></html>", "failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			n := &mappedNetwork{data: map[string]string{tc.url: tc.body}}
			c := availabilityTestChecker(t, n)
			a := c.Check(context.Background(), availabilityFilm("test", tc.url))
			require(t, a.State == tc.state, "wrong reachability: "+str(a))
			if tc.state == "media_ok" || tc.state == "page_ok" {
				require(t, a.GroupKey == "g0" && a.EpisodeKey == "e0", "selected episode missing")
			}
			b, e := json.Marshal(a)
			mustNoErr(t, e)
			require(t, !strings.Contains(string(b), "media.example.com") && !strings.Contains(string(b), "anime.example.com"), "availability exposes media URL")
		})
	}
}

func TestAvailabilityHLS(t *testing.T) {
	base := "https://media.example.com/hls/"
	for _, tc := range []struct{ name, playlist, segment, state string }{
		{"master to transport", "#EXTM3U\n#EXTINF:4,\nseg.ts\n#EXT-X-ENDLIST\n", availabilityTS(), "media_ok"},
		{"html segment", "#EXTM3U\n#EXTINF:4,\nseg.ts\n", "<html><body>The media server is temporarily unavailable</body></html>", "failed"},
		{"private child", "#EXTM3U\n#EXTINF:4,\nhttp://127.0.0.1/seg.ts\n", availabilityTS(), "failed"},
		{"private unused key", "#EXTM3U\n#EXT-X-KEY:METHOD=NONE,URI=\"http://192.168.1.2/key\"\n#EXTINF:4,\nseg.ts\n", availabilityTS(), "failed"},
		{"empty playlist", "#EXTM3U\n#EXT-X-ENDLIST\n", availabilityTS(), "failed"},
		{"unsupported key", "#EXTM3U\n#EXT-X-KEY:METHOD=SAMPLE-AES,URI=\"key\"\n#EXTINF:4,\nseg.ts\n", availabilityTS(), "failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			n := &mappedNetwork{data: map[string]string{base + "master.m3u8": "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=300000\nmain.m3u8\n", base + "main.m3u8": tc.playlist, base + "seg.ts": tc.segment}}
			c := availabilityTestChecker(t, n)
			a := c.Check(context.Background(), availabilityFilm("test", base+"master.m3u8"))
			require(t, a.State == tc.state, "wrong HLS state: "+str(a))
			for _, r := range n.requests {
				require(t, !strings.Contains(r.URL, "127.0.0.1") && !strings.Contains(r.URL, "192.168.1.2"), "private child requested")
				if strings.HasSuffix(r.URL, "seg.ts") {
					require(t, r.Headers["Range"] == "bytes=0-196607" && r.Limit == availabilitySampleBytes, "unbounded segment sample")
				}
			}
		})
	}
	t.Run("map and server identity key", func(t *testing.T) {
		key := []byte("0123456789abcdef")
		plain := []byte(availabilityTS())
		plain = append(plain, make([]byte, aes.BlockSize-len(plain)%aes.BlockSize)...)
		block, e := aes.NewCipher(key)
		mustNoErr(t, e)
		iv := make([]byte, 16)
		iv[15] = 7
		encrypted := make([]byte, len(plain))
		cipher.NewCBCEncrypter(block, iv).CryptBlocks(encrypted, plain)
		n := &mappedNetwork{data: map[string]string{base + "main.m3u8": "#EXTM3U\n#EXT-X-MAP:URI=\"init.mp4\"\n#EXT-X-MEDIA-SEQUENCE:7\n#EXT-X-KEY:METHOD=AES-128,URI=\"key\"\n#EXTINF:4,\nseg.ts\n", base + "seg.ts": string(encrypted), base + "key": string(key), base + "init.mp4": availabilityMP4()}}
		c := availabilityTestChecker(t, n)
		a := c.Check(context.Background(), availabilityFilm("test", base+"main.m3u8"))
		require(t, a.State == "media_ok", "server-provided HLS key sample failed: "+str(a))
	})
}

func TestAvailabilityFallbackAndCache(t *testing.T) {
	n := &mappedNetwork{data: map[string]string{"https://media.example.com/good.mp4": availabilityMP4()}, fail: map[string]bool{"https://media.example.com/bad.mp4": true}}
	c := availabilityTestChecker(t, n)
	now := time.Now()
	c.now = func() time.Time { return now }
	f := availabilityFilm("test", "https://media.example.com/bad.mp4")
	f.Groups = append(f.Groups, Group{Key: "g1", Episodes: []Episode{{Key: "trailer", Name: "预告片", URL: "https://media.example.com/skip.mp4"}, {Key: "main", Name: "第1集", URL: "https://media.example.com/good.mp4"}}})
	a := c.Check(context.Background(), f)
	require(t, a.State == "media_ok" && a.GroupKey == "g1" && a.EpisodeKey == "main", "fallback group not preferred")
	count := len(n.requests)
	_ = c.Check(context.Background(), f)
	require(t, len(n.requests) == count, "good cache missed")
	now = now.Add(3*time.Minute + time.Second)
	_ = c.Check(context.Background(), f)
	require(t, len(n.requests) > count, "good cache did not expire")
	f.ID = "bad"
	f.Groups = f.Groups[:1]
	a = c.Check(context.Background(), f)
	require(t, a.State == "failed", "bad entry accepted")
	count = len(n.requests)
	_ = c.Check(context.Background(), f)
	require(t, len(n.requests) == count, "bad cache missed")
	now = now.Add(46 * time.Second)
	_ = c.Check(context.Background(), f)
	require(t, len(n.requests) > count, "bad cache did not expire")
	for i, state := range []string{"media_ok", "page_ok", "unchecked", "failed"} {
		require(t, availabilityPriority(Availability{State: state}) == i, "wrong availability order")
	}
}

func TestAvailabilityOfficialAndFrames(t *testing.T) {
	for _, tc := range []struct {
		name, page, frame, state string
		failFrame                bool
	}{
		{"official public page", "<html><body>测试电影官方播放页面，无需任何账号即可读取。</body></html>", "", "page_ok", false},
		{"unreachable player frame", "<html><body><iframe src='https://player.example.com/main'></iframe></body></html>", "", "failed", true},
		{"known frame media broken", "<html><body><iframe src='https://player.example.com/main'></iframe></body></html>", "<html><body><video src='https://media.example.com/bad.mp4'></video></body></html>", "failed", false},
		{"dynamic frame page", "<html><body><iframe src='https://player.example.com/main'></iframe></body></html>", "<html><body>影片播放器，通过动态脚本加载媒体。</body></html>", "page_ok", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			link := "https://v.qq.com/x/cover/test.html"
			n := &mappedNetwork{data: map[string]string{link: tc.page, "https://player.example.com/main": tc.frame}, fail: map[string]bool{"https://player.example.com/main": tc.failFrame, "https://media.example.com/bad.mp4": true}}
			c := availabilityTestChecker(t, n)
			f := availabilityFilm("test", link)
			f.Groups[0].Key = "official"
			a := c.Check(context.Background(), f)
			require(t, a.State == tc.state, "wrong official/frame observation: "+str(a))
		})
	}
}

func TestAvailabilityPublicDetailAndRuleInvalidation(t *testing.T) {
	n := &mappedNetwork{data: map[string]string{"https://anime.example.com/detail/1": "<html><body><ul class='road'><li><a href='/play/1'>第1集</a></li></ul></body></html>", "https://anime.example.com/play/1": "<html><body>正确的影片播放页面。<video src='https://media.example.com/good.mp4'></video></body></html>", "https://media.example.com/good.mp4": availabilityMP4()}}
	c := availabilityTestChecker(t, n)
	r := c.manager.snapshot()
	rule := ruleFixture()
	r.Providers = []Provider{{ID: "anime", Name: "测试动漫", Kind: "kazumi", Enabled: true, Rule: &rule}}
	mustNoErr(t, c.manager.save(r))
	f := Film{ID: sourceFilmID("rule", "anime", "https://anime.example.com/detail/1"), ProviderID: "anime", Title: "测试"}
	a := c.Check(context.Background(), f)
	require(t, a.State == "media_ok", "public film detail not reconstructed: "+str(a))
	count := len(n.requests)
	_ = c.Check(context.Background(), f)
	require(t, len(n.requests) == count, "provider cache missed")
	r.Providers[0].Rule.Version = "2"
	mustNoErr(t, c.manager.save(r))
	_ = c.Check(context.Background(), f)
	require(t, len(n.requests) > count, "rule change did not invalidate probe")
	n.fail = map[string]bool{"https://anime.example.com/detail/1": true}
	r.Providers[0].Rule.Version = "3"
	mustNoErr(t, c.manager.save(r))
	a = c.Check(context.Background(), f)
	require(t, a.State == "failed", "inaccessible detail not failed")
}

func TestAvailabilityDetailCacheAtomicFingerprint(t *testing.T) {
	c := availabilityTestChecker(t, &mappedNetwork{})
	f := availabilityFilm(sourceFilmID("vod", "wsyzy", "1"), "https://media.example.com/good.mp4")
	f.ProviderID = "wsyzy"
	key := c.cacheKey(f)
	require(t, c.cacheDetail(context.Background(), key, f), "matching detail not cached")
	// An update clears the cache. A completed old probe must not restore its
	// detail under the new provider definition or catalog address.
	r := c.manager.snapshot()
	for i := range r.Providers {
		if r.Providers[i].ID == "wsyzy" {
			r.Providers[i].Name += " updated"
		}
	}
	mustNoErr(t, c.manager.save(r))
	require(t, !c.cacheDetail(context.Background(), key, f), "old provider detail restored after update")
	require(t, len(c.manager.cache) == 0, "stale detail inserted")
	key = c.cacheKey(f)
	r.Catalog.APIURL = "https://new.example.com/api.php/provide/vod/"
	mustNoErr(t, c.manager.save(r))
	require(t, !c.cacheDetail(context.Background(), key, f), "old catalog detail restored after update")
	key = c.cacheKey(f)
	mustNoErr(t, c.manager.store.SetMany(Obj{"enabled_providers": Obj{"wsyzy": false}}))
	require(t, !c.cacheDetail(context.Background(), key, f), "source switches omitted from atomic fingerprint")
	mustNoErr(t, c.manager.store.SetMany(Obj{"enabled_providers": Obj{"wsyzy": true}}))
	key = c.cacheKey(f)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require(t, !c.cacheDetail(ctx, key, f), "canceled probe restored detail")
	mustNoErr(t, c.manager.store.SetMany(Obj{"network_consent": false}))
	require(t, !c.cacheDetail(context.Background(), key, f), "revoked network restored detail")
}

func TestAvailabilityHLSMapBelongsToSelectedSegment(t *testing.T) {
	base := "https://media.example.com/hls/"
	for _, tc := range []struct {
		name, playlist string
		requestedInit  bool
	}{
		{"later map does not affect first TS", "#EXTM3U\n#EXTINF:4,\nfirst.ts\n#EXT-X-DISCONTINUITY\n#EXT-X-MAP:URI=\"missing-init.mp4\"\n#EXTINF:4,\nlater.m4s\n", false},
		{"active map replaced before first segment", "#EXTM3U\n#EXT-X-MAP:URI=\"missing-init.mp4\"\n#EXT-X-MAP:URI=\"active-init.mp4\"\n#EXTINF:4,\nfirst.ts\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			n := &mappedNetwork{data: map[string]string{base + "main.m3u8": tc.playlist, base + "first.ts": availabilityTS(), base + "active-init.mp4": availabilityMP4()}}
			c := availabilityTestChecker(t, n)
			a := c.Check(context.Background(), availabilityFilm("test", base+"main.m3u8"))
			require(t, a.State == "media_ok", "unrelated MAP affected selected segment: "+str(a))
			requested := false
			for _, r := range n.requests {
				require(t, r.URL != base+"missing-init.mp4" && r.URL != base+"later.m4s", "sample fetched unrelated later MAP/segment")
				requested = requested || r.URL == base+"active-init.mp4"
			}
			require(t, requested == tc.requestedInit, "active initialization not sampled correctly")
		})
	}
}

type availabilityBlockingNetwork struct {
	*mappedNetwork
	started chan struct{}
	release chan struct{}
	mu      sync.Mutex
	active  int
	peak    int
}

func (n *availabilityBlockingNetwork) Request(ctx context.Context, r RequestSpec) ([]byte, Obj, error) {
	n.mu.Lock()
	n.active++
	if n.active > n.peak {
		n.peak = n.active
	}
	n.mu.Unlock()
	defer func() { n.mu.Lock(); n.active--; n.mu.Unlock() }()
	select {
	case n.started <- struct{}{}:
	default:
	}
	select {
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	case <-n.release:
		return []byte(availabilityMP4()), nil, nil
	}
}

func TestAvailabilityConsentAndCancellation(t *testing.T) {
	n := &availabilityBlockingNetwork{mappedNetwork: &mappedNetwork{}, started: make(chan struct{}, 20), release: make(chan struct{})}
	c := availabilityTestChecker(t, n)
	f := availabilityFilm("test", "https://media.example.com/good.mp4")
	e := c.manager.store.SetMany(Obj{"network_consent": false})
	mustNoErr(t, e)
	a := c.Check(context.Background(), f)
	require(t, a.State == "unchecked" && len(n.started) == 0, "no-consent check made request")
	e = c.manager.store.SetMany(Obj{"network_consent": true})
	mustNoErr(t, e)
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan Availability, 1)
	go func() { result <- c.Check(ctx, f) }()
	<-n.started
	cancel()
	a = <-result
	require(t, a.State == "unchecked" && len(c.cache) == 0, "canceled check cached failure")
	go func() { result <- c.Check(context.Background(), f) }()
	<-n.started
	e = c.manager.store.SetMany(Obj{"network_consent": false})
	mustNoErr(t, e)
	select {
	case a = <-result:
		require(t, a.State == "unchecked" && len(c.cache) == 0, "revoked permission cached result")
	case <-time.After(time.Second):
		t.Fatal("revoked permission did not abort in-flight request")
	}
}

func TestAvailabilityGlobalConcurrency(t *testing.T) {
	n := &availabilityBlockingNetwork{mappedNetwork: &mappedNetwork{}, started: make(chan struct{}, 20), release: make(chan struct{})}
	c := availabilityTestChecker(t, n)
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = c.Check(context.Background(), availabilityFilm("test"+str(i), "https://media.example.com/good.mp4"))
		}(i)
	}
	for i := 0; i < 6; i++ {
		select {
		case <-n.started:
		case <-time.After(2 * time.Second):
			t.Fatal("workers did not start")
		}
	}
	close(n.release)
	wg.Wait()
	require(t, n.peak == 6, "availability concurrency not bounded at six")
}

type availabilityMeasuredBody struct {
	reader *strings.Reader
	read   int
	closed bool
}

func (b *availabilityMeasuredBody) Read(p []byte) (int, error) {
	n, e := b.reader.Read(p)
	b.read += n
	return n, e
}
func (b *availabilityMeasuredBody) Close() error { b.closed = true; return nil }

type availabilityOpeningNetwork struct {
	*mappedNetwork
	bodies      []*availabilityMeasuredBody
	ignoreRange bool
}

func (n *availabilityOpeningNetwork) Open(ctx context.Context, r RequestSpec) (*http.Response, error) {
	n.requests = append(n.requests, r)
	body, ok := n.data[r.URL]
	if !ok {
		return nil, problem("测试媒体不存在", "http", 502)
	}
	status := 200
	if raw := r.Headers["Range"]; raw != "" && !n.ignoreRange {
		bounds := strings.Split(strings.TrimPrefix(raw, "bytes="), "-")
		start, _ := strconv.Atoi(bounds[0])
		end, _ := strconv.Atoi(bounds[1])
		if start < 0 || start >= len(body) {
			return nil, fmt.Errorf("range outside body")
		}
		if end >= len(body) {
			end = len(body) - 1
		}
		body = body[start : end+1]
		status = 206
	}
	reader := &availabilityMeasuredBody{reader: strings.NewReader(body)}
	n.bodies = append(n.bodies, reader)
	u, _ := url.Parse(r.URL)
	return &http.Response{StatusCode: status, Header: http.Header{}, Body: reader, Request: &http.Request{URL: u}}, nil
}

var _ io.ReadCloser = (*availabilityMeasuredBody)(nil)

func TestAvailabilityStreamingBoundAndByteRanges(t *testing.T) {
	t.Run("origin ignores range", func(t *testing.T) {
		link := "https://media.example.com/large.mp4"
		n := &availabilityOpeningNetwork{mappedNetwork: &mappedNetwork{data: map[string]string{link: availabilityMP4() + strings.Repeat("x", 2*availabilitySampleBytes)}}, ignoreRange: true}
		c := availabilityTestChecker(t, n)
		a := c.Check(context.Background(), availabilityFilm("test", link))
		require(t, a.State == "media_ok" && len(n.bodies) == 1, "large stream sample failed")
		require(t, n.bodies[0].closed && n.bodies[0].read == availabilitySampleBytes, "stream not bounded and closed")
	})
	for _, ignore := range []bool{false, true} {
		t.Run("byterange ignored="+str(ignore), func(t *testing.T) {
			base := "https://media.example.com/hls/"
			n := &availabilityOpeningNetwork{mappedNetwork: &mappedNetwork{data: map[string]string{base + "main.m3u8": "#EXTM3U\n#EXT-X-MAP:URI=\"single.mp4\",BYTERANGE=\"92@0\"\n#EXTINF:4,\n#EXT-X-BYTERANGE:92@200\nsingle.mp4\n", base + "single.mp4": availabilityMP4() + strings.Repeat("x", 108) + availabilityMP4()}}, ignoreRange: ignore}
			c := availabilityTestChecker(t, n)
			a := c.Check(context.Background(), availabilityFilm("test", base+"main.m3u8"))
			if ignore {
				require(t, a.State == "failed", "ignored nonzero byte range falsely promoted")
			} else {
				require(t, a.State == "media_ok", "byte range sample failed: "+str(a))
				require(t, n.requests[len(n.requests)-1].Headers["Range"] == "bytes=200-291", "wrong HLS byte offset")
			}
		})
	}
}
