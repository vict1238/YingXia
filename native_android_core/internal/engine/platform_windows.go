// SPDX-License-Identifier: GPL-3.0-or-later
package engine

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

var kernel32 = syscall.NewLazyDLL("kernel32.dll")
var moveFileEx = kernel32.NewProc("MoveFileExW")
var createMutex = kernel32.NewProc("CreateMutexW")
var closeHandle = kernel32.NewProc("CloseHandle")
var globalFree = kernel32.NewProc("GlobalFree")
var winhttp = syscall.NewLazyDLL("winhttp.dll")
var getIEProxy = winhttp.NewProc("WinHttpGetIEProxyConfigForCurrentUser")

func replaceFile(from, to string) error {
	f, e := syscall.UTF16PtrFromString(from)
	if e != nil {
		return e
	}
	t, e := syscall.UTF16PtrFromString(to)
	if e != nil {
		return e
	}
	r, _, e := moveFileEx.Call(uintptr(unsafe.Pointer(f)), uintptr(unsafe.Pointer(t)), 0x1|0x8)
	if r == 0 {
		return e
	}
	return nil
}
func instanceLock(dir string) (func(), bool, error) {
	name := fmt.Sprintf("Local\\YingXia_%x", sha256.Sum256([]byte(strings.ToLower(filepath.Clean(dir)))))
	p, e := syscall.UTF16PtrFromString(name)
	if e != nil {
		return nil, false, e
	}
	h, _, err := createMutex.Call(0, 0, uintptr(unsafe.Pointer(p)))
	if h == 0 {
		return nil, false, err
	}
	if err == syscall.ERROR_ALREADY_EXISTS {
		closeHandle.Call(h)
		return func() {}, false, nil
	}
	return func() { closeHandle.Call(h) }, true, nil
}

type ieProxyConfig struct {
	AutoDetect    uint32
	AutoConfigURL *uint16
	Proxy         *uint16
	ProxyBypass   *uint16
}

func readWide(p *uint16) string {
	if p == nil {
		return ""
	}
	buf := []uint16{}
	for i := uintptr(0); i < 16384; i++ {
		c := *(*uint16)(unsafe.Pointer(uintptr(unsafe.Pointer(p)) + i*2))
		if c == 0 {
			break
		}
		buf = append(buf, c)
	}
	return syscall.UTF16ToString(buf)
}
func proxyForRequest(r *http.Request) (*url.URL, error) {
	p, e := http.ProxyFromEnvironment(r)
	if e != nil || p != nil {
		return p, e
	}
	var cfg ieProxyConfig
	ok, _, _ := getIEProxy.Call(uintptr(unsafe.Pointer(&cfg)))
	if ok == 0 {
		return nil, nil
	}
	defer func() {
		for _, p := range []*uint16{cfg.AutoConfigURL, cfg.Proxy, cfg.ProxyBypass} {
			if p != nil {
				globalFree.Call(uintptr(unsafe.Pointer(p)))
			}
		}
	}()
	raw := readWide(cfg.Proxy)
	if raw == "" {
		return nil, nil
	}
	selected := ""
	if strings.Contains(raw, "=") {
		for _, piece := range strings.Split(raw, ";") {
			k, v, ok := strings.Cut(strings.TrimSpace(piece), "=")
			if ok && k == r.URL.Scheme {
				selected = strings.TrimSpace(v)
				break
			}
		}
	} else {
		selected = strings.TrimSpace(strings.Split(raw, ";")[0])
	}
	if selected == "" {
		return nil, nil
	}
	if !strings.Contains(selected, "://") {
		selected = "http://" + selected
	}
	p, e = url.Parse(selected)
	if e != nil || p.Hostname() == "" {
		return nil, nil
	}
	return p, nil
}
