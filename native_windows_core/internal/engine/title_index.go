// SPDX-License-Identifier: GPL-3.0-or-later
package engine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode"

	"github.com/mozillazg/go-pinyin"
)

const titleIndexLimit = 2000
const titleIndexBytes = 2 << 20

// This index contains observed metadata only. It never guesses a Chinese title
// from arbitrary Latin letters, downloads entire catalogs, or stores media URLs.
type titleRecord struct {
	Title    string `json:"title"`
	Year     string `json:"year,omitempty"`
	FilmID   string `json:"film_id,omitempty"`
	Seen     uint64 `json:"seen"`
	forms    []string
	initials []string
}
type TitleIndex struct {
	mu       sync.Mutex
	path     string
	entries  map[string]titleRecord
	sequence uint64
	closed   bool
}

func searchNorm(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, widthNorm(normalizeTitle(s)))
}

func titleForms(title string) ([]string, []string) {
	a := pinyin.NewArgs()
	a.Heteronym = true
	a.Fallback = func(r rune, _ pinyin.Args) []string { return []string{string(r)} }
	parts := pinyin.Pinyin(searchNorm(title), a)
	full, initials := []string{""}, []string{""}
	for _, choices := range parts {
		if len(choices) > 2 {
			choices = choices[:2]
		}
		next, first := []string{}, []string{}
		for i, prefix := range full {
			for _, choice := range choices {
				choice = searchNorm(choice)
				if choice == "" {
					continue
				}
				next = append(next, prefix+choice)
				first = append(first, initials[i]+string([]rune(choice)[0]))
				if len(next) >= 16 {
					break
				}
			}
			if len(next) >= 16 {
				break
			}
		}
		full, initials = next, first
	}
	return full, initials
}

func newTitleIndex(dir string) *TitleIndex {
	x := &TitleIndex{path: filepath.Join(dir, "title_index.json"), entries: map[string]titleRecord{}}
	if st, e := os.Stat(x.path); e == nil && st.Size() <= titleIndexBytes {
		if raw, e := os.ReadFile(x.path); e == nil {
			var records []titleRecord
			if json.Unmarshal(raw, &records) == nil && len(records) <= titleIndexLimit {
				for _, r := range records {
					if r.Title == "" || len([]rune(r.Title)) > 200 || len(r.Year) > 20 || len(r.FilmID) > 512 {
						continue
					}
					r.forms, r.initials = titleForms(r.Title)
					x.entries[searchNorm(r.Title)+"\x00"+r.Year] = r
					if r.Seen > x.sequence {
						x.sequence = r.Seen
					}
				}
			}
		}
	}
	return x
}

func (x *TitleIndex) observe(items []Obj) {
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.closed {
		return
	}
	for _, item := range items {
		title := text(item["title"], 200)
		key := searchNorm(title)
		if key == "" {
			continue
		}
		year := text(item["year"], 20)
		x.sequence++
		r := titleRecord{Title: title, Year: year, FilmID: runeLimit(sget(item, "id"), 512), Seen: x.sequence}
		r.forms, r.initials = titleForms(title)
		x.entries[key+"\x00"+year] = r
	}
	records := make([]titleRecord, 0, len(x.entries))
	for _, r := range x.entries {
		records = append(records, r)
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].Seen != records[j].Seen {
			return records[i].Seen > records[j].Seen
		}
		return records[i].Title < records[j].Title
	})
	if len(records) > titleIndexLimit {
		records = records[:titleIndexLimit]
	}
	// The byte bound also holds for unusually long Unicode IDs and titles.
	for len(records) > 0 {
		raw, e := json.Marshal(records)
		if e == nil && len(raw) <= titleIndexBytes {
			_ = atomicBytes(x.path, raw)
			break
		}
		records = records[:len(records)-1]
	}
	x.entries = map[string]titleRecord{}
	for _, r := range records {
		x.entries[searchNorm(r.Title)+"\x00"+r.Year] = r
	}
}

