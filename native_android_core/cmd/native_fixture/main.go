// SPDX-License-Identifier: GPL-3.0-or-later
//go:build native_fixture

package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"yingxia.local/nativecore/internal/engine"
)

func main() {
	dir := flag.String("data-dir", "", "isolated fixture data directory required")
	flag.Parse()
	if *dir == "" {
		fmt.Println(`{"error":"fixture requires --data-dir"}`)
		os.Exit(2)
	}
	r, e := engine.NewFixtureRuntime(*dir)
	if e != nil {
		fmt.Println(`{"error":"fixture startup failed"}`)
		os.Exit(1)
	}
	defer r.Stop()
	fmt.Println(r.Bootstrap())
	go func() { _, _ = io.Copy(io.Discard, os.Stdin); r.Stop() }()
	r.Wait()
}
