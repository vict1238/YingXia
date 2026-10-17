// SPDX-License-Identifier: GPL-3.0-or-later
package engine

import (
	"context"
	"strings"
)

func (s *Service) Resume(ctx context.Context, filmID string) (Obj, error) {
	if filmID == "" || len(filmID) > 6100 {
		return nil, problem("影片编号无效。", "bad_id")
	}
	r := s.store.Resume(filmID)
	if r["found"] != true {
		return r, nil
	}
	f, e := s.Detail(ctx, filmID)
	if e != nil {
		return nil, e
	}
	if generation, ok := numValue(r["history_generation"]); !ok || uint64(generation) != s.store.HistoryGeneration() {
		return s.store.Resume(filmID), nil
	}
	// Prefer the saved group; names provide a safe migration from older records
	// which did not contain stable group/episode keys.
	groups := append([]Group{}, f.Groups...)
	for i, g := range groups {
		if g.Key == sget(r, "group_key") || sget(r, "group_key") == "" && (sget(r, "source") == g.Name || strings.HasSuffix(sget(r, "source"), " / "+g.Name)) {
			groups[0], groups[i] = groups[i], groups[0]
			break
		}
	}
	for _, g := range groups {
		ep, err := chooseEpisode(g, sget(r, "episode"))
		if err != nil {
			continue
		}
		r["group_key"], r["episode_key"], r["episode"] = g.Key, ep.Key, ep.Name
		return r, nil
	}
	r["found"], r["position"], r["policy"] = false, 0, "restart"
	r["selection_missing"] = true
	return r, nil
}
