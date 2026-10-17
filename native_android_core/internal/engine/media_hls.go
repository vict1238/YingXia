// SPDX-License-Identifier: GPL-3.0-or-later
package engine

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

var hlsURI = re(`URI="([^"]+)"`)

type relayHLSSegment struct {
	lines                              []string
	uri, key, init, initKey, byteRange string
	duration                           float64
	sequence                           uint64
	date                               time.Time
	discontinuities                    int
}
type relayHLS struct {
	header, tail                    []string
	segments                        []relayHLSSegment
	sequence, discontinuitySequence uint64
	finite, safe                    bool
}

func hlsGlobal(s string) bool {
	for _, p := range []string{"#EXTM3U", "#EXT-X-VERSION:", "#EXT-X-TARGETDURATION:", "#EXT-X-MEDIA-SEQUENCE:", "#EXT-X-DISCONTINUITY-SEQUENCE:", "#EXT-X-PLAYLIST-TYPE:", "#EXT-X-INDEPENDENT-SEGMENTS", "#EXT-X-START:", "#EXT-X-ALLOW-CACHE:"} {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}
func hlsNumber(s, prefix string) float64 {
	v := strings.TrimSpace(strings.TrimPrefix(s, prefix))
	v, _, _ = strings.Cut(v, ",")
	n, _ := strconv.ParseFloat(v, 64)
	if math.IsNaN(n) || math.IsInf(n, 0) || n < 0 {
		return 0
	}
	return n
}
func hlsByteRange(raw string, previous int64, same bool) (string, int64, bool) {
	n, o, explicit := strings.Cut(raw, "@")
	length, e := strconv.ParseInt(n, 10, 64)
	if e != nil || length <= 0 || length > 1<<50 {
		return "", 0, false
	}
	start := previous
	if explicit {
		start, e = strconv.ParseInt(o, 10, 64)
	} else if !same {
		return "", 0, false
	}
	if e != nil || start < 0 || start > 1<<50 || start+length > 1<<51 {
		return "", 0, false
	}
	return fmt.Sprintf("bytes=%d-%d", start, start+length-1), start + length, true
}
func parseRelayHLS(lines []string) relayHLS {
	p := relayHLS{safe: true}
	var pending []string
	key, init, initKey, rawRange, priorURI := "", "", "", "", ""
	previousEnd := int64(0)
	duration := float64(0)
	var date time.Time
	discontinuities := 0
	hasInf := false
	for _, raw := range lines {
		s := strings.TrimSpace(raw)
		if s == "" {
			continue
		}
		if strings.HasPrefix(s, "#") {
			switch {
			case strings.HasPrefix(s, "#EXT-X-STREAM-INF:"), strings.HasPrefix(s, "#EXT-X-I-FRAMES-ONLY"), strings.HasPrefix(s, "#EXT-X-PART"), strings.HasPrefix(s, "#EXT-X-SKIP:"), strings.HasPrefix(s, "#EXT-X-DEFINE:"):
				p.safe = false
			case strings.HasPrefix(s, "#EXT-X-MEDIA-SEQUENCE:"):
				var e error
				p.sequence, e = strconv.ParseUint(strings.TrimSpace(strings.TrimPrefix(s, "#EXT-X-MEDIA-SEQUENCE:")), 10, 64)
				if e != nil {
					p.safe = false
				}
			case strings.HasPrefix(s, "#EXT-X-DISCONTINUITY-SEQUENCE:"):
				var e error
				p.discontinuitySequence, e = strconv.ParseUint(strings.TrimSpace(strings.TrimPrefix(s, "#EXT-X-DISCONTINUITY-SEQUENCE:")), 10, 64)
				if e != nil {
					p.safe = false
				}
			case s == "#EXT-X-ENDLIST":
				p.finite = true
			case strings.HasPrefix(s, "#EXTINF:"):
				duration = hlsNumber(s, "#EXTINF:")
				hasInf = duration > 0
				if !hasInf {
					p.safe = false
				}
			case strings.HasPrefix(s, "#EXT-X-KEY:"):
				key = s
				a := availabilityHLSAttrs(s)
				if a["METHOD"] != "NONE" && (a["METHOD"] != "AES-128" || (a["KEYFORMAT"] != "" && a["KEYFORMAT"] != "identity")) {
					p.safe = false
				}
			case strings.HasPrefix(s, "#EXT-X-MAP:"):
				init = s
				initKey = key
				a := availabilityHLSAttrs(s)
				if a["BYTERANGE"] != "" && !strings.Contains(a["BYTERANGE"], "@") {
					p.safe = false
				}
				if key != "" {
					a = availabilityHLSAttrs(key)
					if a["METHOD"] == "AES-128" && a["IV"] == "" {
						p.safe = false
					}
				}
			case strings.HasPrefix(s, "#EXT-X-BYTERANGE:"):
				rawRange = strings.TrimSpace(strings.TrimPrefix(s, "#EXT-X-BYTERANGE:"))
			case strings.HasPrefix(s, "#EXT-X-PROGRAM-DATE-TIME:"):
				var e error
				date, e = time.Parse(time.RFC3339Nano, strings.TrimSpace(strings.TrimPrefix(s, "#EXT-X-PROGRAM-DATE-TIME:")))
				if e != nil {
					p.safe = false
				}
			case s == "#EXT-X-DISCONTINUITY":
				discontinuities++
			}
			if hlsGlobal(s) {
				p.header = append(p.header, s)
			} else {
				pending = append(pending, s)
			}
			continue
		}
		if !hasInf {
			p.safe = false
		}
		br := ""
		if rawRange != "" {
			var ok bool
			br, previousEnd, ok = hlsByteRange(rawRange, previousEnd, priorURI == s)
			if !ok {
				p.safe = false
			}
		} else {
			previousEnd = 0
		}
		seg := relayHLSSegment{lines: pending, uri: s, key: key, init: init, initKey: initKey, byteRange: br, duration: duration, sequence: p.sequence + uint64(len(p.segments)), date: date, discontinuities: discontinuities}
		p.segments = append(p.segments, seg)
		pending = nil
		priorURI = s
		rawRange = ""
		duration = 0
		hasInf = false
		discontinuities = 0
		if !date.IsZero() {
			date = date.Add(time.Duration(seg.duration * float64(time.Second)))
		}
	}
	p.tail = pending
	return p
}
func cueOut(s string) (bool, float64) {
	if s == "#EXT-X-CUE-OUT" {
		return true, 0
	}
	if strings.HasPrefix(s, "#EXT-X-CUE-OUT:") {
		a := availabilityHLSAttrs(s)
		if v := a["DURATION"]; v != "" {
			n, _ := strconv.ParseFloat(v, 64)
			return true, n
		}
		return true, hlsNumber(s, "#EXT-X-CUE-OUT:")
	}
	return false, 0
}
func hlsAdTag(s string) bool {
	return strings.HasPrefix(s, "#EXT-X-CUE-") || strings.HasPrefix(s, "#EXT-OATCLS-SCTE35:") || strings.HasPrefix(s, "#EXT-X-SCTE35:")
}
func relayAdSegments(p relayHLS) ([]bool, int, float64) {
	drop := make([]bool, len(p.segments))
	start := -1
	duration := float64(0)
	elapsed := float64(0)
	mark := func(a, b int) {
		if a < 0 || b <= a {
			return
		}
		sum := float64(0)
		for i := a; i < b; i++ {
			sum += p.segments[i].duration
		}
		if sum > 600 {
			return
		}
		for i := a; i < b; i++ {
			drop[i] = true
		}
	}
	for i, s := range p.segments {
		for _, line := range s.lines {
			if line == "#EXT-X-CUE-IN" || strings.HasPrefix(line, "#EXT-X-CUE-IN:") {
				if start >= 0 {
					mark(start, i)
				}
				start = -1
				duration = 0
				elapsed = 0
			}
			if out, d := cueOut(line); out {
				start = i
				duration = d
				elapsed = 0
			}
		}
		if start >= 0 {
			elapsed += s.duration
		}
		if start >= 0 && duration > 0 && duration <= 600 {
			if elapsed >= duration-0.05 {
				if math.Abs(elapsed-duration) <= 0.15 {
					mark(start, i+1)
				}
				start = -1
				duration = 0
				elapsed = 0
			}
		}
	}
	for _, line := range p.tail {
		if line == "#EXT-X-CUE-IN" || strings.HasPrefix(line, "#EXT-X-CUE-IN:") {
			if start >= 0 {
				mark(start, len(p.segments))
			}
			start = -1
		}
	}
	// DATERANGE is interpreted only with an explicit ad class or SCTE35-OUT,
	// a complete date interval, and segment timestamps. Interstitial asset
	// declarations alone do not imply that the main-content segments are ads.
	ranges := map[string]map[string]string{}
	all := append([]string(nil), p.header...)
	for _, s := range p.segments {
		all = append(all, s.lines...)
	}
	all = append(all, p.tail...)
	for _, line := range all {
		if strings.HasPrefix(line, "#EXT-X-DATERANGE:") {
			a := availabilityHLSAttrs(line)
			id := a["ID"]
			if id == "" {
				continue
			}
			if ranges[id] == nil {
				ranges[id] = map[string]string{}
			}
			for k, v := range a {
				ranges[id][k] = v
			}
		}
	}
	for _, a := range ranges {
		class := strings.ToLower(a["CLASS"])
		ad := a["SCTE35-OUT"] != "" || class == "ad" || class == "ads" || class == "ad-break" || class == "advertisement"
		if !ad || a["X-ASSET-URI"] != "" || a["X-ASSET-LIST"] != "" {
			continue
		}
		begin, e := time.Parse(time.RFC3339Nano, a["START-DATE"])
		if e != nil {
			continue
		}
		end, e := time.Parse(time.RFC3339Nano, a["END-DATE"])
		if e != nil {
			d, _ := strconv.ParseFloat(a["DURATION"], 64)
			if d <= 0 || d > 600 || math.IsNaN(d) || math.IsInf(d, 0) {
				continue
			}
			end = begin.Add(time.Duration(d * float64(time.Second)))
		}
		if !end.After(begin) || end.Sub(begin) > 600*time.Second {
			continue
		}
		for i, s := range p.segments {
			if !s.date.IsZero() && !s.date.Before(begin.Add(-time.Millisecond)) && !s.date.Add(time.Duration(s.duration*float64(time.Second))).After(end.Add(time.Millisecond)) {
				drop[i] = true
			}
		}
	}
	count := 0
	seconds := float64(0)
	for i, v := range drop {
		if v {
			count++
			seconds += p.segments[i].duration
		}
	}
	if count == len(p.segments) {
		return make([]bool, len(drop)), 0, 0
	}
	return drop, count, seconds
}
func normalizedRelayHLS(p relayHLS, drop []bool) []string {
	first := 0
	for first < len(drop) && drop[first] {
		first++
	}
	header := append([]string(nil), p.header...)
	seq := p.sequence + uint64(first)
	disc := p.discontinuitySequence
	for i := 0; i < first; i++ {
		disc += uint64(p.segments[i].discontinuities)
	}
	if first < len(p.segments) {
		disc += uint64(p.segments[first].discontinuities)
	}
	hasSeq, hasDisc := false, false
	for i, line := range header {
		if strings.HasPrefix(line, "#EXT-X-START:") {
			header[i] = adjustRelayStart(line, p.segments, drop)
		}
		if strings.HasPrefix(line, "#EXT-X-MEDIA-SEQUENCE:") {
			header[i] = fmt.Sprintf("#EXT-X-MEDIA-SEQUENCE:%d", seq)
			hasSeq = true
		}
		if strings.HasPrefix(line, "#EXT-X-DISCONTINUITY-SEQUENCE:") {
			header[i] = fmt.Sprintf("#EXT-X-DISCONTINUITY-SEQUENCE:%d", disc)
			hasDisc = true
		}
	}
	if !hasSeq {
		header = append(header, fmt.Sprintf("#EXT-X-MEDIA-SEQUENCE:%d", seq))
	}
	if !hasDisc && disc > 0 {
		header = append(header, fmt.Sprintf("#EXT-X-DISCONTINUITY-SEQUENCE:%d", disc))
	}
	result := header
	priorKey, priorMap := "", ""
	previous := -1
	for i, s := range p.segments {
		if drop[i] {
			continue
		}
		// A splice needs a decoder/timestamp boundary. Ordinary source
		// discontinuities are retained; no timestamp bytes are modified.
		if previous >= 0 && (i != previous+1 || s.discontinuities > 0) {
			for n := 0; n < max(1, s.discontinuities); n++ {
				result = append(result, "#EXT-X-DISCONTINUITY")
			}
		}
		key := s.key
		if key != "" {
			a := availabilityHLSAttrs(key)
			if a["METHOD"] == "AES-128" && a["IV"] == "" {
				key += fmt.Sprintf(",IV=0x%032x", s.sequence)
			}
		}
		if s.init != priorMap {
			if s.init != "" {
				if s.initKey != "" && s.initKey != priorKey {
					result = append(result, s.initKey)
					priorKey = s.initKey
				}
				result = append(result, s.init)
			}
			priorMap = s.init
		}
		if key != priorKey {
			if key != "" {
				result = append(result, key)
			}
			priorKey = key
		}
		for _, line := range s.lines {
			if hlsAdTag(line) || strings.HasPrefix(line, "#EXT-X-KEY:") || strings.HasPrefix(line, "#EXT-X-MAP:") || strings.HasPrefix(line, "#EXT-X-BYTERANGE:") || line == "#EXT-X-DISCONTINUITY" {
				continue
			}
			if strings.HasPrefix(line, "#EXT-X-DATERANGE:") {
				a := availabilityHLSAttrs(line)
				class := strings.ToLower(a["CLASS"])
				if a["SCTE35-OUT"] != "" || a["SCTE35-IN"] != "" || class == "ad" || class == "ads" || class == "ad-break" || class == "advertisement" {
					continue
				}
			}
			result = append(result, line)
		}
		if s.byteRange != "" {
			raw := strings.TrimPrefix(s.byteRange, "bytes=")
			a, b, _ := strings.Cut(raw, "-")
			x, _ := strconv.ParseInt(a, 10, 64)
			y, _ := strconv.ParseInt(b, 10, 64)
			result = append(result, fmt.Sprintf("#EXT-X-BYTERANGE:%d@%d", y-x+1, x))
		}
		result = append(result, s.uri)
		previous = i
	}
	for _, line := range p.tail {
		if !hlsAdTag(line) && !strings.HasPrefix(line, "#EXT-X-DATERANGE:") {
			result = append(result, line)
		}
	}
	return result
}

func adjustRelayStart(line string, segments []relayHLSSegment, drop []bool) string {
	a := availabilityHLSAttrs(line)
	offset, e := strconv.ParseFloat(a["TIME-OFFSET"], 64)
	if e != nil || math.IsNaN(offset) || math.IsInf(offset, 0) {
		return line
	}
	total := float64(0)
	for _, s := range segments {
		total += s.duration
	}
	target := offset
	if target < 0 {
		target = total + target
	}
	target = max(0, min(total, target))
	original, retained := float64(0), float64(0)
	for i, s := range segments {
		if target <= original+s.duration {
			if !drop[i] {
				retained += target - original
			}
			break
		}
		original += s.duration
		if !drop[i] {
			retained += s.duration
		}
	}
	value := "#EXT-X-START:TIME-OFFSET=" + strconv.FormatFloat(retained, 'f', 3, 64)
	if a["PRECISE"] != "" {
		value += ",PRECISE=" + a["PRECISE"]
	}
	return value
}

func (m *MediaRelay) rewritePlaylist(body []byte, base string, headers map[string]string) ([]byte, error) {
	if len(body) > 4000000 || !strings.HasPrefix(strings.TrimSpace(string(body)), "#EXTM3U") {
		return nil, problem("HLS 播放列表无效。", "hls", 502)
	}
	lines := strings.Split(strings.ReplaceAll(string(body), "\r\n", "\n"), "\n")
	if len(lines) > 32000 {
		return nil, problem("HLS 播放列表条目过多。", "hls", 502)
	}
	// Validate the original manifest before filtering: ad metadata may not hide
	// a private URL or invalid origin from the existing security boundary.
	for _, line := range lines {
		s := strings.TrimSpace(line)
		if s == "" {
			continue
		}
		if strings.HasPrefix(s, "#") {
			for _, match := range hlsURI.FindAllStringSubmatch(line, -1) {
				if _, e := ruleURL(base, match[1]); e != nil {
					return nil, e
				}
			}
		} else if _, e := ruleURL(base, s); e != nil {
			return nil, e
		}
	}
	p := parseRelayHLS(lines)
	m.mu.Lock()
	enabled := m.adSkip
	m.mu.Unlock()
	if enabled && p.safe && p.finite && len(p.segments) > 0 {
		drop, count, seconds := relayAdSegments(p)
		if count > 0 {
			lines = normalizedRelayHLS(p, drop)
			p = parseRelayHLS(lines)
			m.mu.Lock()
			m.stats.skipped += uint64(count)
			m.stats.skippedSeconds += seconds
			m.mu.Unlock()
		}
	} else if enabled && len(p.segments) > 0 {
		m.mu.Lock()
		m.stats.deferred++
		m.mu.Unlock()
	}
	// Associate only EXTINF media segments with the bounded forward window.
	ranges := map[string][]string{}
	for _, s := range p.segments {
		// Sliding live windows may reuse segment URLs. Their original fetch
		// semantics are retained rather than caching a mutable URL for 90 s.
		if p.finite && s.duration > 0 {
			ranges[s.uri] = append(ranges[s.uri], s.byteRange)
		}
	}
	var ids []string
	for i, line := range lines {
		s := strings.TrimSpace(line)
		if s == "" {
			continue
		}
		if strings.HasPrefix(s, "#") {
			var failure error
			lines[i] = hlsURI.ReplaceAllStringFunc(line, func(match string) string {
				parts := hlsURI.FindStringSubmatch(match)
				u, e := ruleURL(base, parts[1])
				if e != nil {
					failure = e
					return match
				}
				ticket, e := m.register(u, headers)
				if e != nil {
					failure = e
					return match
				}
				return `URI="` + ticket + `"`
			})
			if failure != nil {
				return nil, failure
			}
		} else {
			u, e := ruleURL(base, s)
			if e != nil {
				return nil, e
			}
			br := ""
			segment := false
			if r := ranges[s]; len(r) > 0 {
				segment = true
				br = r[0]
				ranges[s] = r[1:]
			}
			ticket, e := m.registerMedia(u, headers, segment, br)
			if e != nil {
				return nil, e
			}
			lines[i] = ticket
			if segment {
				ids = append(ids, mediaTicket(ticket))
			}
		}
	}
	m.mu.Lock()
	for i, id := range ids {
		end := min(i+3, len(ids))
		m.plans[id] = append([]string(nil), ids[i+1:end]...)
	}
	m.stats.playlists++
	m.mu.Unlock()
	return []byte(strings.Join(lines, "\n")), nil
}
