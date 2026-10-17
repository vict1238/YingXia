// SPDX-License-Identifier: GPL-3.0-or-later
package engine

import (
	"context"
	"html"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

type CatalogConfig struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	SuggestURL string `json:"suggest_url"`
	APIURL     string `json:"api_url"`
	PlayerURL  string `json:"player_url"`
}
type ParserConfig struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Template string `json:"template"`
	Enabled  bool   `json:"enabled"`
	Status   string `json:"status"`
}
type Config struct {
	ConfigVersion int            `json:"config_version"`
	Provenance    string         `json:"provenance"`
	Catalog       CatalogConfig  `json:"catalog"`
	Parsers       []ParserConfig `json:"parsers"`
}
type filmCache struct {
	When time.Time
	Film Film
}
type searchCache struct {
	When  time.Time
	Films []Film
}
type Catalog struct {
	config   CatalogConfig
	net      Fetcher
	mu       sync.Mutex
	films    map[string]filmCache
	searches map[string]searchCache
}

func newCatalog(c CatalogConfig, n Fetcher) *Catalog {
	return &Catalog{config: c, net: n, films: map[string]filmCache{}, searches: map[string]searchCache{}}
}
func rows(d Obj) ([]Obj, error) {
	a, ok := d["list"].([]any)
	if !ok {
		return nil, problem("目录缺少 list 数组。", "bad_schema", 502)
	}
	out := []Obj{}
	for _, x := range a {
		if r, ok := x.(map[string]any); ok {
			out = append(out, r)
		}
	}
	return out, nil
}
func (c *Catalog) save(fs []Film) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.films) > 1000 {
		c.films = map[string]filmCache{}
	}
	for _, f := range fs {
		c.films[f.ID] = filmCache{time.Now(), f}
	}
}
func (c *Catalog) Search(ctx context.Context, query string) ([]Film, []string, error) {
	c.mu.Lock()
	cached, ok := c.searches[query]
	c.mu.Unlock()
	if ok && time.Since(cached.When) < 5*time.Minute {
		return cached.Films, []string{"来自 5 分钟本机会话缓存；不是实时播放检测。"}, nil
	}
	warnings := []string{}
	films := []Film{}
	d, e := c.net.JSON(ctx, c.config.SuggestURL, Obj{"mid": 1, "wd": query})
	var rr []Obj
	if e == nil {
		rr, e = rows(d)
	}
	if e == nil {
		for i, r := range rr {
			if i >= 40 {
				break
			}
			f, e := filmFromVOD(r)
			if e == nil {
				films = append(films, f)
			}
		}
		if len(films) > 0 {
			ids := []string{}
			for _, f := range films {
				ids = append(ids, f.ID)
			}
			detail, de := c.net.JSON(ctx, c.config.APIURL, Obj{"ac": "detail", "ids": strings.Join(ids, ",")})
			var dr []Obj
			if de == nil {
				dr, de = rows(detail)
			}
			if de == nil {
				byID := map[string]Film{}
				for _, r := range dr {
					f, fe := filmFromVOD(r)
					if fe == nil {
						byID[f.ID] = f
					}
				}
				missing := false
				for i, f := range films {
					if found, ok := byID[f.ID]; ok {
						films[i] = found
					} else {
						missing = true
					}
				}
				if missing {
					warnings = append(warnings, "部分详情尚未取得；选择作品时会再次读取详情。")
				}
			} else {
				warnings = append(warnings, "已取得片名，但详情接口未成功；点击作品可重试。")
			}
		}
	} else {
		d, e = c.net.JSON(ctx, c.config.APIURL, Obj{"ac": "detail", "wd": query, "pg": 1})
		if e == nil {
			rr, e = rows(d)
		}
		if e != nil {
			return nil, nil, e
		}
		for i, r := range rr {
			if i >= 60 {
				break
			}
			f, e := filmFromVOD(r)
			if e == nil {
				films = append(films, f)
			}
		}
		warnings = append(warnings, "联想搜索未成功，已改用同一目录的详情搜索；仅显示返回的第一页。")
	}
	sort.SliceStable(films, func(i, j int) bool {
		return ranking(query, films[i].Title, films[i].Year) > ranking(query, films[j].Title, films[j].Year)
	})
	c.save(films)
	c.mu.Lock()
	if len(c.searches) > 200 {
		c.searches = map[string]searchCache{}
	}
	c.searches[query] = searchCache{time.Now(), films}
	c.mu.Unlock()
	return films, warnings, nil
}
func (c *Catalog) Detail(ctx context.Context, id string) (Film, error) {
	c.mu.Lock()
	cached, ok := c.films[id]
	c.mu.Unlock()
	if ok && len(cached.Film.Groups) > 0 && time.Since(cached.When) < 5*time.Minute {
		return cached.Film, nil
	}
	if !idRE.MatchString(id) {
		return Film{}, problem("无效影片编号。", "bad_id")
	}
	d, e := c.net.JSON(ctx, c.config.APIURL, Obj{"ac": "detail", "ids": id})
	if e != nil {
		return Film{}, e
	}
	rr, e := rows(d)
	if e != nil {
		return Film{}, e
	}
	for _, r := range rr {
		f, e := filmFromVOD(r)
		if e == nil && f.ID == id {
			c.save([]Film{f})
			return f, nil
		}
	}
	return Film{}, problem("接口未返回所选影片；不会改播其他作品。", "not_found", 404)
}

var anchorRE = re(`(?is)<a\b([^>]*)>(.*?)</a\s*>`)
var attrRE = re(`(?is)([a-zA-Z_:][a-zA-Z0-9_:.-]*)\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s>]+))`)
var imageRE = re(`(?is)<img\b([^>]*)>`)

func attrs(s string) map[string]string {
	a := map[string]string{}
	for _, m := range attrRE.FindAllStringSubmatch(s, -1) {
		v := m[2]
		if v == "" {
			v = m[3]
		}
		if v == "" {
			v = m[4]
		}
		a[strings.ToLower(m[1])] = html.UnescapeString(v)
	}
	return a
}
func locatePlatform(ctx context.Context, n Fetcher, platform, query string) ([]Obj, string, error) {
	link, e := officialSearch(platform, query)
	if e != nil {
		return nil, "", e
	}
	b, _, e := n.Fetch(ctx, link, 2000000)
	if e != nil {
		return nil, link, e
	}
	base, _ := url.Parse(link)
	out := []Obj{}
	seen := map[string]bool{}
	for _, m := range anchorRE.FindAllStringSubmatch(string(b), -1) {
		a := attrs(m[1])
		u, e := url.Parse(a["href"])
		if e != nil {
			continue
		}
		_, s, e := platformURL(base.ResolveReference(u).String(), platform)
		if e != nil || seen[s] {
			continue
		}
		label := a["title"]
		if label == "" {
			content := m[2]
			for _, im := range imageRE.FindAllStringSubmatch(m[2], -1) {
				content += " " + attrs(im[1])["alt"]
			}
			label = text(content, 180)
		}
		if label == "" {
			continue
		}
		seen[s] = true
		if ranking(query, label, "") >= 30 {
			out = append(out, Obj{"title": label, "url": s, "platform": platform})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return ranking(query, sget(out[i], "title"), "") > ranking(query, sget(out[j], "title"), "")
	})
	if len(out) > 30 {
		out = out[:30]
	}
	return out, link, nil
}