// cappedDistance is O(len(a)*cap), and refuses long or unrelated strings before
// allocating. It supports one/two missing or mistyped characters from TV input.
func cappedDistance(a, b string, cap int) int {
	aa, bb := []rune(a), []rune(b)
	if len(aa) > 120 || len(bb) > 200 || absInt(len(aa)-len(bb)) > cap {
		return cap + 1
	}
	prev := make([]int, len(bb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(aa); i++ {
		cur := make([]int, len(bb)+1)
		for j := range cur {
			cur[j] = cap + 1
		}
		cur[0] = i
		lo, hi := max(1, i-cap), min(len(bb), i+cap)
		for j := lo; j <= hi; j++ {
			cost := 0
			if aa[i-1] != bb[j-1] {
				cost = 1
			}
			cur[j] = min(cur[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(bb)]
}
func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func titleScore(q string, r titleRecord) (int, string) {
	t := searchNorm(r.Title)
	if q == t {
		return 1000, "title"
	}
	if strings.HasPrefix(t, q) {
		return 900, "title_prefix"
	}
	if strings.Contains(t, q) {
		return 800, "title_contains"
	}
	for _, f := range r.forms {
		if q == f {
			return 970, "pinyin"
		}
	}
	for _, f := range r.initials {
		if q == f && len(q) >= 2 {
			return 930, "initials"
		}
	}
	if len(q) >= 3 {
		for _, f := range r.forms {
			if strings.HasPrefix(f, q) {
				return 850, "pinyin_prefix"
			}
		}
		for _, f := range r.initials {
			if strings.HasPrefix(f, q) {
				return 810, "initials_prefix"
			}
		}
	}
	length := len([]rune(q))
	if length >= 3 {
		cap := 1
		if length >= 7 {
			cap = 2
		}
		if d := cappedDistance(q, t, cap); d <= cap {
			return 720 - d*20, "fuzzy_title"
		}
		if length >= 5 {
			for _, f := range r.forms {
				if d := cappedDistance(q, f, cap); d <= cap {
					return 700 - d*20, "fuzzy_pinyin"
				}
			}
		}
		if length >= 4 {
			for _, f := range r.initials {
				if cappedDistance(q, f, 1) <= 1 {
					return 640, "fuzzy_initials"
				}
			}
		}
	}
	return 0, ""
}

func (x *TitleIndex) suggest(query string) Obj {
	q := searchNorm(query)
	result := Obj{"original_query": query, "resolved_query": query, "resolved": false, "scope": "local_metadata", "hint": "支持已搜索、收藏及历史片名的全拼、首字母和近似输入", "items": []Obj{}}
	if q == "" || len([]rune(q)) > 120 {
		return result
	}
	x.mu.Lock()
	type match struct {
		r     titleRecord
		score int
		kind  string
	}
	matches := []match{}
	for _, r := range x.entries {
		if score, kind := titleScore(q, r); score > 0 {
			matches = append(matches, match{r, score, kind})
		}
	}
	x.mu.Unlock()
	sort.Slice(matches, func(i, j int) bool {
		a, b := matches[i], matches[j]
		if a.score != b.score {
			return a.score > b.score
		}
		if a.r.Title != b.r.Title {
			return a.r.Title < b.r.Title
		}
		return a.r.Year > b.r.Year
	})
	items := []Obj{}
	seen := map[string]bool{}
	for _, m := range matches {
		key := searchNorm(m.r.Title)
		if seen[key] {
			continue
		}
		seen[key] = true
		items = append(items, Obj{"title": m.r.Title, "query": m.r.Title, "year": m.r.Year, "film_id": m.r.FilmID, "match": m.kind, "score": m.score})
		if len(items) >= 8 {
			break
		}
	}
	result["items"] = items
	if len(items) > 0 {
		kind := sget(items[0], "match")
		score, _ := numValue(items[0]["score"])
		second := float64(0)
		if len(items) > 1 {
			second, _ = numValue(items[1]["score"])
		}
		if (kind == "pinyin" || kind == "initials") && score > second || kind == "pinyin_prefix" && len(q) >= 5 && score-second >= 80 {
			result["resolved_query"], result["resolved"] = sget(items[0], "title"), true
		}
	}
	return result
}
func (x *TitleIndex) close() { x.mu.Lock(); x.closed = true; x.mu.Unlock() }

func (s *Service) SearchSuggest(ctx context.Context, query string, online bool) (Obj, error) {
	query = strings.TrimSpace(query)
	if len([]rune(query)) > 120 {
		return nil, problem("请输入 1–120 个字符的片名。", "query")
	}
	return s.suggestQuery(ctx, query, online), nil
}

func (s *Service) searchQuery(ctx context.Context, query string) (string, Obj) {
	info := s.suggestQuery(ctx, query, true)
	return sget(info, "resolved_query"), info
}

func searchInfo(result, info Obj) Obj {
	for _, k := range []string{"original_query", "resolved_query", "resolved", "scope", "hint"} {
		result[k] = info[k]
	}
	return result
}
