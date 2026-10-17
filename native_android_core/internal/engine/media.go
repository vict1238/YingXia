// SPDX-License-Identifier: GPL-3.0-or-later
package engine

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"html"
	"net/url"
	"path"
	"strings"

	"github.com/antchfx/htmlquery"
)

func isMediaURL(link string) bool {
	u, e := url.Parse(link)
	if e != nil {
		return false
	}
	switch strings.ToLower(path.Ext(u.Path)) {
	case ".m3u8", ".mp4", ".webm", ".m4v", ".mov":
		return true
	}
	return false
}

var playerObject = re(`(?s)(?:var\s+|let\s+|const\s+)?player_[a-zA-Z0-9_]+\s*=\s*(\{[^\n]*?\})\s*;`)
var rawMediaURL = re(`https?:[^\s<>"']+?\.(?:m3u8|mp4|webm)(?:\?[^\s<>"']*)?`)

func extractPageMedia(raw []byte, base string) (string, []string) {
	for _, match := range playerObject.FindAllSubmatch(raw, 8) {
		var o Obj
		if json.Unmarshal(match[1], &o) == nil {
			link := sget(o, "url")
			switch sget(o, "encrypt") {
			case "1":
				if decoded, e := url.QueryUnescape(link); e == nil {
					link = decoded
				}
			case "2":
				if b, e := base64.StdEncoding.DecodeString(link); e == nil {
					link = string(b)
					if decoded, e := url.QueryUnescape(link); e == nil {
						link = decoded
					}
				}
			}
			if isMediaURL(link) {
				if u, e := ruleURL(base, link); e == nil {
					return u, nil
				}
			}
		}
	}
	doc, e := htmlquery.Parse(strings.NewReader(string(raw)))
	if e == nil {
		nodes, _ := htmlquery.QueryAll(doc, "//video[@src] | //video/source[@src] | //source[@src]")
		for _, node := range nodes {
			if u, e := ruleURL(base, htmlquery.SelectAttr(node, "src")); e == nil && isMediaURL(u) {
				return u, nil
			}
		}
	}
	text := strings.ReplaceAll(string(raw), `\/`, `/`)
	matches := rawMediaURL.FindAllString(text, 8)
	if len(matches) == 1 {
		if u, e := publicURL(html.UnescapeString(matches[0])); e == nil {
			return u, nil
		}
	}
	frames := []string{}
	if doc != nil {
		nodes, _ := htmlquery.QueryAll(doc, "//iframe[@src]")
		for i, node := range nodes {
			if i >= 3 {
				break
			}
			if u, e := ruleURL(base, htmlquery.SelectAttr(node, "src")); e == nil {
				frames = append(frames, u)
			}
		}
	}
	return "", frames
}
func resolveRuleMedia(ctx context.Context, n Fetcher, link string, rule KazumiRule) (string, error) {
	if isMediaURL(link) {
		return link, nil
	}
	headers := ruleHeaders(rule)
	b, _, e := fetchRequest(ctx, n, RequestSpec{URL: link, Method: "GET", Headers: headers, Limit: 4000000})
	if e != nil {
		return "", e
	}
	media, frames := extractPageMedia(b, link)
	if media != "" {
		return media, nil
	}
	for _, frame := range frames {
		b, _, e := fetchRequest(ctx, n, RequestSpec{URL: frame, Method: "GET", Headers: headers, Limit: 2000000})
		if e == nil {
			if media, _ := extractPageMedia(b, frame); media != "" {
				return media, nil
			}
		}
	}
	return "", problem("此来源需要网页播放器。", "page_player")
}
