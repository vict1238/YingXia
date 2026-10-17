//go:build windows

// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"testing"
	"unsafe"

	"github.com/wailsapp/go-webview2/pkg/combridge"
	"github.com/wailsapp/go-webview2/pkg/edge"
	"golang.org/x/sys/windows"
)

// Exercise the native call boundary, where generated wrappers pass the wrong
// pointer/value forms. These callbacks use the actual Windows syscall ABI.
func TestNativePolicyCOMABI(t *testing.T) {
	stringValue, err := windows.UTF16FromString("http://127.0.0.1:49327/?q=test#workspace")
	if err != nil {
		t.Fatal(err)
	}
	duplicate := windows.NewLazySystemDLL("shlwapi.dll").NewProc("SHStrDupW")
	stringGetter := edge.NewComProc(func(_ uintptr, out **uint16) uintptr {
		hr, _, _ := duplicate.Call(uintptr(unsafe.Pointer(&stringValue[0])), uintptr(unsafe.Pointer(out)))
		return hr
	})
	if got, err := nativeCOMString(stringGetter, nil); err != nil || got != "http://127.0.0.1:49327/?q=test#workspace" {
		t.Fatalf("COM string out-pointer: got %q, %v", got, err)
	}
	boolGetter := edge.NewComProc(func(_ uintptr, out *int32) uintptr { *out = 1; return 0 })
	if got, err := nativeCOMBool(boolGetter, nil); err != nil || !got {
		t.Fatalf("COM BOOL out-pointer: got %v, %v", got, err)
	}
	var passedBool uintptr
	boolSetter := edge.NewComProc(func(_ uintptr, value uintptr) uintptr { passedBool = value; return 0 })
	if hr := nativeSetTrue(boolSetter, nil); hr != 0 || passedBool != 1 {
		t.Fatalf("COM BOOL value: HRESULT %x, argument %x", hr, passedBool)
	}
	const token int64 = 0x00123456789abcde
	var removedToken uintptr
	remove := edge.NewComProc(func(_ uintptr, value uintptr) uintptr { removedToken = value; return 0 })
	nativeRemoveEvent(remove, nil, token)
	if removedToken != uintptr(token) {
		t.Fatalf("COM token must pass by value: got %x", removedToken)
	}
}

func TestNativePolicyHandlerCOMLifetime(t *testing.T) {
	policy := &nativeWebViewPolicy{origin: "http://127.0.0.1:49327"}
	handler := combridge.New[nativeNavigationHandler](policy)
	defer handler.Close()
	type callbackVtbl struct {
		nativeUnknownVtbl
		Invoke edge.ComProc
	}
	type callbackObject struct{ Vtbl *callbackVtbl }
	object := (*callbackObject)(unsafe.Pointer(combridge.IUnknownFromUintptr(handler.Ref())))
	iid, err := windows.GUIDFromString("{9adbe429-f36d-432b-9ddc-f8881fbd76e3}")
	if err != nil {
		t.Fatal(err)
	}
	var queried *callbackObject
	hr, _, _ := object.Vtbl.QueryInterface.Call(uintptr(unsafe.Pointer(object)), uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&queried)))
	if hr != 0 || queried == nil {
		t.Fatalf("callback interface query failed: %x", hr)
	}
	defer queried.Vtbl.Release.Call(uintptr(unsafe.Pointer(queried)))
	// A failed string getter must fail closed, through the real native Invoke
	// trampoline and combridge object lookup, with a by-value Cancel BOOL.
	var canceled uintptr
	args := nativeNavigationArgs{Vtbl: &nativeNavigationArgsVtbl{
		GetUri:    edge.NewComProc(func(_ uintptr, _ **uint16) uintptr { return uintptr(windows.E_FAIL) }),
		PutCancel: edge.NewComProc(func(_ uintptr, value uintptr) uintptr { canceled = value; return 0 }),
	}}
	hr, _, _ = queried.Vtbl.Invoke.Call(uintptr(unsafe.Pointer(queried)), 0, uintptr(unsafe.Pointer(&args)))
	if hr != 0 || canceled != 1 {
		t.Fatalf("native callback must cancel unreadable navigation: HRESULT %x, Cancel %x", hr, canceled)
	}
	unknown, _ := windows.GUIDFromString("{11111111-2222-3333-4444-555555555555}")
	var unsupported uintptr = 1
	hr, _, _ = object.Vtbl.QueryInterface.Call(uintptr(unsafe.Pointer(object)), uintptr(unsafe.Pointer(&unknown)), uintptr(unsafe.Pointer(&unsupported)))
	if uint32(hr) != uint32(windows.E_NOINTERFACE) || unsupported != 0 {
		t.Fatalf("unknown interface must fail and clear output: HRESULT %x, output %x", hr, unsupported)
	}
}

func TestNativeTopLevelNavigationPolicy(t *testing.T) {
	const origin = "http://127.0.0.1:49327"
	for _, target := range []string{origin, origin + "/", origin + "/?q=film#workspace", origin + "#search"} {
		if !nativeTopLevelAllowed(origin, target) {
			t.Errorf("application navigation blocked: %q", target)
		}
	}
	for _, target := range []string{
		"https://example.com/", "http://localhost:49327/", "http://127.0.0.1:49328/", "https://127.0.0.1:49327/",
		origin + "/api/state", origin + "/%2f", origin + "/./", origin + "/app.js", "http://user@127.0.0.1:49327/",
		"http://127.0.0.1:49327.example.com/", "http://127.0.0.1:49327@evil.example/", "file:///C:/Windows/win.ini", "javascript:alert(1)",
		"data:text/html,hello", "about:blank", "/", " " + origin, origin + "\n", origin + "\\example.com",
	} {
		if nativeTopLevelAllowed(origin, target) {
			t.Errorf("foreign/non-UI navigation allowed: %q", target)
		}
	}
	for _, badOrigin := range []string{"https://127.0.0.1:49327", "http://localhost:49327", "http://127.0.0.1", "http://127.0.0.1:0", "http://127.0.0.1:65536", origin + "/api/state", origin + "/?query=1"} {
		if nativeTopLevelAllowed(badOrigin, badOrigin) {
			t.Errorf("invalid application origin accepted: %q", badOrigin)
		}
	}
}

func TestNativeExternalWindowPolicy(t *testing.T) {
	for _, target := range []string{"https://www.iqiyi.com/v_123.html", "https://search.bilibili.com/all?keyword=%E5%8A%A8%E6%BC%AB", "http://8.8.8.8/"} {
		if _, ok := nativeExternalLink(target, true); !ok {
			t.Errorf("public user link blocked: %q", target)
		}
		if _, ok := nativeExternalLink(target, false); ok {
			t.Errorf("script popup allowed: %q", target)
		}
	}
	for _, target := range []string{
		"file:///C:/Windows/win.ini", "javascript:alert(1)", "ms-settings:privacy", "data:text/html,hello", "about:blank",
		"https://user:password@example.com/", "http://127.0.0.1:49327/", "http://127.0.0.1/", "http://192.168.1.1/", "http://[::1]/",
		"http://localhost/", "http://router.local/", "http://2130706433/", "http://0x7f000001/", "http://example.com:9222/", "/relative",
		"https://example.com\\@localhost/", " https://example.com/", "https://example.com/\n",
	} {
		if _, ok := nativeExternalLink(target, true); ok {
			t.Errorf("unsafe external link allowed: %q", target)
		}
	}
}
