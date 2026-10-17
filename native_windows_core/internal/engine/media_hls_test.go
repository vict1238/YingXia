// SPDX-License-Identifier: GPL-3.0-or-later
package engine

import (
	"strings"
	"testing"
)

func relayTestRewrite(t *testing.T, manifest string, enabled bool) (*MediaRelay, string, []string) {
	t.Helper()
	m := newMediaRelay(nil)
	t.Cleanup(m.close)
	m.setAdSkip(enabled)
	body, e := m.rewritePlaylist([]byte(manifest), "https://media.example.org/vod/index.m3u8", map[string]string{"Referer": "https://player.example.org/"})
	if e != nil {
		t.Fatal(e)
	}
	var urls []string
	for _, line := range strings.Split(string(body), "\n") {
		if strings.HasPrefix(line, "/api/media?") {
			g, e := m.grant(mediaTicket(line))
			if e != nil {
				t.Fatal(e)
			}
			urls = append(urls, g.URL)
		}
	}
	return m, string(body), urls
}
func TestRelayMarkedAdsAndOrdinaryDiscontinuity(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		skip       int
		want       []string
	}{
		{"normal-short-segment", "#EXTM3U\n#EXT-X-TARGETDURATION:6\n#EXTINF:1,\na.ts\n#EXT-X-DISCONTINUITY\n#EXTINF:6,\nb.ts\n#EXT-X-ENDLIST", 0, []string{"a.ts", "b.ts"}},
		{"paired-cue", "#EXTM3U\n#EXTINF:6,\na.ts\n#EXT-X-CUE-OUT\n#EXTINF:5,\nad.ts\n#EXT-X-CUE-IN\n#EXTINF:6,\nb.ts\n#EXT-X-ENDLIST", 1, []string{"a.ts", "b.ts"}},
		{"bounded-cue", "#EXTM3U\n#EXT-X-CUE-OUT:5\n#EXTINF:5,\nad.ts\n#EXTINF:6,\nb.ts\n#EXT-X-ENDLIST", 1, []string{"b.ts"}},
		{"unbounded-cue-preserved", "#EXTM3U\n#EXT-X-CUE-OUT\n#EXTINF:5,\nad.ts\n#EXTINF:6,\nb.ts\n#EXT-X-ENDLIST", 0, []string{"ad.ts", "b.ts"}},
		{"live-sequence-preserved", "#EXTM3U\n#EXT-X-MEDIA-SEQUENCE:91\n#EXT-X-CUE-OUT:5\n#EXTINF:5,\nad.ts\n#EXTINF:6,\nb.ts", 0, []string{"ad.ts", "b.ts"}},
		{"date-ad-class", "#EXTM3U\n#EXT-X-PROGRAM-DATE-TIME:2026-10-16T00:00:00Z\n#EXTINF:6,\na.ts\n#EXT-X-DATERANGE:ID=\"break\",CLASS=\"ad-break\",START-DATE=\"2026-10-16T00:00:06Z\",DURATION=5\n#EXTINF:5,\nad.ts\n#EXTINF:6,\nb.ts\n#EXT-X-ENDLIST", 1, []string{"a.ts", "b.ts"}},
		{"scte-out-daterange", "#EXTM3U\n#EXT-X-PROGRAM-DATE-TIME:2026-10-16T00:00:00Z\n#EXT-X-DATERANGE:ID=\"splice\",START-DATE=\"2026-10-16T00:00:00Z\",SCTE35-OUT=0xFC,DURATION=5\n#EXTINF:5,\nad.ts\n#EXTINF:6,\nb.ts\n#EXT-X-ENDLIST", 1, []string{"b.ts"}},
		{"program-date-range", "#EXTM3U\n#EXT-X-PROGRAM-DATE-TIME:2026-10-16T00:00:00Z\n#EXT-X-DATERANGE:ID=\"chapter\",CLASS=\"chapter\",START-DATE=\"2026-10-16T00:00:00Z\",DURATION=5\n#EXTINF:5,\na.ts\n#EXTINF:6,\nb.ts\n#EXT-X-ENDLIST", 0, []string{"a.ts", "b.ts"}},
		{"interstitial-keeps-main-content", "#EXTM3U\n#EXT-X-PROGRAM-DATE-TIME:2026-10-16T00:00:00Z\n#EXT-X-DATERANGE:ID=\"external\",CLASS=\"com.apple.hls.interstitial\",START-DATE=\"2026-10-16T00:00:00Z\",DURATION=5,X-ASSET-URI=\"asset.m3u8\"\n#EXTINF:5,\na.ts\n#EXTINF:6,\nb.ts\n#EXT-X-ENDLIST", 0, []string{"a.ts", "b.ts"}},
		{"partial-segment-not-removed", "#EXTM3U\n#EXT-X-PROGRAM-DATE-TIME:2026-10-16T00:00:00Z\n#EXT-X-DATERANGE:ID=\"partial\",CLASS=\"ad\",START-DATE=\"2026-10-16T00:00:01Z\",DURATION=2\n#EXTINF:5,\na.ts\n#EXTINF:6,\nb.ts\n#EXT-X-ENDLIST", 0, []string{"a.ts", "b.ts"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, out, urls := relayTestRewrite(t, tc.body, true)
			if len(urls) != len(tc.want) {
				t.Fatalf("URLs: %v\n%s", urls, out)
			}
			for i, w := range tc.want {
				if !strings.HasSuffix(urls[i], "/"+w) {
					t.Fatalf("URLs: %v", urls)
				}
			}
			if m.stats.skipped != uint64(tc.skip) {
				t.Fatalf("skip count %d", m.stats.skipped)
			}
			if tc.name == "normal-short-segment" && !strings.Contains(out, "#EXT-X-DISCONTINUITY") {
				t.Fatal("ordinary boundary lost")
			}
			if tc.name == "paired-cue" && !strings.Contains(out, "#EXT-X-DISCONTINUITY") {
				t.Fatal("splice must reset timestamp continuity")
			}
		})
	}
}
func TestRelayAdPreferenceEncryptionAndByteRanges(t *testing.T) {
	body := "#EXTM3U\n#EXT-X-VERSION:4\n#EXT-X-MEDIA-SEQUENCE:42\n#EXT-X-KEY:METHOD=AES-128,URI=\"secret.key\"\n#EXTINF:5,\n#EXT-X-BYTERANGE:100@0\nmovie.ts\n#EXT-X-CUE-OUT:5\n#EXTINF:5,\n#EXT-X-BYTERANGE:100\nmovie.ts\n#EXTINF:5,\n#EXT-X-BYTERANGE:100\nmovie.ts\n#EXT-X-ENDLIST"
	m, out, urls := relayTestRewrite(t, body, true)
	if len(urls) != 2 || m.stats.skipped != 1 || !strings.Contains(out, "IV=0x0000000000000000000000000000002a") || !strings.Contains(out, "IV=0x0000000000000000000000000000002c") || !strings.Contains(out, "#EXT-X-BYTERANGE:100@200") {
		t.Fatalf("encryption/range state broken:\n%s", out)
	}
	var ranges []string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "/api/media?") {
			g, _ := m.grant(mediaTicket(line))
			ranges = append(ranges, g.ByteRange)
		}
	}
	if strings.Join(ranges, ",") != "bytes=0-99,bytes=200-299" {
		t.Fatalf("prefetch ranges %v", ranges)
	}
	m2, out2, urls2 := relayTestRewrite(t, body, false)
	if len(urls2) != 3 || m2.stats.skipped != 0 || strings.Contains(out2, "IV=") {
		t.Fatal("disabled preference rewrote timeline")
	}
}
func TestRelayAdMapsAndKeyRotation(t *testing.T) {
	body := "#EXTM3U\n#EXT-X-KEY:METHOD=AES-128,URI=\"a.key\",IV=0x01\n#EXT-X-MAP:URI=\"a.mp4\",BYTERANGE=\"20@0\"\n#EXTINF:5,\na.m4s\n#EXT-X-CUE-OUT:5\n#EXT-X-KEY:METHOD=NONE\n#EXT-X-MAP:URI=\"ad.mp4\"\n#EXTINF:5,\nad.m4s\n#EXT-X-KEY:METHOD=AES-128,URI=\"c.key\",IV=0x03\n#EXT-X-MAP:URI=\"c.mp4\",BYTERANGE=\"30@4\"\n#EXT-X-KEY:METHOD=AES-128,URI=\"d.key\",IV=0x04\n#EXTINF:5,\nb.m4s\n#EXT-X-ENDLIST"
	m, out, urls := relayTestRewrite(t, body, true)
	if len(urls) != 2 || m.stats.skipped != 1 {
		t.Fatalf("%s", out)
	}
	var state []string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "#EXT-X-KEY:") || strings.HasPrefix(line, "#EXT-X-MAP:") {
			match := hlsURI.FindStringSubmatch(line)
			if len(match) > 1 {
				g, _ := m.grant(mediaTicket(match[1]))
				state = append(state, g.URL)
			}
		}
	}
	joined := strings.Join(state, ",")
	for _, want := range []string{"a.key", "a.mp4", "c.key", "c.mp4", "d.key"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing state %s: %v", want, state)
		}
	}
	if strings.Contains(joined, "ad.mp4") {
		t.Fatal("removed ad map retained")
	}
	if !strings.Contains(joined, "c.key,https://media.example.org/vod/c.mp4,https://media.example.org/vod/d.key") {
		t.Fatalf("map encryption order incorrect: %v", state)
	}
}
func TestRelayPlaylistSecurityAndMaster(t *testing.T) {
	m := newMediaRelay(nil)
	defer m.close()
	for _, body := range []string{"#EXTM3U\n#EXT-X-CUE-OUT:5\n#EXTINF:5,\nhttp://127.0.0.1/secret\n#EXTINF:5,\nhttps://media.example.org/b.ts\n#EXT-X-ENDLIST", "#EXTM3U\n#EXT-X-KEY:METHOD=AES-128,URI=\"http://10.0.0.1/key\"\n#EXTINF:5,\nhttps://media.example.org/b.ts\n#EXT-X-ENDLIST"} {
		if _, e := m.rewritePlaylist([]byte(body), "https://media.example.org/index.m3u8", nil); e == nil {
			t.Fatal("private URL bypassed filtering")
		}
	}
	_, out, urls := relayTestRewrite(t, "#EXTM3U\n#EXT-X-MEDIA:TYPE=AUDIO,URI=\"audio.m3u8\"\n#EXT-X-STREAM-INF:BANDWIDTH=100000\nvideo.m3u8", true)
	if len(urls) != 1 || !strings.Contains(out, `URI="/api/media?ticket=`) {
		t.Fatalf("master rewrite: %s", out)
	}
}

