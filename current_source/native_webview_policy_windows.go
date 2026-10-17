//go:build windows

// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/wailsapp/go-webview2/pkg/combridge"
	"github.com/wailsapp/go-webview2/pkg/edge"

	"golang.org/x/sys/windows"
)

// Minimal local COM declarations avoid pkg/webview2: that generated package
// registers unsupported Go string callback arguments during init and panics.
// Method order follows the public Microsoft WebView2.h interface definitions.
type nativeUnknownVtbl struct {
	QueryInterface edge.ComProc
	AddRef         edge.ComProc
	Release        edge.ComProc
}
type nativeCoreWebViewVtbl struct {
	nativeUnknownVtbl
	GetSettings                            edge.ComProc
	GetSource                              edge.ComProc
	Navigate                               edge.ComProc
	NavigateToString                       edge.ComProc
	AddNavigationStarting                  edge.ComProc
	RemoveNavigationStarting               edge.ComProc
	AddContentLoading                      edge.ComProc
	RemoveContentLoading                   edge.ComProc
	AddSourceChanged                       edge.ComProc
	RemoveSourceChanged                    edge.ComProc
	AddHistoryChanged                      edge.ComProc
	RemoveHistoryChanged                   edge.ComProc
	AddNavigationCompleted                 edge.ComProc
	RemoveNavigationCompleted              edge.ComProc
	AddFrameNavigationStarting             edge.ComProc
	RemoveFrameNavigationStarting          edge.ComProc
	AddFrameNavigationCompleted            edge.ComProc
	RemoveFrameNavigationCompleted         edge.ComProc
	AddScriptDialogOpening                 edge.ComProc
	RemoveScriptDialogOpening              edge.ComProc
	AddPermissionRequested                 edge.ComProc
	RemovePermissionRequested              edge.ComProc
	AddProcessFailed                       edge.ComProc
	RemoveProcessFailed                    edge.ComProc
	AddScriptToExecuteOnDocumentCreated    edge.ComProc
	RemoveScriptToExecuteOnDocumentCreated edge.ComProc
	ExecuteScript                          edge.ComProc
	CapturePreview                         edge.ComProc
	Reload                                 edge.ComProc
	PostWebMessageAsJSON                   edge.ComProc
	PostWebMessageAsString                 edge.ComProc
	AddWebMessageReceived                  edge.ComProc
	RemoveWebMessageReceived               edge.ComProc
	CallDevToolsProtocolMethod             edge.ComProc
	GetBrowserProcessID                    edge.ComProc
	GetCanGoBack                           edge.ComProc
	GetCanGoForward                        edge.ComProc
	GoBack                                 edge.ComProc
	GoForward                              edge.ComProc
	GetDevToolsProtocolEventReceiver       edge.ComProc
	Stop                                   edge.ComProc
	AddNewWindowRequested                  edge.ComProc
	RemoveNewWindowRequested               edge.ComProc
	AddDocumentTitleChanged                edge.ComProc
	RemoveDocumentTitleChanged             edge.ComProc
	GetDocumentTitle                       edge.ComProc
	AddHostObjectToScript                  edge.ComProc
	RemoveHostObjectFromScript             edge.ComProc
	OpenDevToolsWindow                     edge.ComProc
	AddContainsFullScreenElementChanged    edge.ComProc
	RemoveContainsFullScreenElementChanged edge.ComProc
	GetContainsFullScreenElement           edge.ComProc
	AddWebResourceRequested                edge.ComProc
	RemoveWebResourceRequested             edge.ComProc
	AddWebResourceRequestedFilter          edge.ComProc
	RemoveWebResourceRequestedFilter       edge.ComProc
	AddWindowCloseRequested                edge.ComProc
	RemoveWindowCloseRequested             edge.ComProc
}
type nativeCoreWebView struct{ Vtbl *nativeCoreWebViewVtbl }
type nativeNavigationArgsVtbl struct {
	nativeUnknownVtbl
	GetUri             edge.ComProc
	GetIsUserInitiated edge.ComProc
	GetIsRedirected    edge.ComProc
	GetRequestHeaders  edge.ComProc
	GetCancel          edge.ComProc
	PutCancel          edge.ComProc
}
type nativeNavigationArgs struct{ Vtbl *nativeNavigationArgsVtbl }
type nativeNewWindowArgsVtbl struct {
	nativeUnknownVtbl
	GetUri             edge.ComProc
	PutNewWindow       edge.ComProc
	GetNewWindow       edge.ComProc
	PutHandled         edge.ComProc
	GetHandled         edge.ComProc
	GetIsUserInitiated edge.ComProc
}
type nativeNewWindowArgs struct{ Vtbl *nativeNewWindowArgsVtbl }
type nativeDownloadArgsVtbl struct {
	nativeUnknownVtbl
	GetDownloadOperation edge.ComProc
	GetCancel            edge.ComProc
	PutCancel            edge.ComProc
	GetResultFilePath    edge.ComProc
	PutResultFilePath    edge.ComProc
	GetHandled           edge.ComProc
	PutHandled           edge.ComProc
}
type nativeDownloadArgs struct{ Vtbl *nativeDownloadArgsVtbl }

