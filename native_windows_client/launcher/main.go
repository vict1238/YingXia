// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

//go:embed payload/*
var assets embed.FS

const appVersion = "2.3.0-native-windows"

func main() {
	if err := run(); err != nil {
		showError("映匣无法启动：\n" + err.Error())
		os.Exit(1)
	}
}

func run() error {
	flags := flag.NewFlagSet("YingXia", flag.ContinueOnError)
	dataDir := flags.String("data-dir", "", "optional isolated data directory")
	version := flags.Bool("version", false, "show version")
	if err := flags.Parse(os.Args[1:]); err != nil {
		return err
	}
	if *version {
		fmt.Println(appVersion)
		return nil
	}
	if flags.NArg() != 0 {
		return errors.New("命令行参数无效")
	}
	local := os.Getenv("LOCALAPPDATA")
	if local == "" {
		return errors.New("未找到本机应用数据目录")
	}
	selectedData := *dataDir
	if selectedData == "" {
		selectedData = os.Getenv("YINGXIA_DATA_DIR")
	}
	if selectedData == "" {
		selectedData = filepath.Join(local, "YingXia", "standalone")
	}
	selectedData, err := filepath.Abs(selectedData)
	if err != nil {
		return err
	}
	identity := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(selectedData))))
	windowClass := "YingXiaNativeWindow_" + hex.EncodeToString(identity[:8])
	unlock, held, err := claimLauncher(windowClass)
	if err != nil {
		return err
	}
	defer unlock()
	if !held {
		restoreExistingWindow(windowClass)
		return nil
	}
	bundle, err := assets.ReadFile("payload/bundle.zip")
	if err != nil {
		return errors.New("原生运行包未编入程序，请重新构建")
	}
	rawManifest, err := assets.ReadFile("payload/manifest.json")
	if err != nil {
		return err
	}
	var manifest bundleManifest
	if err := json.Unmarshal(rawManifest, &manifest); err != nil {
		return err
	}
	digest := sha256.Sum256(bundle)
	if hex.EncodeToString(digest[:]) != manifest.BundleSHA256 {
		return errors.New("原生运行包校验失败")
	}
	cache := filepath.Join(local, "YingXia", "native-runtime-v2")
	if override := os.Getenv("YINGXIA_NATIVE_CACHE_DIR"); override != "" {
		cache = override
	}
	cache, err = filepath.Abs(cache)
	if err != nil || filepath.Dir(cache) == cache {
		return errors.New("原生运行目录无效")
	}
	runtimeDir, err := prepareRuntime(cache, bundle, manifest)
	if err != nil {
		return err
	}
	job, err := newChildJob()
	if err != nil {
		return err
	}
	defer job.Close()
	command := exec.Command(filepath.Join(runtimeDir, "yingxia_native.exe"))
	command.Dir = runtimeDir
	backend := os.Getenv("YINGXIA_NATIVE_BACKEND")
	if backend == "" {
		backend = filepath.Join(runtimeDir, "yingxia_backend.exe")
	}
	command.Env = append(os.Environ(), "YINGXIA_NATIVE_BACKEND="+backend, "YINGXIA_DATA_DIR="+selectedData, "YINGXIA_NATIVE_WINDOW_CLASS="+windowClass)
	configureChild(command)
	if err := command.Start(); err != nil {
		return fmt.Errorf("原生窗口无法打开：%w", err)
	}
	if err := job.Assign(command.Process.Pid); err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return err
	}
	if err := command.Wait(); err != nil {
		return fmt.Errorf("原生窗口异常退出：%w", err)
	}
	return nil
}
