// SPDX-License-Identifier: GPL-3.0-or-later
package engine

import (
	"context"
	"net/http"
	"net/url"
	"sort"
	"sync"
	"time"
)

const publicSuggestHost = "suggest.video.iqiyi.com"

type suggestionCache struct {
	items   []Obj
	expires time.Time
}
type PublicTitleSuggestions struct {
	mu    sync.Mutex
	cache map[string]suggestionCache
	slots chan struct{}
}

func newPublicTitleSuggestions() *PublicTitleSuggestions {
	return &PublicTitleSuggestions{cache: map[string]suggestionCache{}, slots: make(chan struct{}, 1)}
}
func romanQuery(q string) bool {
	if len(q) < 2 || len(q) > 60 {
		return false
	}
	for _, r := range q {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}
func (s *Service) publicSuggestions(parent context.Context, q string) []Obj {
	if !romanQuery(q) || s.requireNetwork() != nil {
		return nil
	}
	p := s.publicTitles
	p.mu.Lock()
	cached, ok := p.cache[q]
	p.mu.Unlock()
	if ok && time.Now().Before(cached.expires) {
		return cloneList(cached.items)
	}
	select {
	case p.slots <- struct{}{}:
	default:
		return nil
	}
	defer func() { <-p.slots }()
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
	var fetch Fetcher = s.network
	if n, ok := s.network.(*Network); ok {
		client := *n.client
		previous := client.CheckRedirect
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			if req.URL.Scheme != "https" || req.URL.Host != publicSuggestHost {
				return problem("联想服务重定向已拒绝。", "suggest_redirect")
			}
			if previous != nil {
				return previous(req, via)
			}
			return nil
		}
		fetch = &Network{client: &client}
	}
	body, meta, e := fetch.Fetch(ctx, "https://"+publicSuggestHost+"/?key="+url.QueryEscape(q), 128<<10)
	if e != nil || ctx.Err() != nil || s.requireNetwork() != nil {
		return nil
	}
	if endpoint := sget(meta, "endpoint"); endpoint != "" {
		u, e := url.Parse(endpoint)
		if e != nil || u.Scheme != "https" || u.Host != publicSuggestHost {
			return nil
		}
	}
	o, e := decodeObject(body)
	if e != nil || sget(o, "code") != "A00000" {
		return nil
	}
	rows, _ := o["data"].([]any)
	items := []Obj{}
	seen := map[string]bool{}
	for i, raw := range rows {
		if i >= 20 {
			break
		}
		row, ok := raw.(map[string]any)
		if !ok || row["is_known_query"] != true {
			continue
		}
		title := text(row["name"], 120)
		if title == "" || hasControl(title) {
			continue
		}
		forms, initials := titleForms(title)
		r := titleRecord{Title: title, forms: forms, initials: initials}
		score, kind := titleScore(q, r)
		if score < 640 || seen[title] {
			continue
		}
		seen[title] = true
		items = append(items, Obj{"title": title, "query": title, "match": kind, "score": score, "origin": "public_suggestion"})
	}
	sort.Slice(items, func(i, j int) bool {
		a, _ := numValue(items[i]["score"])
		b, _ := numValue(items[j]["score"])
		if a != b {
			return a > b
		}
		return sget(items[i], "title") < sget(items[j], "title")
	})
	if len(items) > 8 {
		items = items[:8]
	}
	p.mu.Lock()
	for key, entry := range p.cache {
		if time.Now().After(entry.expires) {
			delete(p.cache, key)
		}
	}
	if len(p.cache) >= 64 {
		p.cache = map[string]suggestionCache{}
	}
	p.cache[q] = suggestionCache{cloneList(items), time.Now().Add(10 * time.Minute)}
	p.mu.Unlock()
	return items
}

func (s *Service) suggestQuery(ctx context.Context, query string, online bool) Obj {
	info := s.titles.suggest(query)
	if !online || info["resolved"] == true {
		return info
	}
	remote := s.publicSuggestions(ctx, searchNorm(query))
	if len(remote) == 0 {
		return info
	}
	local, _ := info["items"].([]Obj)
	all := append(local, remote...)
	sort.SliceStable(all, func(i, j int) bool {
		a, _ := numValue(all[i]["score"])
		b, _ := numValue(all[j]["score"])
		if a != b {
			return a > b
		}
		return sget(all[i], "title") < sget(all[j], "title")
	})
	seen := map[string]bool{}
	items := []Obj{}
	for _, item := range all {
		title := sget(item, "title")
		if seen[title] {
			continue
		}
		seen[title] = true
		items = append(items, item)
		if len(items) >= 8 {
			break
		}
	}
	info["items"], info["scope"], info["hint"] = items, "local_and_public_suggestions", "拼音联想结合本机片名与爱奇艺公开联想；选择片名后检索已启用来源"
	if len(items) > 0 {
		score, _ := numValue(items[0]["score"])
		second := float64(0)
		if len(items) > 1 {
			second, _ = numValue(items[1]["score"])
		}
		kind := sget(items[0], "match")
		if (kind == "pinyin" || kind == "initials") && score > second {
			info["resolved"], info["resolved_query"] = true, sget(items[0], "title")
		}
	}
	return info
}