func nativeContainsFullScreenElement(sender *edge.ICoreWebView2) (bool, error) {
	if sender == nil {
		return false, fmt.Errorf("WebView2 is not initialized")
	}
	view := (*nativeCoreWebView)(unsafe.Pointer(sender))
	return nativeCOMBool(view.Vtbl.GetContainsFullScreenElement, unsafe.Pointer(view))
}

func nativeCloseWebView(controller *edge.ICoreWebView2Controller) error {
	if controller == nil {
		return nil
	}
	type controllerVtbl struct {
		BeforeClose [24]edge.ComProc
		Close       edge.ComProc
	}
	view := (*struct{ Vtbl *controllerVtbl })(unsafe.Pointer(controller))
	hr, _, _ := view.Vtbl.Close.Call(uintptr(unsafe.Pointer(controller)))
	if int32(hr) < 0 {
		return syscall.Errno(hr)
	}
	return nil
}

// The event interface IDs and inherited vtable order are from Microsoft's
// WebView2.h. combridge owns native callback allocations and implements COM
// QueryInterface/AddRef/Release, keeping the Go handlers alive until release.
type nativeNavigationHandler interface {
	navigationStarting(*nativeNavigationArgs) uintptr
}
type nativeNewWindowHandler interface {
	newWindowRequested(*nativeNewWindowArgs) uintptr
}
type nativeDownloadHandler interface {
	downloadStarting(*nativeDownloadArgs) uintptr
}

func init() {
	combridge.RegisterVTable[combridge.IUnknown, nativeNavigationHandler]("{9adbe429-f36d-432b-9ddc-f8881fbd76e3}",
		func(this uintptr, _ *nativeCoreWebView, args *nativeNavigationArgs) uintptr {
			return combridge.Resolve[nativeNavigationHandler](this).navigationStarting(args)
		})
	combridge.RegisterVTable[combridge.IUnknown, nativeNewWindowHandler]("{d4c185fe-c81c-4989-97af-2d3fa7ab5651}",
		func(this uintptr, _ *nativeCoreWebView, args *nativeNewWindowArgs) uintptr {
			return combridge.Resolve[nativeNewWindowHandler](this).newWindowRequested(args)
		})
	combridge.RegisterVTable[combridge.IUnknown, nativeDownloadHandler]("{efedc989-c396-41ca-83f7-07f845a55724}",
		func(this uintptr, _ *nativeCoreWebView, args *nativeDownloadArgs) uintptr {
			return combridge.Resolve[nativeDownloadHandler](this).downloadStarting(args)
		})
}

// ICoreWebView2_4 inherits every method from ICoreWebView2, _2 and _3.
// The generated pkg/webview2 _4 vtable omits that inheritance; do not use it.
type nativeDownloadViewVtbl struct {
	nativeCoreWebViewVtbl
	Version2               [7]edge.ComProc
	Version3               [5]edge.ComProc
	AddFrameCreated        edge.ComProc
	RemoveFrameCreated     edge.ComProc
	AddDownloadStarting    edge.ComProc
	RemoveDownloadStarting edge.ComProc
}
type nativeDownloadView struct{ Vtbl *nativeDownloadViewVtbl }

type nativeWebViewPolicy struct {
	origin                                                        string
	view                                                          *nativeCoreWebView
	downloadView                                                  *nativeDownloadView
	navigation                                                    *combridge.ComObject[nativeNavigationHandler]
	newWindow                                                     *combridge.ComObject[nativeNewWindowHandler]
	download                                                      *combridge.ComObject[nativeDownloadHandler]
	navigationToken, newWindowToken, downloadToken                int64
	navigationRegistered, newWindowRegistered, downloadRegistered bool
	ctx                                                           context.Context
	cancel                                                        context.CancelFunc
	closed                                                        bool // configure, callbacks and close all run on the STA UI thread.
}

