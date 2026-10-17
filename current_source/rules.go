// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/antchfx/htmlquery"
	"github.com/antchfx/xpath"
	"github.com/ohler55/ojg/jp"
)

const supportedRuleAPI = 8

type RuleRequest struct {
	Method   string            `json:"method"`
	URL      string            `json:"url"`
	Headers  map[string]string `json:"headers"`
	Query    Obj               `json:"query"`
	BodyType string            `json:"bodyType"`
	Body     any               `json:"body"`
}
type RuleSearchAPI struct {
	Request    RuleRequest `json:"request"`
	ListPath   string      `json:"listPath"`
	NamePath   string      `json:"namePath"`
	SourcePath string      `json:"sourcePath"`
}
type RuleEpisodePage struct {
	URL   string `json:"url"`
	Query Obj    `json:"query"`
}
type RuleChapterAPI struct {
	Request          RuleRequest     `json:"request"`
	Format           string          `json:"format"`
	RoadsPath        string          `json:"roadsPath"`
	RoadNamePath     string          `json:"roadNamePath"`
	EpisodesPath     string          `json:"episodesPath"`
	EpisodeNamePath  string          `json:"episodeNamePath"`
	EpisodeURLPath   string          `json:"episodeUrlPath"`
	EpisodePage      RuleEpisodePage `json:"episodePage"`
	PlayFromPath     string          `json:"playFromPath"`
	PlayURLPath      string          `json:"playUrlPath"`
	RoadSeparator    string          `json:"roadSeparator"`
	EpisodeSeparator string          `json:"episodeSeparator"`
	NameURLSeparator string          `json:"nameUrlSeparator"`
}
type KazumiRule struct {
	API           string         `json:"api"`
	Name          string         `json:"name"`
	Version       string         `json:"version"`
	BaseURL       string         `json:"baseURL"`
	SearchURL     string         `json:"searchURL"`
	SearchList    string         `json:"searchList"`
	SearchName    string         `json:"searchName"`
	SearchResult  string         `json:"searchResult"`
	ChapterRoads  string         `json:"chapterRoads"`
	ChapterResult string         `json:"chapterResult"`
	SearchMode    string         `json:"searchMode"`
	ChapterMode   string         `json:"chapterMode"`
	UsePost       bool           `json:"usePost"`
	Referer       string         `json:"referer"`
	UserAgent     string         `json:"userAgent"`
	SearchAPI     RuleSearchAPI  `json:"searchApiConfig"`
	ChapterAPI    RuleChapterAPI `json:"chapterApiConfig"`
}
type RequestSpec struct {
	URL     string
	Method  string
	Headers map[string]string
	Body    []byte
	Limit   int
}
type RequestFetcher interface {
	Request(context.Context, RequestSpec) ([]byte, Obj, error)
}

