// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/wailsapp/go-webview2/pkg/edge"
	"github.com/wailsapp/go-webview2/webviewloader"
	"golang.org/x/sys/windows"
)

const (
	wmClose                  = 0x0010
	wmNativeFullscreen       = 0x8001
	wmNativeToggleFullscreen = 0x8002
	wmNativeExitFullscreen   = 0x8003
	windowStyle              = 0x02CF0000 // WS_CLIPCHILDREN | WS_OVERLAPPEDWINDOW
)

var (
	nativeCreateWindow  = user32.NewProc("CreateWindowExW")
	nativeDefWindowProc = user32.NewProc("DefWindowProcW")
	nativePostMessage   = user32.NewProc("PostMessageW")
	nativeShowWindow    = user32.NewProc("ShowWindow")
	nativeSetWindowPos  = user32.NewProc("SetWindowPos")
	nativeGetWindowRect = user32.NewProc("GetWindowRect")
	nativeSetWindowLong = user32.NewProc("SetWindowLongPtrW")
	nativeGetWindowLong = user32.NewProc("GetWindowLongPtrW")
	nativeIsIconic      = user32.NewProc("IsIconic")
	nativeIsZoomed      = user32.NewProc("IsZoomed")
	nativeSetForeground = user32.NewProc("SetForegroundWindow")
	nativeHost          *nativeAppWindow // Accessed only by the STA UI thread.
)

type nativeRect struct{ Left, Top, Right, Bottom int32 }
type nativePoint struct{ X, Y int32 }
type nativeMessage struct {
	Window         uintptr
	Message        uint32
	WParam, LParam uintptr
	Time           uint32
	Point          nativePoint
	Private        uint32
}
type nativeWindowClass struct {
	Size, Style                        uint32
	Proc                               uintptr
	ClassExtra, WindowExtra            int32
	Instance, Icon, Cursor, Background uintptr
	Menu, Name                         *uint16
	SmallIcon                          uintptr
}
type nativeMonitorInfo struct {
	Size          uint32
	Monitor, Work nativeRect
	Flags         uint32
}
type nativeAppWindow struct {
	hwnd                                     uintptr
	web                                      *edge.Chromium
	policy                                   *nativeWebViewPolicy
	ready, closing, fullscreen, wasMaximized bool
	restoreRect                              nativeRect
	restoreStyle                             uintptr
}

func nativeWindowClassName(dir string) string {
	h := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(dir))))
	return fmt.Sprintf("YingXiaWindow_%x", h[:8])
}

