// SPDX-License-Identifier: GPL-3.0-or-later
// Native backend for the original YingXia v1.0 UI; no external Go modules.
package engine

import (
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

type Obj = map[string]any

type AppError struct {
	Message string
	Code    string
	Status  int
}

func (e *AppError) Error() string { return e.Message }
func problem(message, code string, status ...int) error {
	s := 400
	if len(status) > 0 {
		s = status[0]
	}
	return &AppError{message, code, s}
}
func str(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case json.Number:
		return string(x)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	default:
		return fmt.Sprint(v)
	}
}
func sget(d Obj, k string) string { return str(d[k]) }
func defaultStr(d Obj, k, v string) string {
	if x, ok := d[k]; ok {
		return str(x)
	}
	return v
}
func re(pattern string) *regexp.Regexp { return regexp.MustCompile(pattern) }
func runeLimit(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

var hideHTML = re(`(?is)<(?:script|style)\b[^>]*>.*?</(?:script|style)\s*>`)
var stripHTML = re(`(?s)<[^>]*>`)
var breakHTML = re(`(?i)</?(?:p|br|div|li)\b[^>]*>`)

func text(v any, n int) string {
	s := runeLimit(str(v), 20000)
	s = hideHTML.ReplaceAllString(s, "")
	s = breakHTML.ReplaceAllString(s, " ")
	s = stripHTML.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	return runeLimit(strings.Join(strings.Fields(s), " "), n)
}
func widthNorm(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 0xff01 && r <= 0xff5e {
			return r - 0xfee0
		}
		if r == 0x3000 || r == 0xa0 {
			return ' '
		}
		return r
	}, s)
}

const nums = `[0-9零〇一二两三四五六七八九十百千]+`
const sites = `(?:腾讯视频|爱奇艺|优酷|芒果TV|哔哩哔哩|bilibili|搜狐视频|腾讯|B站)`

var trailingSite = re(`(?i)\s*[-_|｜·:：]\s*` + sites + `(?:\s*[-_|｜].*)?$`)
var leadingSite = re(`(?i)^` + sites + `\s*[-_|｜:：]\s*`)
var epInTitle = re(`第\s*` + nums + `\s*[集话期](?:\s*[上下中])?`)
var titleBoiler = re(`\s*[-_|｜]\s*(?:在线观看|高清正版|免费观看|完整版|全集)\s*$`)

func normalizeTitle(v string) string {
	s := widthNorm(text(v, 250))
	s = trailingSite.ReplaceAllString(s, "")
	s = leadingSite.ReplaceAllString(s, "")
	s = epInTitle.ReplaceAllString(s, "")
	s = titleBoiler.ReplaceAllString(s, "")
	s = strings.NewReplacer("《", "", "》", "").Replace(s)
	return strings.Join(strings.Fields(strings.Trim(s, " -_|｜")), " ")
}
func number(v string) *int {
	v = widthNorm(strings.TrimSpace(v))
	if v == "" {
		return nil
	}
	if n, e := strconv.Atoi(v); e == nil {
		if n >= 0 && n < 100000 {
			return &n
		}
		return nil
	}
	d := map[rune]int{'零': 0, '〇': 0, '一': 1, '二': 2, '两': 2, '三': 3, '四': 4, '五': 5, '六': 6, '七': 7, '八': 8, '九': 9}
	u := map[rune]int{'十': 10, '百': 100, '千': 1000}
	hasUnit := false
	for _, r := range v {
		if _, ok := u[r]; ok {
			hasUnit = true
		} else if _, ok := d[r]; !ok {
			return nil
		}
	}
	sum, current := 0, 0
	for _, r := range v {
		if n, ok := d[r]; ok {
			if !hasUnit {
				sum = sum*10 + n
			} else {
				current = n
			}
		} else {
			if current == 0 {
				current = 1
			}
			sum += current * u[r]
			current = 0
		}
	}
	sum += current
	if sum >= 100000 {
		return nil
	}
	return &sum
}

var seasonCN = re(`第\s*(` + nums + `)\s*季`)
var seasonEN = re(`(?i)\b(?:season\s*|s)(\d{1,3})\b`)

func season(v string) *int {
	for _, r := range []*regexp.Regexp{seasonCN, seasonEN} {
		if m := r.FindStringSubmatch(v); m != nil {
			return number(m[1])
		}
	}
	return nil
}

var specialEp = re(`(?i)预告|花絮|番外|特别|\bSP\b|trailer|special`)
var epCN = re(`第\s*(` + nums + `)\s*[集话期]`)
var epEN = re(`(?i)\b(?:episode|ep|e)\s*0*(\d{1,4})\b`)
var epOnly = re(`^(` + nums + `)(?:\s*[集话期])?$`)

func episodeNumber(label string) *int {
	label = widthNorm(text(label, 200))
	if specialEp.MatchString(label) {
		return nil
	}
	for _, r := range []*regexp.Regexp{epCN, epEN, epOnly} {
		if m := r.FindStringSubmatch(label); m != nil {
			return number(m[1])
		}
	}
	return nil
}