func TestRelayLeadingAdsPreserveSequenceAndStartOffset(t *testing.T) {
	body := "#EXTM3U\n#EXT-X-MEDIA-SEQUENCE:100\n#EXT-X-DISCONTINUITY-SEQUENCE:9\n#EXT-X-START:TIME-OFFSET=7,PRECISE=YES\n#EXT-X-CUE-OUT:5\n#EXT-X-DISCONTINUITY\n#EXTINF:5,\nad.ts\n#EXT-X-DISCONTINUITY\n#EXTINF:6,\nmain.ts\n#EXT-X-ENDLIST"
	_, out, urls := relayTestRewrite(t, body, true)
	if len(urls) != 1 || !strings.Contains(out, "#EXT-X-MEDIA-SEQUENCE:101") || !strings.Contains(out, "#EXT-X-DISCONTINUITY-SEQUENCE:11") || !strings.Contains(out, "#EXT-X-START:TIME-OFFSET=2.000,PRECISE=YES") {
		t.Fatalf("leading timeline state incorrect:\n%s", out)
	}
}
func TestRelayLiveURLIsNotForwardCached(t *testing.T) {
	m, _, _ := relayTestRewrite(t, "#EXTM3U\n#EXT-X-MEDIA-SEQUENCE:55\n#EXTINF:6,\nmutable.ts", true)
	for _, g := range m.grants {
		if g.Segment {
			t.Fatal("mutable live segment entered finite-media cache")
		}
	}
	if len(m.plans) != 0 {
		t.Fatal("live prefetch plan created")
	}
}
