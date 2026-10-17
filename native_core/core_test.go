// SPDX-License-Identifier: GPL-3.0-or-later
package core

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"testing"
	"time"
)

func TestStopThenConcurrentStartWaitsForRuntimeRetirement(t *testing.T) {
	Stop()
	defer Stop()
	dir := t.TempDir()
	var first map[string]string
	if e := json.Unmarshal([]byte(Start(dir)), &first); e != nil || first["error"] != "" {
		t.Fatalf("initial start: %v %s", e, first["error"])
	}
	retiring := active
	u, e := url.Parse(first["origin"])
	if e != nil {
		t.Fatal(e)
	}
	conn, e := net.Dial("tcp", u.Host)
	if e != nil {
		t.Fatal(e)
	}
	defer conn.Close()
	_, e = fmt.Fprintf(conn, "POST /api/clear-history HTTP/1.1\r\nHost: %s\r\nCookie: %s\r\nOrigin: %s\r\nX-YingXia-CSRF: %s\r\nContent-Type: application/json\r\nContent-Length: 2\r\n\r\n{", u.Host, first["cookie"], first["origin"], first["csrf"])
	if e != nil {
		t.Fatal(e)
	}
	// The incomplete authenticated body keeps Shutdown in flight. The old
	// runtime becomes non-running before its file mutex is released.
	time.Sleep(100 * time.Millisecond)
	stopped := make(chan struct{})
	go func() { Stop(); close(stopped) }()
	deadline := time.Now().Add(time.Second)
	for retiring.Running() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if retiring.Running() {
		t.Fatal("runtime was not marked retiring")
	}
	restarted := make(chan string, 1)
	go func() { restarted <- Start(dir) }()
	select {
	case <-restarted:
		t.Fatal("Start returned before retiring runtime released its mutex")
	case <-time.After(100 * time.Millisecond):
	}
	_, _ = conn.Write([]byte("}"))
	select {
	case <-stopped:
	case <-time.After(6 * time.Second):
		t.Fatal("Stop failed to finish")
	}
	var next map[string]string
	select {
	case raw := <-restarted:
		if e = json.Unmarshal([]byte(raw), &next); e != nil {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		t.Fatal("Start did not resume after Stop")
	}
	if next["error"] != "" || next["origin"] == first["origin"] || next["cookie"] == first["cookie"] {
		t.Fatal("Start returned stale descriptor or already-running error")
	}
}
