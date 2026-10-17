// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type StoreData struct {
	Schema    int   `json:"schema"`
	Settings  Obj   `json:"settings"`
	Favorites []Obj `json:"favorites"`
	History   []Obj `json:"history"`
	Bindings  []Obj `json:"bindings"`
}
type Store struct {
	mu                sync.Mutex
	path              string
	data              StoreData
	historyGeneration uint64
}

func newStore(dir string) (*Store, error) {
	if e := os.MkdirAll(dir, 0700); e != nil {
		return nil, e
	}
	s := &Store{path: filepath.Join(dir, "library.json"), data: StoreData{1, Obj{}, []Obj{}, []Obj{}, []Obj{}}}
	b, e := os.ReadFile(s.path)
	if e == nil {
		if e = json.Unmarshal(b, &s.data); e != nil {
			return nil, problem("本机 library.json 损坏；请先备份后移走该文件，不会自动覆盖数据。", "storage", 500)
		}
	} else if !os.IsNotExist(e) {
		return nil, e
	}
	if s.data.Schema != 1 {
		return nil, problem("本机数据格式不支持。", "storage", 500)
	}
	if s.data.Settings == nil {
		s.data.Settings = Obj{}
	}
	return s, nil
}
func token() string {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
func now() string { return time.Now().UTC().Format(time.RFC3339) }
func (s *Store) saveLocked() error {
	b, e := json.MarshalIndent(s.data, "", "  ")
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(s.path), ".library-*.tmp")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	return replaceFile(name, s.path)
}
func (s *Store) Settings() Obj { s.mu.Lock(); defer s.mu.Unlock(); return cloneObj(s.data.Settings) }
func (s *Store) SetMany(values Obj) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old := cloneObj(s.data.Settings)
	for k, v := range values {
		s.data.Settings[k] = v
	}
	if e := s.saveLocked(); e != nil {
		s.data.Settings = old
		return e
	}
	return nil
}
func cloneObj(o Obj) Obj {
	b, _ := json.Marshal(o)
	var v Obj
	_ = json.Unmarshal(b, &v)
	if v == nil {
		return Obj{}
	}
	return v
}
func cloneList(a []Obj) []Obj {
	out := []Obj{}
	for _, o := range a {
		out = append(out, cloneObj(o))
	}
	return out
}
func (s *Store) Favorites() []Obj {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneList(s.data.Favorites)
}
func (s *Store) Toggle(f Obj) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	old := s.data.Favorites
	next := []Obj{}
	found := false
	for _, x := range old {
		if sget(x, "id") == sget(f, "id") {
			found = true
		} else {
			next = append(next, x)
		}
	}
	if !found {
		x := cloneObj(f)
		delete(x, "groups")
		next = append([]Obj{x}, next...)
	}
	s.data.Favorites = next
	if e := s.saveLocked(); e != nil {
		s.data.Favorites = old
		return false, e
	}
	return !found, nil
}
func (s *Store) Opened(id, title, ep, source, mode string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.openedLocked(id, title, ep, source, mode)
}

func (s *Store) HistoryGeneration() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.historyGeneration
}

// A successful clear invalidates requests that began before it. Check and
// commit under the same mutex so delayed resolution cannot recreate records.
func (s *Store) OpenedOnGeneration(generation uint64, id, title, ep, source, mode string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if generation != s.historyGeneration {
		return "", nil
	}
	return s.openedLocked(id, title, ep, source, mode)
}

func (s *Store) openedLocked(id, title, ep, source, mode string) (string, error) {
	if !boolSetting(s.data.Settings, "save_history", true) {
		return "", nil
	}
	key := token()[:32]
	t := now()
	o := Obj{"id": key, "film_id": id, "title": title, "episode": ep, "source": source, "mode": mode, "position": 0, "evidence": "opened", "created_at": t, "updated_at": t}
	old := s.data.History
	s.data.History = append([]Obj{o}, old...)
	if len(s.data.History) > 300 {
		s.data.History = s.data.History[:300]
	}
	if e := s.saveLocked(); e != nil {
		s.data.History = old
		return "", e
	}
	return key, nil
}
func (s *Store) Progress(id, evidence string, position any) error {
	if evidence != "browser_time_advanced" && evidence != "user_confirmed" {
		return problem("无效播放证据类型。", "evidence")
	}
	switch position.(type) {
	case float64, float32, int, int64, json.Number:
	default:
		return problem("无效播放进度。", "position")
	}
	p, ok := numValue(position)
	if !ok || math.IsNaN(p) || math.IsInf(p, 0) || p < 0 || p > 604800 {
		return problem("无效播放进度。", "position")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, x := range s.data.History {
		if sget(x, "id") == id {
			n := cloneObj(x)
			n["position"] = p
			n["evidence"] = evidence
			n["updated_at"] = now()
			s.data.History[i] = n
			if e := s.saveLocked(); e != nil {
				s.data.History[i] = x
				return e
			}
			break
		}
	}
	return nil
}
func (s *Store) History() []Obj {
	s.mu.Lock()
	defer s.mu.Unlock()
	a := s.data.History
	if len(a) > 100 {
		a = a[:100]
	}
	return cloneList(a)
}
func (s *Store) ClearHistory() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old := s.data.History
	s.data.History = []Obj{}
	if e := s.saveLocked(); e != nil {
		s.data.History = old
		return e
	}
	s.historyGeneration++
	return nil
}
func (s *Store) Bind(id, ep, platform, link string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old := s.data.Bindings
	out := []Obj{}
	for _, x := range old {
		if sget(x, "film_id") != id || sget(x, "episode") != ep || sget(x, "platform") != platform {
			out = append(out, x)
		}
	}
	out = append(out, Obj{"film_id": id, "episode": ep, "platform": platform, "url": link})
	s.data.Bindings = out
	if e := s.saveLocked(); e != nil {
		s.data.Bindings = old
		return e
	}
	return nil
}
func (s *Store) Bindings(id, ep string) []Obj {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []Obj{}
	for _, x := range s.data.Bindings {
		if sget(x, "film_id") == id && sget(x, "episode") == ep {
			out = append(out, cloneObj(x))
		}
	}
	return out
}
