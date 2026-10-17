//go:build !windows

// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

func proxyForRequest(r *http.Request) (*url.URL, error) { return http.ProxyFromEnvironment(r) }
func replaceFile(from, to string) error                 { return os.Rename(from, to) }
func hideProcess(c *exec.Cmd)                           {}
func showError(s string)                                { fmt.Fprintln(os.Stderr, s) }
func defaultBrowser(link string) error {
	cmd := exec.Command("xdg-open", link)
	if e := cmd.Start(); e != nil {
		return e
	}
	go cmd.Wait()
	return nil
}
func activateAppWindow(link, prefer, dir string) error { return openBrowserApp(link, prefer, dir) }
func serveDesktop(server *Server, prefer, dir string) error {
	if e := openBrowserApp(server.origin, prefer, dir); e != nil {
		return e
	}
	done := make(chan struct{})
	defer close(done)
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		started := time.Now()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if (server.hadClient.Load() && time.Now().Unix()-server.lastSeen.Load() > 300) ||
					(!server.hadClient.Load() && time.Since(started) > 10*time.Minute) {
					server.shutdown()
					return
				}
			}
		}
	}()
	return server.HTTP.Serve(server.listener)
}
func instanceLock(dir string) (func(), bool, error) {
	f, e := os.OpenFile(filepath.Join(dir, "instance.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, false, e
	}
	e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if e == syscall.EWOULDBLOCK {
		f.Close()
		return func() {}, false, nil
	}
	if e != nil {
		f.Close()
		return nil, false, e
	}
	return func() { syscall.Flock(int(f.Fd()), syscall.LOCK_UN); f.Close() }, true, nil
}
