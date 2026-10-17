// SPDX-License-Identifier: GPL-3.0-or-later
package engine

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
)

// Runtime owns the private API. Credentials are returned directly to its native
// host and are never published by an unauthenticated HTTP bootstrap endpoint.
type Runtime struct {
	server   *Server
	cancel   context.CancelFunc
	done     chan struct{}
	once     sync.Once
	stopOnce sync.Once
	unlock   func()
	logFile  *os.File
	stopping atomic.Bool
}

func NewRuntime(dir string) (*Runtime, error) {
	return newRuntime(dir, nil)
}

func newRuntime(dir string, network Fetcher) (*Runtime, error) {
	if dir == "" {
		return nil, problem("未提供数据目录。", "storage", 500)
	}
	dir, e := filepath.Abs(dir)
	if e != nil {
		return nil, e
	}
	if e = os.MkdirAll(dir, 0700); e != nil {
		return nil, e
	}
	unlock, held, e := instanceLock(dir)
	if e != nil {
		return nil, e
	}
	if !held {
		return nil, problem("映匣已在运行，请先关闭另一个版本。", "already_running", 409)
	}
	s, e := newService(dir, network)
	if e != nil {
		unlock()
		return nil, e
	}
	server, e := newServer(s, 0)
	if e != nil {
		unlock()
		return nil, e
	}
	logPath := filepath.Join(dir, "native.log")
	if st, err := os.Stat(logPath); err == nil && st.Size() > 2000000 {
		_ = os.Remove(logPath + ".1")
		_ = os.Rename(logPath, logPath+".1")
	}
	lf, e := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		_ = server.listener.Close()
		unlock()
		return nil, e
	}
	log.SetOutput(lf)
	ctx, cancel := context.WithCancel(context.Background())
	r := &Runtime{server: server, cancel: cancel, done: make(chan struct{}), unlock: unlock, logFile: lf}
	server.onQuit = r.Stop
	s.startSourceUpdates(ctx)
	go func() {
		log.Print("native backend started ", version)
		err := server.HTTP.Serve(server.listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("native API stopped type=%T", err)
		}
		r.stopOnce.Do(func() {
			r.stopping.Store(true)
			r.server.service.store.Close()
			r.cancel()
			_ = server.HTTP.Close()
			_ = server.listener.Close()
			r.finish()
		})
	}()
	return r, nil
}

func (r *Runtime) Bootstrap() string {
	b, _ := json.Marshal(Obj{"origin": r.server.origin, "cookie": r.server.cookieName + "=" + r.server.session, "csrf": r.server.csrf, "version": version})
	return string(b)
}

func (r *Runtime) Running() bool {
	if r.stopping.Load() {
		return false
	}
	select {
	case <-r.done:
		return false
	default:
		return true
	}
}

func (r *Runtime) Wait() { <-r.done }

func (r *Runtime) Stop() {
	r.stopOnce.Do(func() {
		r.stopping.Store(true)
		r.server.service.store.Close()
		r.cancel()
		r.server.shutdown()
		_ = r.server.HTTP.Close()
		_ = r.server.listener.Close()
		r.finish()
	})
	<-r.done
}

func (r *Runtime) finish() {
	r.once.Do(func() {
		r.cancel()
		r.server.service.searches.cancelActive()
		r.server.service.sourceSpeeds.cancelActive()
		r.server.service.alternatives.cancelActive()
		r.server.service.titles.close()
		r.server.service.media.close()
		log.Print("native backend stopped")
		log.SetOutput(io.Discard)
		_ = r.logFile.Close()
		r.unlock()
		close(r.done)
	})
}
