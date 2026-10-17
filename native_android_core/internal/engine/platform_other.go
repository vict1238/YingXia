// SPDX-License-Identifier: GPL-3.0-or-later
//go:build !windows

package engine

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"syscall"
)

func replaceFile(from, to string) error                 { return os.Rename(from, to) }
func proxyForRequest(r *http.Request) (*url.URL, error) { return http.ProxyFromEnvironment(r) }
func instanceLock(dir string) (func(), bool, error) {
	f, e := os.OpenFile(filepath.Join(dir, ".native.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, false, e
	}
	e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if e == syscall.EWOULDBLOCK {
		_ = f.Close()
		return func() {}, false, nil
	}
	if e != nil {
		_ = f.Close()
		return nil, false, e
	}
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, true, nil
}
