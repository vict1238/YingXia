// SPDX-License-Identifier: GPL-3.0-or-later
// Package core exposes a small gomobile-compatible API shared by the Windows
// Flutter transport and the Android JNI bridge. No browser engine is included.
package core

import (
	"encoding/json"
	"sync"

	"yingxia.local/nativecore/internal/engine"
)

var runtimeMu sync.Mutex
var active *engine.Runtime

// Start starts the private loopback API and returns a JSON connection descriptor.
// On failure it returns {"error":"..."}, making the binding exception-free.
func Start(dataDir string) string {
	runtimeMu.Lock()
	defer runtimeMu.Unlock()
	if active != nil && active.Running() {
		return active.Bootstrap()
	}
	if active != nil {
		active.Stop()
		active = nil
	}
	r, e := startRuntime(dataDir)
	if e != nil {
		b, _ := json.Marshal(map[string]string{"error": e.Error()})
		return string(b)
	}
	active = r
	return r.Bootstrap()
}

// Stop shuts down searches, source updates, media requests and local storage.
func Stop() {
	runtimeMu.Lock()
	defer runtimeMu.Unlock()
	if active != nil {
		active.Stop()
		active = nil
	}
}

// Running reports whether the native backend is alive.
func Running() bool {
	runtimeMu.Lock()
	defer runtimeMu.Unlock()
	return active != nil && active.Running()
}

// AwaitStopped blocks until the current API exits. The Windows child uses this to exit
// after an authenticated /api/quit request as well as after host pipe closure.
func AwaitStopped() {
	runtimeMu.Lock()
	r := active
	runtimeMu.Unlock()
	if r != nil {
		r.Wait()
	}
}