// configureNativeWebView is called after Embed, before the first Navigate.
// It provides no privileged JavaScript bridge and does not change iframe rules.
func configureNativeWebView(browser *edge.Chromium, origin string) (*nativeWebViewPolicy, error) {
	if !nativeTopLevelAllowed(origin, origin) {
		return nil, fmt.Errorf("invalid native application origin")
	}
	if browser == nil || browser.GetController() == nil {
		return nil, fmt.Errorf("WebView2 controller is not initialized")
	}
	view, err := browser.GetController().GetCoreWebView2()
	if err != nil {
		return nil, err
	}
	p := &nativeWebViewPolicy{origin: origin, view: (*nativeCoreWebView)(unsafe.Pointer(view))}
	p.ctx, p.cancel = context.WithCancel(context.Background())
	configured := false
	defer func() {
		if !configured {
			p.close()
		}
	}()
	p.navigation = combridge.New[nativeNavigationHandler](p)
	p.newWindow = combridge.New[nativeNewWindowHandler](p)
	p.download = combridge.New[nativeDownloadHandler](p)
	if err = nativeRegisterEvent(p.view.Vtbl.AddNavigationStarting, unsafe.Pointer(p.view), p.navigation.Ref(), &p.navigationToken); err != nil {
		return nil, err
	}
	p.navigationRegistered = true
	if err = nativeRegisterEvent(p.view.Vtbl.AddNewWindowRequested, unsafe.Pointer(p.view), p.newWindow.Ref(), &p.newWindowToken); err != nil {
		return nil, err
	}
	p.newWindowRegistered = true
	// DownloadStarting is available since Runtime 91. Require this interface so
	// a missing capability cannot silently enable downloads in the app window.
	iid, _ := windows.GUIDFromString("{20d02d59-6df2-42dc-bd06-f98a694b1302}")
	hr, _, _ := p.view.Vtbl.QueryInterface.Call(uintptr(unsafe.Pointer(p.view)), uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&p.downloadView)))
	if int32(hr) < 0 || p.downloadView == nil {
		return nil, fmt.Errorf("WebView2 download policy requires a newer runtime: HRESULT 0x%08x", uint32(hr))
	}
	if err = nativeRegisterEvent(p.downloadView.Vtbl.AddDownloadStarting, unsafe.Pointer(p.downloadView), p.download.Ref(), &p.downloadToken); err != nil {
		return nil, err
	}
	p.downloadRegistered = true
	configured = true
	return p, nil
}

func nativeRegisterEvent(proc edge.ComProc, object unsafe.Pointer, handler uintptr, token *int64) error {
	hr, _, _ := proc.Call(uintptr(object), handler, uintptr(unsafe.Pointer(token)))
	if int32(hr) < 0 {
		return syscall.Errno(hr)
	}
	return nil
}

func nativeRemoveEvent(proc edge.ComProc, object unsafe.Pointer, token int64) {
	// EventRegistrationToken is a signed 64-bit value passed by value on x64.
	hr, _, _ := proc.Call(uintptr(object), uintptr(token))
	if int32(hr) < 0 {
		log.Printf("WebView2 event removal failed: HRESULT 0x%08x", uint32(hr))
	}
}

func nativeCOMString(proc edge.ComProc, object unsafe.Pointer) (string, error) {
	var value *uint16
	hr, _, _ := proc.Call(uintptr(object), uintptr(unsafe.Pointer(&value)))
	if value != nil {
		defer windows.CoTaskMemFree(unsafe.Pointer(value))
	}
	if int32(hr) < 0 {
		return "", syscall.Errno(hr)
	}
	return windows.UTF16PtrToString(value), nil
}

func nativeCOMBool(proc edge.ComProc, object unsafe.Pointer) (bool, error) {
	var value int32 // Win32 BOOL is four bytes, unlike Go bool.
	hr, _, _ := proc.Call(uintptr(object), uintptr(unsafe.Pointer(&value)))
	if int32(hr) < 0 {
		return false, syscall.Errno(hr)
	}
	return value != 0, nil
}

func nativeSetTrue(proc edge.ComProc, object unsafe.Pointer) uintptr {
	hr, _, _ := proc.Call(uintptr(object), 1) // BOOL is passed by value.
	return hr
}

