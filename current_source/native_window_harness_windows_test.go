// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"os"
	"testing"
)

// Explicit development harness runs the production entry point and native host.
// It is compiled only into the Go test artifact, never into the release EXE.
func TestNativeWindowHarness(t *testing.T) {
	dir := os.Getenv("YINGXIA_NATIVE_TEST_DIR")
	if dir == "" {
		t.Skip("manual native window harness not requested")
	}
	args := os.Args
	os.Args = []string{"YingXia", "--data-dir", dir}
	defer func() { os.Args = args }()
	if code := run(); code != 0 {
		t.Fatalf("native application exit code %d", code)
	}
}