// A second launch only activates the owned window. It never launches a browser.
func activateAppWindow(_ string, _ string, dir string) error {
	class, _ := syscall.UTF16PtrFromString(nativeWindowClassName(dir))
	for i := 0; i < 80; i++ {
		hwnd, _, _ := user32.NewProc("FindWindowW").Call(uintptr(unsafe.Pointer(class)), 0)
		if hwnd != 0 {
			if iconic, _, _ := nativeIsIconic.Call(hwnd); iconic != 0 {
				nativeShowWindow.Call(hwnd, 9) // SW_RESTORE
			}
			nativeSetForeground.Call(hwnd)
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("映匣后台已在运行。请先关闭旧版窗口和后台，再打开新版。")
}

func serveDesktop(server *Server, _ string, dir string) error {
	served := make(chan struct{})
	var serveErr error
	go func() {
		serveErr = server.HTTP.Serve(server.listener)
		close(served)
	}()
	windowErr := runNativeWindow(server.origin, dir, served)
	server.shutdown()
	<-served
	if windowErr != nil {
		return windowErr
	}
	return serveErr
}

func runNativeWindow(origin, dir string, serverDone <-chan struct{}) error {
	// COM callbacks, window messages and WebView calls stay on this STA thread.
	// pkg/edge also pins its initialization thread, which is the main thread.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err != nil && err != syscall.Errno(1) {
		return fmt.Errorf("无法初始化窗口组件：%w", err)
	}
	defer windows.CoUninitialize()
	runtimeVersion, err := webviewloader.GetAvailableCoreWebView2BrowserVersionString("")
	if err != nil || runtimeVersion == "" {
		return errors.New("缺少 Microsoft Edge WebView2 Runtime。请安装后重新打开映匣：\nhttps://developer.microsoft.com/microsoft-edge/webview2/")
	}
	log.Print("native WebView2 runtime ", runtimeVersion)
	// This affects only our process; support mixed-DPI screens on Windows 10/11.
	if p := user32.NewProc("SetProcessDpiAwarenessContext"); p.Find() == nil {
		p.Call(^uintptr(3))
	}
	class, _ := syscall.UTF16PtrFromString(nativeWindowClassName(dir))
	title, _ := syscall.UTF16PtrFromString("映匣 · YingXia")
	instance, _, _ := kernel32.NewProc("GetModuleHandleW").Call(0)
	// rsrc reserves ID 1 for the manifest, followed by the icon group at ID 2.
	icon, _, _ := user32.NewProc("LoadImageW").Call(instance, 2, 1, 0, 0, 0x8040)
	cursor, _, _ := user32.NewProc("LoadCursorW").Call(0, 32512)
	brush, _, _ := syscall.NewLazyDLL("gdi32.dll").NewProc("CreateSolidBrush").Call(0x13100F)
	defer syscall.NewLazyDLL("gdi32.dll").NewProc("DeleteObject").Call(brush)
	wc := nativeWindowClass{Proc: syscall.NewCallback(nativeWindowProc), Instance: instance, Name: class, Icon: icon, SmallIcon: icon, Cursor: cursor, Background: brush}
	wc.Size = uint32(unsafe.Sizeof(wc))
	if atom, _, e := user32.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&wc))); atom == 0 {
		return fmt.Errorf("无法注册窗口：%w", e)
	}
	defer user32.NewProc("UnregisterClassW").Call(uintptr(unsafe.Pointer(class)), instance)
	w := &nativeAppWindow{web: edge.NewChromium()}
	nativeHost = w
	defer func() { nativeHost = nil }()
	w.web.DataPath = filepath.Join(dir, "webview-profile")
	w.web.SetGlobalPermission(edge.CoreWebView2PermissionStateDeny)
	// Explicit developer opt-in, absent in normal launches; never disable TLS or sandboxing.
	if s := os.Getenv("YINGXIA_WEBVIEW_DEBUG_PORT"); s != "" {
		if port, e := strconv.Atoi(s); e == nil && port > 0 && port <= 65535 {
			w.web.AdditionalBrowserArgs = []string{fmt.Sprintf("--remote-debugging-port=%d", port), "--remote-debugging-address=127.0.0.1"}
			log.Print("developer WebView debugging enabled on loopback")
		}
	}
	w.web.SetErrorCallback(func(e error) {
		log.Print("native WebView2 error: ", e)
		_ = os.Remove(filepath.Join(dir, "session.json"))
		showError("窗口组件发生错误，请重新打开映匣。\n日志：" + filepath.Join(dir, "app.log"))
	})
	w.web.ContainsFullScreenElementChangedCallback = func(sender *edge.ICoreWebView2, _ *edge.ICoreWebView2ContainsFullScreenElementChangedEventArgs) {
		// COM BOOL is int32, not Go bool. Use the matching base-interface slot.
		if full, err := nativeContainsFullScreenElement(sender); err == nil {
			value := uintptr(0)
			if full {
				value = 1
			}
			nativePostMessage.Call(w.hwnd, wmNativeFullscreen, value, 0)
		}
	}
	w.web.AcceleratorKeyCallback = func(key uint) bool {
		if key == 0x7A {
			nativePostMessage.Call(w.hwnd, wmNativeToggleFullscreen, 0, 0)
			return true
		} // F11
		if key == 0x1B && w.fullscreen {
			nativePostMessage.Call(w.hwnd, wmNativeExitFullscreen, 0, 0)
			return true
		}
		return false
	}
	w.web.ProcessFailedCallback = func(_ *edge.ICoreWebView2, _ *edge.ICoreWebView2ProcessFailedEventArgs) {
		// Close cleanly instead of keeping a dead playback window and background server.
		log.Print("native WebView2 process failure")
		nativePostMessage.Call(w.hwnd, wmClose, 0, 0)
	}
	work := nativeRect{Right: 1340, Bottom: 900}
	user32.NewProc("SystemParametersInfoW").Call(48, 0, uintptr(unsafe.Pointer(&work)), 0)
	scale := int32(96)
	if p := user32.NewProc("GetDpiForSystem"); p.Find() == nil {
		dpi, _, _ := p.Call()
		if dpi > 0 {
			scale = int32(dpi)
		}
	}
	width, height := int32(1340)*scale/96, int32(900)*scale/96
	if width > work.Right-work.Left-24 {
		width = work.Right - work.Left - 24
	}
	if height > work.Bottom-work.Top-24 {
		height = work.Bottom - work.Top - 24
	}
	x, y := work.Left+(work.Right-work.Left-width)/2, work.Top+(work.Bottom-work.Top-height)/2
	w.hwnd, _, err = nativeCreateWindow.Call(0, uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(title)), windowStyle, uintptr(x), uintptr(y), uintptr(width), uintptr(height), 0, 0, instance, 0)
	if w.hwnd == 0 {
		return fmt.Errorf("无法创建窗口：%w", err)
	}
	defer func() {
		if !w.closing {
			nativeWindowProc(w.hwnd, wmClose, 0, 0)
		}
	}()
	// Avoid a light title bar and a white flash while the local UI is loading.
	dark := int32(1)
	syscall.NewLazyDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute").Call(w.hwnd, 20, uintptr(unsafe.Pointer(&dark)), 4)
	if !w.web.Embed(w.hwnd) || w.web.GetController() == nil {
		return errors.New("无法创建 WebView2 窗口。")
	}
	if w.closing {
		return nil
	}
	w.ready = true
	w.web.SetBackgroundColour(15, 20, 24, 255)
	settings, err := w.web.GetSettings()
	if err != nil {
		return err
	}
	for _, e := range []error{
		settings.PutAreHostObjectsAllowed(false), settings.PutIsWebMessageEnabled(false),
		settings.PutAreDevToolsEnabled(false), settings.PutIsStatusBarEnabled(false),
		settings.PutAreDefaultContextMenusEnabled(false), settings.PutAreDefaultScriptDialogsEnabled(false),
	} {
		if e != nil {
			return e
		}
	}
	w.policy, err = configureNativeWebView(w.web, origin)
	if err != nil {
		return fmt.Errorf("无法设置窗口导航：%w", err)
	}
	w.updateScale()
	w.web.Resize()
	w.web.Navigate(origin)
	// Wait until Embed has completed before allowing cross-thread close requests.
	watchDone := make(chan struct{})
	defer close(watchDone)
	go func() {
		select {
		case <-serverDone:
			nativePostMessage.Call(w.hwnd, wmClose, 0, 0)
		case <-watchDone:
		}
	}()
	nativeShowWindow.Call(w.hwnd, 5)
	user32.NewProc("UpdateWindow").Call(w.hwnd)
	w.web.Focus()
	log.Print("native window ready")
	var msg nativeMessage
	for {
		r, _, e := user32.NewProc("GetMessageW").Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) == -1 {
			return fmt.Errorf("窗口消息循环失败：%w", e)
		}
		if r == 0 {
			break
		}
		user32.NewProc("TranslateMessage").Call(uintptr(unsafe.Pointer(&msg)))
		user32.NewProc("DispatchMessageW").Call(uintptr(unsafe.Pointer(&msg)))
	}
	log.Print("native window closed")
	return nil
}