func (p *nativeWebViewPolicy) navigationStarting(args *nativeNavigationArgs) uintptr {
	if args == nil {
		return uintptr(windows.E_INVALIDARG)
	}
	target, err := nativeCOMString(args.Vtbl.GetUri, unsafe.Pointer(args))
	if p.closed || err != nil || !nativeTopLevelAllowed(p.origin, target) {
		return nativeSetTrue(args.Vtbl.PutCancel, unsafe.Pointer(args))
	}
	return 0
}

func (p *nativeWebViewPolicy) newWindowRequested(args *nativeNewWindowArgs) uintptr {
	if args == nil {
		return uintptr(windows.E_INVALIDARG)
	}
	// Always consume the request: the runtime must never create a popup itself.
	if hr := nativeSetTrue(args.Vtbl.PutHandled, unsafe.Pointer(args)); int32(hr) < 0 {
		return hr
	}
	if p.closed {
		return 0
	}
	initiated, err := nativeCOMBool(args.Vtbl.GetIsUserInitiated, unsafe.Pointer(args))
	if err != nil || !initiated {
		return 0
	}
	target, err := nativeCOMString(args.Vtbl.GetUri, unsafe.Pointer(args))
	if err != nil {
		return 0
	}
	link, ok := nativeExternalLink(target, initiated)
	if !ok {
		return 0
	}
	// DNS checking and Shell execution must not block WebView2's STA callbacks.
	go func() {
		ctx, cancel := context.WithTimeout(p.ctx, 5*time.Second)
		defer cancel()
		u, _ := url.Parse(link)
		if _, err := lookupPublic(ctx, u.Hostname()); err != nil || ctx.Err() != nil {
			return
		}
		if err := defaultBrowser(link); err != nil {
			log.Print("external browser could not be opened")
		}
	}()
	return 0
}

func (p *nativeWebViewPolicy) downloadStarting(args *nativeDownloadArgs) uintptr {
	if args == nil {
		return uintptr(windows.E_INVALIDARG)
	}
	if hr := nativeSetTrue(args.Vtbl.PutCancel, unsafe.Pointer(args)); int32(hr) < 0 {
		return hr
	}
	return nativeSetTrue(args.Vtbl.PutHandled, unsafe.Pointer(args))
}

// close must precede controller.Close and run on the same STA thread as Embed.
func (p *nativeWebViewPolicy) close() {
	if p == nil || p.closed {
		return
	}
	p.closed = true
	if p.cancel != nil {
		p.cancel()
	}
	if p.downloadRegistered {
		nativeRemoveEvent(p.downloadView.Vtbl.RemoveDownloadStarting, unsafe.Pointer(p.downloadView), p.downloadToken)
	}
	if p.newWindowRegistered {
		nativeRemoveEvent(p.view.Vtbl.RemoveNewWindowRequested, unsafe.Pointer(p.view), p.newWindowToken)
	}
	if p.navigationRegistered {
		nativeRemoveEvent(p.view.Vtbl.RemoveNavigationStarting, unsafe.Pointer(p.view), p.navigationToken)
	}
	if p.download != nil {
		_ = p.download.Close()
	}
	if p.newWindow != nil {
		_ = p.newWindow.Close()
	}
	if p.navigation != nil {
		_ = p.navigation.Close()
	}
	if p.downloadView != nil {
		p.downloadView.Vtbl.Release.Call(uintptr(unsafe.Pointer(p.downloadView)))
		p.downloadView = nil
	}
	if p.view != nil {
		p.view.Vtbl.Release.Call(uintptr(unsafe.Pointer(p.view)))
		p.view = nil
	}
}

func nativeTopLevelAllowed(origin, target string) bool {
	if len(target) > 8192 || hasControl(target) || strings.Contains(target, "\\") || strings.TrimSpace(target) != target {
		return false
	}
	base, err := url.Parse(origin)
	if err != nil || base.Scheme != "http" || base.Hostname() != "127.0.0.1" || base.User != nil || base.Opaque != "" || base.RawQuery != "" || base.Fragment != "" || (base.EscapedPath() != "" && base.EscapedPath() != "/") {
		return false
	}
	port, err := strconv.Atoi(base.Port())
	if err != nil || port < 1 || port > 65535 {
		return false
	}
	u, err := url.Parse(target)
	return err == nil && u.Opaque == "" && u.User == nil && u.Scheme == base.Scheme && u.Host == base.Host && (u.EscapedPath() == "" || u.EscapedPath() == "/")
}

func nativeExternalLink(target string, userInitiated bool) (string, bool) {
	if !userInitiated || strings.TrimSpace(target) != target {
		return "", false
	}
	link, err := publicURL(target)
	return link, err == nil
}
