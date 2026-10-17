// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"encoding/json"
	"github.com/antchfx/xpath"
	"testing"
)

func TestBundledRulesValidate(t *testing.T) {
	b, _ := assets.ReadFile("providers.json")
	var r SourceRegistry
	mustNoErr(t, json.Unmarshal(b, &r))
	for _, p := range r.Providers {
		t.Run(p.ID, func(t *testing.T) {
			if p.Rule != nil {
				for _, s := range []string{p.Rule.SearchList, p.Rule.SearchName, p.Rule.SearchResult, p.Rule.ChapterRoads, p.Rule.ChapterResult} {
					if s != "" {
						if _, e := xpath.Compile(relativeXPath(s)); e != nil {
							t.Fatalf("%s: %s: %v", p.Name, s, e)
						}
					}
				}
			}
			mustNoErr(t, validateProvider(p))
		})
	}
}
