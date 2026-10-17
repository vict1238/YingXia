// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/remote"
)

func publicMediaRequest(ctx context.Context, link string) bool {
	clean, e := publicURL(link)
	if e != nil {
		return false
	}
	_, e = lookupPublic(ctx, hostName(clean))
	return e == nil
}

// Only a fresh headless profile is used. Ordinary media URLs are observed;
// daily browser profiles, account credentials and DRM keys are never accessed.
func sniffMedia(ctx context.Context, dir, link string) (string, error) {
	return sniffMediaChecked(ctx, dir, link, publicMediaRequest)
}
func sniffMediaChecked(ctx context.Context, dir, link string, policy func(context.Context, string) bool) (string, error) {
	ctx, timeout := context.WithTimeout(ctx, 18*time.Second)
	defer timeout()
	if !policy(ctx, link) {
		return "", problem("播放页地址不可访问。", "unsafe_url")
	}
	browser := findBrowser("auto")
	if browser == "" {
		return "", problem("未找到浏览器。", "browser")
	}
	profile, e := os.MkdirTemp(dir, "resolver-profile-")
	if e != nil {
		return "", e
	}
	defer func() {
		absolute, _ := filepath.Abs(profile)
		root, _ := filepath.Abs(dir)
		if filepath.Dir(absolute) == root && strings.HasPrefix(filepath.Base(absolute), "resolver-profile-") {
			_ = os.RemoveAll(absolute)
		}
	}()
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	opts = append(opts, remote.WebSocket, chromedp.Flag("remote-debugging-address", "127.0.0.1"), chromedp.ExecPath(browser), chromedp.UserDataDir(profile), chromedp.Flag("autoplay-policy", "no-user-gesture-required"), chromedp.Flag("mute-audio", true), chromedp.ModifyCmdFunc(func(cmd *exec.Cmd) { hideProcess(cmd) }))
	allocator, cancelAllocator := chromedp.NewExecAllocator(ctx, opts...)
	defer cancelAllocator()
	tab, cancelTab := chromedp.NewContext(allocator)
	defer cancelTab()
	requests := chromedp.Events(tab, network.RequestWillBeSent)
	responses := chromedp.Events(tab, network.ResponseReceived)
	paused := chromedp.Events(tab, fetch.RequestPaused)
	result := make(chan string, 1)
	var hosts sync.Map
	observe := func(link string) {
		if !isMediaURL(link) {
			return
		}
		if policy(tab, link) {
			select {
			case result <- link:
			default:
			}
		}
	}
	go func() {
		for ev, e := range requests {
			if e != nil {
				return
			}
			observe(ev.Request.URL)
		}
	}()
	go func() {
		for ev, e := range responses {
			if e != nil {
				return
			}
			if strings.Contains(strings.ToLower(ev.Response.MimeType), "mpegurl") {
				if policy(tab, ev.Response.URL) {
					select {
					case result <- ev.Response.URL:
					default:
					}
				}
			} else {
				observe(ev.Response.URL)
			}
		}
	}()
	go func() {
		for ev, e := range paused {
			if e != nil {
				return
			}
			ev := ev
			go func() {
				key := hostName(ev.Request.URL)
				allowed := false
				if v, ok := hosts.Load(key); ok {
					allowed = v.(bool)
				} else {
					checkCtx, cancel := context.WithTimeout(tab, 3*time.Second)
					allowed = policy(checkCtx, ev.Request.URL)
					cancel()
					hosts.Store(key, allowed)
				}
				if allowed {
					_, _ = chromedp.Call(tab, fetch.ContinueRequest, fetch.ContinueRequestParams{RequestID: ev.RequestID})
				} else {
					_, _ = chromedp.Call(tab, fetch.FailRequest, fetch.FailRequestParams{RequestID: ev.RequestID, ErrorReason: network.ErrorReasonBlockedByClient})
				}
			}()
		}
	}()
	if _, e = chromedp.Call(tab, network.Enable, network.EnableParams{}); e != nil {
		return "", problem("浏览器取流初始化失败。", "browser_media")
	}
	if _, e = chromedp.Call(tab, fetch.Enable, fetch.EnableParams{}); e != nil {
		return "", problem("浏览器请求检查初始化失败。", "browser_media")
	}
	done := make(chan error, 1)
	go func() { done <- chromedp.Do(tab, chromedp.Navigate(link)) }()
	for {
		select {
		case link := <-result:
			return link, nil
		case e := <-done:
			if e != nil {
				return "", problem("网页取流未完成，可切换网页播放。", "browser_media")
			}
			done = nil
		case <-ctx.Done():
			return "", problem("网页取流超时，可切换网页播放。", "browser_media")
		}
	}
}
