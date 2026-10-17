// SPDX-License-Identifier: GPL-3.0-or-later
package engine

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

var gitSHA = re(`^[a-f0-9]{40}$`)

func (m *SourceManager) repositoryHead(ctx context.Context, repo string) (string, error) {
	if !boolSetting(m.store.Settings(), "network_consent", false) {
		return "", problem("联网已关闭。", "consent_required", 403)
	}
	d, e := m.net.JSON(ctx, "https://api.github.com/repos/"+repo+"/commits/main", Obj{})
	if e != nil {
		return "", e
	}
	sha := sget(d, "sha")
	if !gitSHA.MatchString(sha) {
		return "", problem("上游版本信息无效。", "update_schema")
	}
	return sha, nil
}
func (m *SourceManager) repositoryFile(ctx context.Context, repo, sha, file string, limit int) ([]byte, error) {
	if !boolSetting(m.store.Settings(), "network_consent", false) {
		return nil, problem("联网已关闭。", "consent_required", 403)
	}
	if !gitSHA.MatchString(sha) || strings.Contains(file, "..") || strings.ContainsAny(file, "\\?#") {
		return nil, problem("上游文件路径无效。", "update_schema")
	}
	b, _, e := m.net.Fetch(ctx, "https://raw.githubusercontent.com/"+repo+"/"+sha+"/"+file, limit)
	return b, e
}
func (m *SourceManager) update(ctx context.Context) (Obj, error) {
	if !boolSetting(m.store.Settings(), "network_consent", false) {
		return nil, problem("请先允许联网。", "consent_required", 403)
	}
	if !m.updateMu.TryLock() {
		return nil, problem("正在更新来源，请稍候。", "busy", 429)
	}
	defer m.updateMu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 85*time.Second)
	defer cancel()
	next := m.snapshot()
	if next.Upstreams == nil {
		next.Upstreams = map[string]UpstreamState{}
	}
	next.LastCheckedAt = now()
	changes := []string{}
	changed := false
	for _, entry := range []struct{ key, repo string }{{"88lin", "88lin/video_vip"}, {"kazumi_rules", "Predidit/KazumiRules"}, {"kazumi", "Predidit/Kazumi"}} {
		state := next.Upstreams[entry.key]
		state.Repository = entry.repo
		state.CheckedAt = now()
		state.Error = ""
		sha, e := m.repositoryHead(ctx, entry.repo)
		if e != nil {
			state.Status = "检查失败"
			state.Error = e.Error()
			next.Upstreams[entry.key] = state
			continue
		}
		if sha == state.Commit {
			state.Status = "已是最新"
			if state.NeedsAppUpdate {
				state.Status = "需要升级程序"
			}
			next.Upstreams[entry.key] = state
			continue
		}
		var version string
		var applied bool
		switch entry.key {
		case "88lin":
			var c CatalogConfig
			var ps []ParserConfig
			var script []byte
			script, e = m.repositoryFile(ctx, entry.repo, sha, "video_vip.user.js", 3000000)
			if e == nil {
				c, ps, version, e = parse88lin(script, next.Catalog, next.Parsers)
			}
			if e == nil {
				before, _ := json.Marshal(Obj{"catalog": next.Catalog, "parsers": next.Parsers})
				after, _ := json.Marshal(Obj{"catalog": c, "parsers": ps})
				applied = sha256.Sum256(before) != sha256.Sum256(after)
				next.Catalog = c
				next.Parsers = ps
				if applied {
					changes = append(changes, fmt.Sprintf("88lin：同步 %d 条解析线路", len(ps)))
				}
			}
		case "kazumi_rules":
			var providers []Provider
			providers, version, e = m.downloadRules(ctx, entry.repo, sha)
			if e == nil {
				old := []Provider{}
				keep := []Provider{}
				for _, p := range next.Providers {
					if p.Origin == "kazumi" {
						old = append(old, p)
					} else {
						keep = append(keep, p)
					}
				}
				before, _ := json.Marshal(old)
				after, _ := json.Marshal(providers)
				applied = sha256.Sum256(before) != sha256.Sum256(after)
				next.Providers = append(keep, providers...)
				if applied {
					changes = append(changes, fmt.Sprintf("Kazumi：同步 %d 个动漫来源", len(providers)))
				}
			}
		case "kazumi":
			var pub, api []byte
			pub, e = m.repositoryFile(ctx, entry.repo, sha, "pubspec.yaml", 100000)
			if e == nil {
				match := re(`(?m)^version:\s*([^\s]+)`).FindSubmatch(pub)
				if len(match) != 2 {
					e = problem("Kazumi 版本格式变化，需要适配。", "update_schema")
				} else {
					version = string(match[1])
				}
			}
			if e == nil {
				api, e = m.repositoryFile(ctx, entry.repo, sha, "lib/request/config/api_endpoints.dart", 100000)
				if e == nil {
					match := re(`apiLevel\s*=\s*(\d+)`).FindSubmatch(api)
					if len(match) != 2 {
						e = problem("Kazumi 规则接口格式变化，需要适配。", "update_schema")
					} else {
						level, _ := strconv.Atoi(string(match[1]))
						state.NeedsAppUpdate = level > supportedRuleAPI
					}
				}
			}
			if e == nil {
				if state.NeedsAppUpdate {
					changes = append(changes, "Kazumi：规则接口已变化，需要升级程序")
				} else {
					changes = append(changes, "Kazumi：检测到新提交，规则接口兼容（"+version+"）")
				}
			}
		}
		if e != nil {
			state.Status = "更新失败，保留原配置"
			state.Error = e.Error()
			if errCodeValue(e) == "rule_api" || errCodeValue(e) == "update_schema" {
				state.NeedsAppUpdate = true
				state.Status = "需要升级程序"
			}
			next.Upstreams[entry.key] = state
			continue
		}
		state.Commit = sha
		if entry.key != "kazumi" {
			state.NeedsAppUpdate = false
		}
		state.Version = version
		state.Status = "已是最新"
		if entry.key == "kazumi" {
			state.Status = "上游有更新，接口兼容"
		}
		if applied {
			state.Status = "已更新"
			state.AppliedAt = now()
			changed = true
		}
		if state.NeedsAppUpdate {
			state.Status = "需要升级程序"
		}
		next.Upstreams[entry.key] = state
	}
	if changed {
		next.LastUpdatedAt = now()
	}
	if len(changes) > 0 {
		next.Changes = changes
	}
	if e := m.save(next); e != nil {
		return nil, e
	}
	return m.publicState(), nil
}
func errCodeValue(e error) string {
	if x, ok := e.(*AppError); ok {
		return x.Code
	}
	return ""
}
func (m *SourceManager) downloadRules(ctx context.Context, repo, sha string) ([]Provider, string, error) {
	b, e := m.repositoryFile(ctx, repo, sha, "index.json", 500000)
	if e != nil {
		return nil, "", e
	}
	var index []struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if json.Unmarshal(b, &index) != nil || len(index) == 0 || len(index) > 80 {
		return nil, "", problem("规则索引格式变化，需要适配。", "update_schema")
	}
	out := make([]Provider, len(index))
	errs := make([]error, len(index))
	seen := map[string]bool{}
	for _, x := range index {
		if !re(`^[A-Za-z0-9_-]{1,60}$`).MatchString(x.Name) || seen[strings.ToLower(x.Name)] {
			return nil, "", problem("上游规则名称无效或重复。", "update_schema")
		}
		seen[strings.ToLower(x.Name)] = true
	}
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for i, x := range index {
		wg.Add(1)
		go func(i int, name, version string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				errs[i] = ctx.Err()
				return
			}
			defer func() { <-sem }()
			raw, e := m.repositoryFile(ctx, repo, sha, name+".json", 250000)
			if e != nil {
				errs[i] = e
				return
			}
			var rule KazumiRule
			if e = json.Unmarshal(raw, &rule); e != nil {
				errs[i] = problem("规则 JSON 格式不支持。", "update_schema")
				return
			}
			if rule.Name != name || rule.Version != version {
				errs[i] = problem("规则索引与内容不一致。", "update_schema")
				return
			}
			if e = validateRule(rule); e != nil {
				errs[i] = e
				return
			}
			out[i] = Provider{ID: "kazumi_" + strings.ToLower(name), Name: name, Kind: "kazumi", Origin: "kazumi", Enabled: true, Rule: &rule}
		}(i, x.Name, x.Version)
	}
	wg.Wait()
	for _, e := range errs {
		if e != nil {
			return nil, "", e
		}
	}
	return out, sha[:8], nil
}
func parse88lin(raw []byte, previous CatalogConfig, old []ParserConfig) (CatalogConfig, []ParserConfig, string, error) {
	versionMatch := re(`(?m)@version\s+([^\s]+)`).FindSubmatch(raw)
	if len(versionMatch) != 2 {
		return previous, nil, "", problem("88lin 版本格式变化，需要适配。", "update_schema")
	}
	listMatch := re(`(?s)videoParseList\s*:\s*\[(.*?)\]`).FindSubmatch(raw)
	if len(listMatch) != 2 {
		return previous, nil, "", problem("88lin 线路格式变化，需要适配。", "update_schema")
	}
	ps := []ParserConfig{}
	for _, object := range re(`\{[^{}]*\}`).FindAll(listMatch[1], -1) {
		var row Obj
		if json.Unmarshal(object, &row) != nil {
			return previous, nil, "", problem("88lin 线路格式不支持。", "update_schema")
		}
		link := sget(row, "url")
		if link == "" {
			continue
		}
		name := text(row["name"], 80)
		if name == "" {
			return previous, nil, "", problem("88lin 线路缺少名称。", "update_schema")
		}
		if !strings.Contains(link, "{url}") {
			link += "{url}"
		}
		if e := validateTemplate(link); e != nil {
			return previous, nil, "", e
		}
		id := ""
		enabled := true
		status := "未实播验证"
		for _, p := range old {
			if p.Name == name || p.Template == link {
				id = p.ID
				enabled = p.Enabled
				status = p.Status
				break
			}
		}
		if id == "" {
			sum := sha256.Sum256([]byte(name))
			id = fmt.Sprintf("parser_%x", sum[:6])
			if strings.Contains(strings.ToUpper(name), "HLS") {
				enabled = false
			}
		}
		ps = append(ps, ParserConfig{ID: id, Name: name, Template: link, Enabled: enabled, Status: status})
	}
	if len(ps) == 0 || len(ps) > 40 {
		return previous, nil, "", problem("88lin 线路数量异常。", "update_schema")
	}
	values := map[string]string{}
	for _, key := range []string{"SITE", "API", "PLAYER"} {
		match := re(`const\s+` + key + `\s*=\s*['"]([^'"]+)['"]`).FindSubmatch(raw)
		if len(match) != 2 {
			return previous, nil, "", problem("88lin 目录格式变化，需要适配。", "update_schema")
		}
		values[key] = string(match[1])
	}
	c := previous
	c.SuggestURL = strings.TrimRight(values["SITE"], "/") + "/index.php/ajax/suggest"
	c.APIURL = values["API"]
	c.PlayerURL = values["PLAYER"]
	if !strings.Contains(c.PlayerURL, "{url}") {
		c.PlayerURL += "{url}"
	}
	test := SourceRegistry{Schema: 1, Providers: []Provider{{ID: "wsyzy", Name: "无损云", Kind: "legacy"}}, Catalog: c, Parsers: ps}
	if e := validateRegistry(test); e != nil {
		return previous, nil, "", e
	}
	return c, ps, string(versionMatch[1]), nil
}
func (s *Service) startSourceUpdates(ctx context.Context) {
	check := func() {
		if s.requireNetwork() != nil || !boolSetting(s.store.Settings(), "auto_update_sources", true) {
			return
		}
		r := s.manager.snapshot()
		last, e := time.Parse(time.RFC3339, r.LastCheckedAt)
		if e == nil && time.Since(last) < 24*time.Hour {
			return
		}
		_, _ = s.manager.update(ctx)
	}
	go func() {
		timer := time.NewTimer(3 * time.Second)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			check()
		}
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				check()
			case <-s.updateTrigger:
				check()
			}
		}
	}()
}
