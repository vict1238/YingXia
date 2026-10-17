// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"
)

type resolveBarrierNetwork struct {
	*fakeNetwork
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (n *resolveBarrierNetwork) JSON(ctx context.Context, base string, params Obj) (Obj, error) {
	n.once.Do(func() {
		close(n.entered)
		select {
		case <-n.release:
		case <-ctx.Done():
		}
	})
	return n.fakeNetwork.JSON(ctx, base, params)
}

// Block the detail request, before source selection or media resolution. This
// proves the clear generation must be captured at Resolve entry.
func pendingResolve(t *testing.T, s *Service, n *resolveBarrierNetwork) <-chan Obj {
	t.Helper()
	result := make(chan Obj, 1)
	go func() {
		playback, err := s.Resolve(context.Background(), Obj{"film_id": "101", "group_key": "g0", "episode_key": "e0"})
		result <- Obj{"playback": playback, "error": err}
	}()
	select {
	case <-n.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("detail request did not wait")
	}
	return result
}

func resolvedResult(t *testing.T, result <-chan Obj) Obj {
	t.Helper()
	select {
	case value := <-result:
		if value["error"] != nil {
			t.Fatal(value["error"])
		}
		return value["playback"].(Obj)
	case <-time.After(2 * time.Second):
		t.Fatal("pending resolve did not finish")
		return nil
	}
}

func TestClearHistoryDuringPendingResolve(t *testing.T) {
	n := &resolveBarrierNetwork{fakeNetwork: &fakeNetwork{}, entered: make(chan struct{}), release: make(chan struct{})}
	s, err := newService(t.TempDir(), n)
	mustNoErr(t, err)
	mustNoErr(t, s.store.SetMany(Obj{"network_consent": true}))
	_, err = s.store.Opened("prior", "prior", "1", "fixture", "media")
	mustNoErr(t, err)
	srv, err := newServer(s, 0)
	mustNoErr(t, err)
	defer srv.listener.Close()
	result := pendingResolve(t, s, n)
	response := perform(srv, "/api/clear-history", "POST", Obj{}, true, nil)
	require(t, response.Code == 200, response.Body.String())
	close(n.release)
	playback := resolvedResult(t, result)
	require(t, sget(playback, "history_id") == "", "pre-clear resolve received a new history ID")
	require(t, len(s.store.History()) == 0, "pre-clear resolve recreated history")
	reopened, err := newStore(s.dir)
	mustNoErr(t, err)
	require(t, len(reopened.History()) == 0, "delayed resolve changed cleared disk history")
	// A new intentional playback after the clear still records normally.
	playback, err = s.Resolve(context.Background(), Obj{"film_id": "101", "group_key": "g0", "episode_key": "e0"})
	mustNoErr(t, err)
	require(t, sget(playback, "history_id") != "" && len(s.store.History()) == 1, "new playback after clear was suppressed")
}

func TestFailedClearDoesNotInvalidatePendingResolve(t *testing.T) {
	n := &resolveBarrierNetwork{fakeNetwork: &fakeNetwork{}, entered: make(chan struct{}), release: make(chan struct{})}
	s, err := newService(t.TempDir(), n)
	mustNoErr(t, err)
	mustNoErr(t, s.store.SetMany(Obj{"network_consent": true}))
	_, err = s.store.Opened("prior", "prior", "1", "fixture", "media")
	mustNoErr(t, err)
	original, err := os.ReadFile(s.store.path)
	mustNoErr(t, err)
	generation := s.store.HistoryGeneration()
	result := pendingResolve(t, s, n)
	// Replacing the target with a directory makes the actual atomic rename fail
	// on all platforms, independently of running the tests with elevated rights.
	mustNoErr(t, os.Remove(s.store.path))
	mustNoErr(t, os.Mkdir(s.store.path, 0700))
	require(t, s.store.ClearHistory() != nil, "expected a real storage error")
	require(t, s.store.HistoryGeneration() == generation, "failed clear advanced generation")
	require(t, len(s.store.History()) == 1, "failed clear lost existing rows")
	mustNoErr(t, os.Remove(s.store.path))
	mustNoErr(t, os.WriteFile(s.store.path, original, 0600))
	close(n.release)
	playback := resolvedResult(t, result)
	require(t, sget(playback, "history_id") != "" && len(s.store.History()) == 2, "failed clear suppressed pending playback")
	bytes, err := os.ReadFile(s.store.path)
	mustNoErr(t, err)
	var saved StoreData
	mustNoErr(t, json.Unmarshal(bytes, &saved))
	require(t, len(saved.History) == 2, "failed-clear retry did not persist both rows")
}
