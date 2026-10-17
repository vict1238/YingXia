// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func dataDirectory() string {
	if s := os.Getenv("YINGXIA_DATA_DIR"); s != "" {
		p, _ := filepath.Abs(s)
		return p
	}
	if runtime.GOOS == "windows" {
		p := os.Getenv("LOCALAPPDATA")
		if p == "" {
			h, _ := os.UserHomeDir()
			p = filepath.Join(h, "AppData", "Local")
		}
		return filepath.Join(p, "YingXia", "standalone")
	}
	h, _ := os.UserHomeDir()
	p := os.Getenv("XDG_DATA_HOME")
	if p == "" {
		p = filepath.Join(h, ".local", "share")
	}
	return filepath.Join(p, "yingxia-standalone")
}
func findBrowser(prefer string) string {
	if runtime.GOOS == "windows" {
		targets := []string{"Microsoft/Edge/Application/msedge.exe", "Google/Chrome/Application/chrome.exe"}
		if prefer == "chrome" {
			targets[0], targets[1] = targets[1], targets[0]
		}
		for _, t := range targets {
			for _, key := range []string{"PROGRAMFILES(X86)", "PROGRAMFILES", "LOCALAPPDATA"} {
				if base := os.Getenv(key); base != "" {
					p := filepath.Join(base, filepath.FromSlash(t))
					if f, e := os.Stat(p); e == nil && !f.IsDir() {
						return p
					}
				}
			}
		}
		return ""
	}
	for _, name := range []string{"microsoft-edge", "google-chrome", "chromium", "chromium-browser"} {
		if p, e := exec.LookPath(name); e == nil {
			return p
		}
	}
	return ""
}
func openBrowserApp(link, prefer, dir string) error {
	if prefer != "default" {
		if p := findBrowser(prefer); p != "" {
			cmd := exec.Command(p, "--app="+link, "--user-data-dir="+filepath.Join(dir, "browser-profile"), "--no-first-run", "--no-default-browser-check", "--window-size=1340,900")
			// The child is a GUI browser; do not request SW_HIDE on its window.
			if e := cmd.Start(); e == nil {
				go cmd.Wait()
				return nil
			}
		}
	}
	return defaultBrowser(link)
}
func readSession(dir string) string {
	b, e := os.ReadFile(filepath.Join(dir, "session.json"))
	if e != nil {
		return ""
	}
	var o Obj
	if json.Unmarshal(b, &o) != nil {
		return ""
	}
	u, e := url.Parse(sget(o, "origin"))
	if e != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.User != nil || u.Port() == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return ""
	}
	return u.String()
}
func run() int {
	fs := flag.NewFlagSet("YingXia", flag.ContinueOnError)
	noBrowser := fs.Bool("no-browser", false, "仅启动后台")
	port := fs.Int("port", 0, "本机端口，0 为自动分配")
	dirFlag := fs.String("data-dir", "", "用户数据目录")
	browser := fs.String("browser", "auto", "兼容参数，仅非 Windows 开发环境使用")
	showVersion := fs.Bool("version", false, "显示版本")
	if e := fs.Parse(os.Args[1:]); e != nil {
		return 2
	}
	if *showVersion {
		fmt.Println("YingXia " + version + " / " + runtime.GOOS + " " + runtime.GOARCH)
		return 0
	}
	if *port < 0 || *port > 65535 || (*browser != "auto" && *browser != "chrome" && *browser != "default") {
		showError("启动参数不正确。")
		return 2
	}
	dir := dataDirectory()
	if *dirFlag != "" {
		dir, _ = filepath.Abs(*dirFlag)
	}
	if e := os.MkdirAll(dir, 0700); e != nil {
		showError("无法创建数据目录：" + dir)
		return 1
	}
	unlock, held, e := instanceLock(dir)
	if e != nil {
		showError("无法取得应用锁：" + e.Error())
		return 1
	}
	if !held {
		for i := 0; i < 30; i++ {
			if link := readSession(dir); link != "" {
				if !*noBrowser {
					if e := activateAppWindow(link, *browser, dir); e != nil {
						showError(e.Error())
					}
				}
				fmt.Println("APP_URL=" + link)
				return 0
			}
			time.Sleep(100 * time.Millisecond)
		}
		showError("映匣已在运行，请检查现有窗口。")
		return 0
	}
	defer unlock()
	logpath := filepath.Join(dir, "app.log")
	if st, e := os.Stat(logpath); e == nil && st.Size() > 2000000 {
		_ = os.Remove(logpath + ".1")
		_ = os.Rename(logpath, logpath+".1")
	}
	lf, e := os.OpenFile(logpath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		showError("无法写入日志，请检查目录权限。")
		return 1
	}
	defer lf.Close()
	log.SetOutput(lf)
	service, e := newService(dir, nil)
	if e != nil {
		log.Print("startup error: ", e)
		showError("启动失败：" + e.Error() + "\n日志：" + logpath)
		return 1
	}
	updatesCtx, cancelUpdates := context.WithCancel(context.Background())
	defer cancelUpdates()
	service.startSourceUpdates(updatesCtx)
	server, e := newServer(service, *port)
	if e != nil {
		showError("无法启动本机服务：" + e.Error())
		return 1
	}
	defer server.listener.Close()
	b, _ := json.Marshal(Obj{"origin": server.origin, "pid": os.Getpid(), "version": version})
	_ = os.WriteFile(filepath.Join(dir, "session.json"), b, 0600)
	defer os.Remove(filepath.Join(dir, "session.json"))
	fmt.Println("映匣独立 EXE 后台已启动，仅监听本机。")
	fmt.Println("APP_URL=" + server.origin)
	log.Print("application started ", version)
	done := make(chan struct{})
	defer close(done)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	defer signal.Stop(sig)
	go func() {
		select {
		case <-sig:
			server.shutdown()
		case <-done:
		}
	}()
	if *noBrowser {
		e = server.HTTP.Serve(server.listener)
	} else {
		e = serveDesktop(server, *browser, dir)
	}
	if e != nil && !errors.Is(e, http.ErrServerClosed) {
		log.Printf("serve failed type=%T", e)
		showError("启动或运行失败：" + e.Error() + "\n日志：" + logpath)
		return 1
	}
	log.Print("application stopped")
	return 0
}
func main() {
	defer func() {
		if r := recover(); r != nil {
			showError("程序遇到未处理异常。请重新启动；若反复出现，请提供 app.log。")
			os.Exit(1)
		}
	}()
	os.Exit(run())
}
func safeLink(s string) bool { return strings.HasPrefix(s, "http://127.0.0.1:") }
