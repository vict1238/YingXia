// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"

	core "yingxia.local/nativecore"
)

func defaultDir() string {
	if p := os.Getenv("YINGXIA_DATA_DIR"); p != "" {
		return p
	}
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("LOCALAPPDATA"), "YingXia", "standalone")
	}
	p, _ := os.UserConfigDir()
	return filepath.Join(p, "YingXia", "native")
}

func main() {
	dir := flag.String("data-dir", defaultDir(), "private application data directory")
	version := flag.Bool("version", false, "display native backend version")
	flag.Parse()
	if *version {
		fmt.Println("YingXia 2.0.0-native")
		return
	}
	bootstrap := core.Start(*dir)
	fmt.Println(bootstrap)
	if !core.Running() {
		os.Exit(1)
	}
	defer core.Stop()
	go func() { _, _ = io.Copy(io.Discard, os.Stdin); core.Stop() }()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	defer signal.Stop(sig)
	go func() { <-sig; core.Stop() }()
	core.AwaitStopped()
}