func fetchRequest(ctx context.Context, n Fetcher, r RequestSpec) ([]byte, Obj, error) {
	if f, ok := n.(RequestFetcher); ok {
		return f.Request(ctx, r)
	}
	if r.Method == "" || r.Method == "GET" {
		return n.Fetch(ctx, r.URL, r.Limit)
	}
	return nil, nil, problem("此来源需要 POST 请求。", "request_method")
}
func validateRule(r KazumiRule) error {
	a, e := strconv.Atoi(r.API)
	if e != nil || a < 1 || a > supportedRuleAPI {
		return problem("规则版本需要升级程序支持。", "rule_api")
	}
	if _, e = publicURL(r.BaseURL); e != nil {
		return e
	}
	if r.Referer != "" {
		if _, e = publicURL(r.Referer); e != nil {
			return e
		}
	}
	if len(r.UserAgent) > 500 || hasControl(r.UserAgent) {
		return problem("规则请求头无效。", "rule")
	}
	if r.SearchMode == "api" {
		if e = validateRuleRequest(r.SearchAPI.Request); e != nil {
			return e
		}
		for _, p := range []string{r.SearchAPI.ListPath, r.SearchAPI.NamePath, r.SearchAPI.SourcePath} {
			if e = validJSONPath(p, false); e != nil {
				return e
			}
		}
	} else {
		if _, e = publicURL(strings.ReplaceAll(r.SearchURL, "@keyword", "test")); e != nil {
			return e
		}
		for _, s := range []string{r.SearchList, r.SearchName, r.SearchResult} {
			if len(s) == 0 || len(s) > 2048 {
				return problem("搜索规则无效。", "rule")
			}
			if _, e = xpath.Compile(relativeXPath(s)); e != nil {
				return problem("搜索规则格式不支持。", "rule")
			}
		}
	}
	if r.ChapterMode == "api" {
		if e = validateRuleRequest(r.ChapterAPI.Request); e != nil {
			return e
		}
		if r.ChapterAPI.Format == "delimited" {
			for _, p := range []string{r.ChapterAPI.PlayFromPath, r.ChapterAPI.PlayURLPath} {
				if e = validJSONPath(p, true); e != nil {
					return e
				}
			}
		} else {
			for _, p := range []string{r.ChapterAPI.RoadsPath, r.ChapterAPI.RoadNamePath, r.ChapterAPI.EpisodesPath, r.ChapterAPI.EpisodeNamePath, r.ChapterAPI.EpisodeURLPath} {
				if e = validJSONPath(p, true); e != nil {
					return e
				}
			}
		}
		if r.ChapterAPI.EpisodePage.URL != "" {
			u := substitute(r.ChapterAPI.EpisodePage.URL, map[string]string{"source": "test", "episodeUrl": "1", "episodeName": "episode", "roadName": "line"}, true)
			if _, e = publicURL(u); e != nil {
				return e
			}
		}
	} else {
		for _, s := range []string{r.ChapterRoads, r.ChapterResult} {
			if s == "" || len(s) > 2048 {
				return problem("选集规则无效。", "rule")
			}
			if _, e = xpath.Compile(relativeXPath(s)); e != nil {
				return problem("选集规则格式不支持。", "rule")
			}
		}
	}
	return nil
}
func validJSONPath(p string, empty bool) error {
	if p == "" && empty {
		return nil
	}
	if len(p) > 1024 {
		return problem("JSON 路径过长。", "rule")
	}
	if _, e := jp.ParseString(p); e != nil {
		return problem("JSON 路径格式不支持。", "rule")
	}
	return nil
}
func validateRuleRequest(r RuleRequest) error {
	if r.Method != "" && r.Method != "GET" && r.Method != "POST" {
		return problem("不支持的规则请求方法。", "rule")
	}
	u := substitute(r.URL, map[string]string{"keyword": "test", "source": "1", "episodeUrl": "1"}, true)
	if _, e := publicURL(u); e != nil {
		return e
	}
	if len(r.Headers) > 20 {
		return problem("规则请求头过多。", "rule")
	}
	for k, v := range r.Headers {
		if !allowedRuleHeader(k) || hasControl(v) || len(v) > 3000 {
			return problem("规则请求头无效。", "rule")
		}
	}
	if r.BodyType != "" && r.BodyType != "none" && r.BodyType != "json" && r.BodyType != "form" {
		return problem("不支持的规则请求格式。", "rule")
	}
	b, _ := json.Marshal(r.Body)
	if len(b) > 65536 {
		return problem("规则请求过大。", "rule")
	}
	return nil
}
func allowedRuleHeader(k string) bool {
	switch strings.ToLower(k) {
	case "user-agent", "referer", "origin", "accept", "content-type", "authorization", "apikey", "x-api-key":
		return true
	}
	return false
}
func relativeXPath(s string) string {
	if strings.TrimSpace(s) == "//" {
		return "."
	}
	parts := strings.Split(s, "|")
	for i, p := range parts {
		p = strings.TrimSpace(p)
		if strings.HasPrefix(p, "/") {
			p = "." + p
		}
		parts[i] = p
	}
	return strings.Join(parts, " | ")
}
func substitute(s string, vars map[string]string, escape bool) string {
	for _, key := range []string{"episodeName", "episodeUrl", "roadName", "keyword", "source"} {
		v, ok := vars[key]
		if ok {
			if escape {
				v = quote(v)
			}
			s = strings.ReplaceAll(s, "@"+key, v)
		}
	}
	return s
}
func substituteValue(v any, vars map[string]string) any {
	switch x := v.(type) {
	case string:
		return substitute(x, vars, false)
	case map[string]any:
		o := Obj{}
		for k, v := range x {
			o[k] = substituteValue(v, vars)
		}
		return o
	case []any:
		a := []any{}
		for _, v := range x {
			a = append(a, substituteValue(v, vars))
		}
		return a
	default:
		return v
	}
}
func ruleHeaders(r KazumiRule) map[string]string {
	h := map[string]string{}
	if r.Referer != "" {
		h["Referer"] = r.Referer
	}
	if r.UserAgent != "" {
		h["User-Agent"] = r.UserAgent
	}
	return h
}
func requestRule(ctx context.Context, n Fetcher, r KazumiRule, req RuleRequest, vars map[string]string) ([]byte, error) {
	u, e := url.Parse(substitute(req.URL, vars, true))
	if e != nil {
		return nil, problem("来源请求地址无效。", "rule")
	}
	q := u.Query()
	for k, v := range req.Query {
		q.Set(k, str(substituteValue(v, vars)))
	}
	u.RawQuery = q.Encode()
	h := ruleHeaders(r)
	for k, v := range req.Headers {
		h[k] = substitute(v, vars, false)
	}
	method := req.Method
	if method == "" {
		method = "GET"
	}
	var body []byte
	if method == "POST" {
		switch req.BodyType {
		case "json":
			body, e = json.Marshal(substituteValue(req.Body, vars))
			h["Content-Type"] = "application/json"
		case "form":
			form := url.Values{}
			if o, ok := substituteValue(req.Body, vars).(map[string]any); ok {
				for k, v := range o {
					form.Set(k, str(v))
				}
			}
			body = []byte(form.Encode())
			h["Content-Type"] = "application/x-www-form-urlencoded"
		}
	}
	if e != nil {
		return nil, e
	}
	b, _, e := fetchRequest(ctx, n, RequestSpec{URL: u.String(), Method: method, Headers: h, Body: body, Limit: 4000000})
	return b, e
}
func jsonValues(v any, p string) []any {
	if p == "" {
		return []any{v}
	}
	x, e := jp.ParseString(p)
	if e != nil {
		return nil
	}
	return x.Get(v)
}
func jsonString(v any, p string) string {
	values := jsonValues(v, p)
	if len(values) == 0 {
		return ""
	}
	return str(values[0])
}
func parseRuleJSON(b []byte) (any, error) {
	var v any
	if e := json.Unmarshal(b, &v); e != nil {
		return nil, problem("来源未返回有效 JSON。", "bad_json", 502)
	}
	return v, nil
}
func ruleURL(base, ref string) (string, error) {
	b, e := url.Parse(base)
	if e != nil {
		return "", e
	}
	r, e := url.Parse(strings.TrimSpace(ref))
	if e != nil {
		return "", e
	}
	return publicURL(b.ResolveReference(r).String())
}
func searchRule(ctx context.Context, n Fetcher, p Provider, q string) ([]Film, error) {
	r := *p.Rule
	out := []Film{}
	seen := map[string]bool{}
	add := func(title, ref string) {
		title = text(title, 200)
		if title == "" || ref == "" || len(ref) > 4096 || seen[ref] {
			return
		}
		if r.ChapterMode != "api" {
			u, e := ruleURL(r.BaseURL, ref)
			if e != nil {
				return
			}
			ref = u
		}
		seen[ref] = true
		out = append(out, Film{ID: sourceFilmID("rule", p.ID, ref), Title: title, Kind: "动漫", Source: p.Name, ProviderID: p.ID, ProviderKind: "kazumi"})
	}
	if r.SearchMode == "api" {
		b, e := requestRule(ctx, n, r, r.SearchAPI.Request, map[string]string{"keyword": q})
		if e != nil {
			return nil, e
		}
		data, e := parseRuleJSON(b)
		if e != nil {
			return nil, e
		}
		for i, row := range jsonValues(data, r.SearchAPI.ListPath) {
			if i >= 60 {
				break
			}
			add(jsonString(row, r.SearchAPI.NamePath), jsonString(row, r.SearchAPI.SourcePath))
		}
		return out, nil
	}
	link := substitute(r.SearchURL, map[string]string{"keyword": q}, true)
	req := RequestSpec{URL: link, Method: "GET", Headers: ruleHeaders(r), Limit: 4000000}
	if r.UsePost {
		u, _ := url.Parse(link)
		req.Method = "POST"
		req.Body = []byte(u.RawQuery)
		u.RawQuery = ""
		req.URL = u.String()
		req.Headers["Content-Type"] = "application/x-www-form-urlencoded"
	}
	b, _, e := fetchRequest(ctx, n, req)
	if e != nil {
		return nil, e
	}
	doc, e := htmlquery.Parse(strings.NewReader(string(b)))
	if e != nil {
		return nil, e
	}
	rows, e := htmlquery.QueryAll(doc, relativeXPath(r.SearchList))
	if e != nil {
		return nil, problem("搜索规则执行失败。", "rule")
	}
	for i, row := range rows {
		if i >= 60 {
			break
		}
		name, _ := htmlquery.Query(row, relativeXPath(r.SearchName))
		link, _ := htmlquery.Query(row, relativeXPath(r.SearchResult))
		if name != nil && link != nil {
			add(htmlquery.InnerText(name), htmlquery.SelectAttr(link, "href"))
		}
	}
	return out, nil
}
func detailRule(ctx context.Context, n Fetcher, p Provider, ref string) (Film, error) {
	r := *p.Rule
	f := Film{Kind: "动漫", Source: p.Name, ProviderID: p.ID, ProviderKind: "kazumi"}
	if r.ChapterMode == "api" {
		b, e := requestRule(ctx, n, r, r.ChapterAPI.Request, map[string]string{"source": ref})
		if e != nil {
			return f, e
		}
		data, e := parseRuleJSON(b)
		if e != nil {
			return f, e
		}
		c := r.ChapterAPI
		if c.Format == "delimited" {
			f.Groups = groupsFromVOD(jsonString(data, c.PlayURLPath), jsonString(data, c.PlayFromPath))
			return f, nil
		}
		for gi, road := range jsonValues(data, c.RoadsPath) {
			if gi >= 40 {
				break
			}
			name := text(jsonString(road, c.RoadNamePath), 80)
			if name == "" || c.RoadNamePath == "" {
				name = p.Name
			}
			g := Group{Key: fmt.Sprintf("g%d", gi), Name: name, Episodes: []Episode{}}
			for ei, ep := range jsonValues(road, c.EpisodesPath) {
				if ei >= 3000 {
					break
				}
				label := text(jsonString(ep, c.EpisodeNamePath), 120)
				if label == "" {
					label = fmt.Sprintf("第%d集", ei+1)
				}
				link := jsonString(ep, c.EpisodeURLPath)
				if c.EpisodePage.URL != "" {
					vars := map[string]string{"source": ref, "episodeUrl": link, "episodeName": label, "roadName": name}
					link = substitute(c.EpisodePage.URL, vars, true)
					u, e := url.Parse(link)
					if e != nil {
						continue
					}
					query := u.Query()
					for k, v := range c.EpisodePage.Query {
						query.Set(k, str(substituteValue(v, vars)))
					}
					u.RawQuery = query.Encode()
					link = u.String()
				}
				clean, e := ruleURL(r.BaseURL, link)
				if e == nil {
					g.Episodes = append(g.Episodes, Episode{Key: fmt.Sprintf("e%d", ei), Name: label, URL: clean, Ordinal: episodeNumber(label)})
				}
			}
			if len(g.Episodes) > 0 {
				f.Groups = append(f.Groups, g)
			}
		}
		return f, nil
	}
	link, e := ruleURL(r.BaseURL, ref)
	if e != nil {
		return f, e
	}
	base, _ := url.Parse(r.BaseURL)
	detail, _ := url.Parse(link)
	if base.Hostname() != detail.Hostname() {
		return f, problem("详情页不属于所选来源。", "unsafe_url")
	}
	b, _, e := fetchRequest(ctx, n, RequestSpec{URL: link, Method: "GET", Headers: ruleHeaders(r), Limit: 4000000})
	if e != nil {
		return f, e
	}
	doc, e := htmlquery.Parse(strings.NewReader(string(b)))
	if e != nil {
		return f, e
	}
	if title, _ := htmlquery.Query(doc, "//meta[@property='og:title']"); title != nil {
		f.Title = normalizeTitle(htmlquery.SelectAttr(title, "content"))
	}
	if f.Title == "" {
		if h, _ := htmlquery.Query(doc, "//h1"); h != nil {
			f.Title = normalizeTitle(htmlquery.InnerText(h))
		}
	}
	roads, e := htmlquery.QueryAll(doc, relativeXPath(r.ChapterRoads))
	if e != nil {
		return f, problem("选集规则执行失败。", "rule")
	}
	for gi, road := range roads {
		if gi >= 40 {
			break
		}
		nodes, e := htmlquery.QueryAll(road, relativeXPath(r.ChapterResult))
		if e != nil {
			return f, problem("选集规则执行失败。", "rule")
		}
		g := Group{Key: fmt.Sprintf("g%d", gi), Name: fmt.Sprintf("%s · 线路%d", p.Name, gi+1), Episodes: []Episode{}}
		seen := map[string]bool{}
		for ei, node := range nodes {
			if ei >= 3000 {
				break
			}
			href := htmlquery.SelectAttr(node, "href")
			if href == "" {
				continue
			}
			u, e := ruleURL(link, href)
			if e != nil || seen[u] {
				continue
			}
			seen[u] = true
			label := text(htmlquery.InnerText(node), 120)
			if label == "" {
				label = fmt.Sprintf("第%d集", ei+1)
			}
			g.Episodes = append(g.Episodes, Episode{Key: fmt.Sprintf("e%d", ei), Name: label, URL: u, Ordinal: episodeNumber(label)})
		}
		if len(g.Episodes) > 0 {
			f.Groups = append(f.Groups, g)
		}
	}
	return f, nil
}