func (w *nativeAppWindow) updateScale() {
	if !w.ready {
		return
	}
	if c := w.web.GetController().GetICoreWebView2Controller3(); c != nil {
		_ = c.PutShouldDetectMonitorScaleChanges(true)
		c.Vtbl.Release.Call(uintptr(unsafe.Pointer(c)))
	}
}

func (w *nativeAppWindow) setFullscreen(full bool) {
	if full == w.fullscreen || !w.ready || w.closing {
		return
	}
	w.fullscreen = full
	if full {
		w.restoreStyle, _, _ = nativeGetWindowLong.Call(w.hwnd, ^uintptr(15))
		nativeGetWindowRect.Call(w.hwnd, uintptr(unsafe.Pointer(&w.restoreRect)))
		maximized, _, _ := nativeIsZoomed.Call(w.hwnd)
		w.wasMaximized = maximized != 0
		monitor, _, _ := user32.NewProc("MonitorFromWindow").Call(w.hwnd, 2)
		info := nativeMonitorInfo{Size: uint32(unsafe.Sizeof(nativeMonitorInfo{}))}
		if ok, _, _ := user32.NewProc("GetMonitorInfoW").Call(monitor, uintptr(unsafe.Pointer(&info))); ok == 0 {
			w.fullscreen = false
			return
		}
		nativeSetWindowLong.Call(w.hwnd, ^uintptr(15), w.restoreStyle&^0x00CF0000) // GWL_STYLE
		r := info.Monitor
		nativeSetWindowPos.Call(w.hwnd, 0, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right-r.Left), uintptr(r.Bottom-r.Top), 0x0024)
	} else {
		nativeSetWindowLong.Call(w.hwnd, ^uintptr(15), w.restoreStyle)
		r := w.restoreRect
		nativeSetWindowPos.Call(w.hwnd, 0, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right-r.Left), uintptr(r.Bottom-r.Top), 0x0024)
		if w.wasMaximized {
			nativeShowWindow.Call(w.hwnd, 3)
		}
	}
	w.web.Resize()
}