var yearRE = re(`\b(19\d{2}|20\d{2})\b`)

func ranking(query, title, year string) int {
	q, t := strings.ToLower(normalizeTitle(query)), strings.ToLower(normalizeTitle(title))
	qs, ts := season(q), season(t)
	score := 0
	if q == t {
		score = 100
	} else if strings.Contains(t, q) {
		score = 65
	} else if strings.Contains(q, t) {
		score = 30
	}
	if qs != nil && (ts == nil || *ts != *qs) {
		score -= 90
	}
	if y := yearRE.FindString(query); y != "" && year != "" {
		if y == year {
			score += 15
		} else {
			score -= 30
		}
	}
	return score
}

type Episode struct {
	Key     string `json:"key"`
	Name    string `json:"name"`
	URL     string `json:"-"`
	Ordinal *int   `json:"ordinal"`
}
type Group struct {
	Key      string    `json:"key"`
	Name     string    `json:"name"`
	Episodes []Episode `json:"episodes"`
}
type Film struct {
	ProviderID   string  `json:"provider_id,omitempty"`
	ProviderKind string  `json:"provider_kind,omitempty"`
	ID           string  `json:"id"`
	Title        string  `json:"title"`
	Year         string  `json:"year"`
	Kind         string  `json:"kind"`
	Area         string  `json:"area"`
	Remarks      string  `json:"remarks"`
	Summary      string  `json:"summary"`
	Actors       string  `json:"actors"`
	Director     string  `json:"director"`
	Source       string  `json:"source"`
	Groups       []Group `json:"groups"`
	Demo         bool    `json:"demo"`
}

func (f Film) public() Obj {
	b, _ := json.Marshal(f)
	var o Obj
	_ = json.Unmarshal(b, &o)
	if o["groups"] == nil {
		o["groups"] = []any{}
	}
	return o
}
func groupsFromVOD(v, names string) []Group {
	out := []Group{}
	labels := strings.Split(names, "$$$")
	for gi, raw := range strings.Split(v, "$$$") {
		if gi >= 40 {
			break
		}
		eps := []Episode{}
		for ei, item := range strings.Split(raw, "#") {
			if ei >= 3000 {
				break
			}
			label, link, found := strings.Cut(item, "$")
			if !found {
				link = label
				label = ""
			}
			link = strings.TrimSpace(link)
			p, e := url.Parse(link)
			if e != nil || p.Hostname() == "" || p.User != nil || (p.Scheme != "http" && p.Scheme != "https") || strings.ContainsAny(link, "\n\r\x00\\") {
				continue
			}
			label = text(label, 120)
			if label == "" {
				label = fmt.Sprintf("条目 %d", ei+1)
			}
			eps = append(eps, Episode{fmt.Sprintf("e%d", ei), label, link, episodeNumber(label)})
		}
		if len(eps) > 0 {
			name := fmt.Sprintf("来源组 %d", gi+1)
			if gi < len(labels) && labels[gi] != "" {
				name = text(labels[gi], 100)
			}
			out = append(out, Group{fmt.Sprintf("g%d", gi), name, eps})
		}
	}
	return out
}

var idRE = re(`^\d{1,15}$`)

func filmFromVOD(row Obj) (Film, error) {
	id := sget(row, "vod_id")
	if id == "" {
		id = sget(row, "id")
	}
	if !idRE.MatchString(id) {
		return Film{}, problem("目录返回了无效影片编号。", "bad_catalog")
	}
	name := sget(row, "vod_name")
	if name == "" {
		name = sget(row, "name")
	}
	return Film{ID: id, Title: text(name, 200), Year: text(row["vod_year"], 20), Kind: text(row["type_name"], 50), Area: text(row["vod_area"], 50), Remarks: text(row["vod_remarks"], 100), Summary: text(row["vod_content"], 1500), Actors: text(row["vod_actor"], 300), Director: text(row["vod_director"], 150), Source: "无损云目录（未实播验证）", Groups: groupsFromVOD(sget(row, "vod_play_url"), sget(row, "vod_play_from"))}, nil
}

var ambiguousEp = re(`(?i)上|下|中|预告|花絮|番外|SP|特别`)

func chooseEpisode(g Group, name string) (Episode, error) {
	exact := []Episode{}
	for _, e := range g.Episodes {
		if e.Name == name {
			exact = append(exact, e)
		}
	}
	if len(exact) == 1 {
		return exact[0], nil
	}
	n := episodeNumber(name)
	if ambiguousEp.MatchString(name) {
		n = nil
	}
	same := []Episode{}
	for _, e := range g.Episodes {
		if n != nil && e.Ordinal != nil && *n == *e.Ordinal && !ambiguousEp.MatchString(e.Name) {
			same = append(same, e)
		}
	}
	if len(same) == 1 {
		return same[0], nil
	}
	return Episode{}, problem("此片源缺少对应集数，请重新选择。", "episode_missing")
}
func hasControl(s string) bool {
	for _, r := range s {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}