func nativeWindowProc(hwnd, msg, wp, lp uintptr) uintptr {
	w := nativeHost
	if w != nil {
		switch msg {
		case wmClose:
			if !w.closing {
				w.closing = true
				if w.ready {
					w.web.ShuttingDown()
					if w.policy != nil {
						w.policy.close()
					}
					_ = nativeCloseWebView(w.web.GetController())
				}
				user32.NewProc("DestroyWindow").Call(hwnd)
			}
			return 0
		case 0x0002: // WM_DESTROY
			user32.NewProc("PostQuitMessage").Call(0)
			return 0
		case 0x0005: // WM_SIZE
			if w.ready && !w.closing {
				if wp == 1 { // SIZE_MINIMIZED
					w.web.Eval("window.dispatchEvent(new Event('yingxia:window-hidden'))")
					_ = w.web.Hide()
				} else {
					_ = w.web.Show()
					w.web.Resize()
				}
			}
			return 0
		case 0x0003, 0x0216: // WM_MOVE / WM_MOVING
			if w.ready && !w.closing {
				_ = w.web.NotifyParentWindowPositionChanged()
			}
		case 0x0007: // WM_SETFOCUS
			if w.ready && !w.closing {
				w.web.Focus()
			}
		case 0x02E0: // WM_DPICHANGED
			if !w.fullscreen {
				var r nativeRect
				// LPARAM is a native RECT pointer valid for this message. Copy it
				// at the Win32 boundary rather than retaining it as a Go pointer.
				kernel32.NewProc("RtlMoveMemory").Call(uintptr(unsafe.Pointer(&r)), lp, unsafe.Sizeof(r))
				nativeSetWindowPos.Call(hwnd, 0, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right-r.Left), uintptr(r.Bottom-r.Top), 0x0014)
			}
			w.updateScale()
			return 0
		case wmNativeFullscreen:
			w.setFullscreen(wp != 0)
			return 0
		case wmNativeToggleFullscreen:
			if w.fullscreen {
				w.exitFullscreen()
			} else {
				w.setFullscreen(true)
			}
			return 0
		case wmNativeExitFullscreen:
			w.exitFullscreen()
			return 0
		}
	}
	r, _, _ := nativeDefWindowProc.Call(hwnd, msg, wp, lp)
	return r
}

func (w *nativeAppWindow) exitFullscreen() {
	if !w.ready || w.closing {
		return
	}
	// Host and DOM fullscreen are separate states and must leave together.
	w.web.Eval("if(document.fullscreenElement)document.exitFullscreen().catch(()=>{})")
	w.setFullscreen(false)
}
