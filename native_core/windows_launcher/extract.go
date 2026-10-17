// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"archive/zip"
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type bundleFile struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}
type bundleManifest struct {
	Version      string       `json:"version"`
	BundleSHA256 string       `json:"bundle_sha256"`
	Files        []bundleFile `json:"files"`
}

func safeEntry(name string) (string, error) {
	name = strings.ReplaceAll(name, "\\", "/")
	if name == "" || strings.Contains(name, ":") || strings.HasPrefix(name, "/") || path.Clean(name) != name || name == "." || name == ".." || strings.HasPrefix(name, "../") {
		return "", errors.New("原生运行包含无效路径")
	}
	return name, nil
}

func manifestFiles(manifest bundleManifest) (map[string]bundleFile, error) {
	if manifest.Version != appVersion || len(manifest.Files) == 0 || len(manifest.Files) > 10000 {
		return nil, errors.New("原生运行包清单无效")
	}
	files := make(map[string]bundleFile, len(manifest.Files))
	var total int64
	for _, item := range manifest.Files {
		name, err := safeEntry(item.Path)
		if err != nil || item.Size < 0 || item.Size > 1<<30 || len(item.SHA256) != 64 {
			return nil, errors.New("原生运行文件清单无效")
		}
		if _, err := hex.DecodeString(item.SHA256); err != nil {
			return nil, err
		}
		key := strings.ToLower(name)
		if _, exists := files[key]; exists {
			return nil, errors.New("原生运行文件路径重复")
		}
		files[key] = item
		total += item.Size
	}
	if total > 2<<30 {
		return nil, errors.New("原生运行包过大")
	}
	for _, required := range []string{"yingxia_native.exe", "yingxia_backend.exe", "flutter_windows.dll", "data/app.so"} {
		if _, exists := files[required]; !exists {
			return nil, fmt.Errorf("原生运行包缺少 %s", required)
		}
	}
	return files, nil
}

func verifiedRuntime(dir string, files map[string]bundleFile) bool {
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return false
	}
	for _, item := range files {
		file := filepath.Join(dir, filepath.FromSlash(item.Path))
		info, err := os.Lstat(file)
		if err != nil || !info.Mode().IsRegular() || info.Size() != item.Size {
			return false
		}
		stream, err := os.Open(file)
		if err != nil {
			return false
		}
		hash := sha256.New()
		_, err = io.Copy(hash, stream)
		closeErr := stream.Close()
		if err != nil || closeErr != nil || hex.EncodeToString(hash.Sum(nil)) != item.SHA256 {
			return false
		}
	}
	return true
}

func removeOwnStage(cache, stage string) error {
	cache, _ = filepath.Abs(cache)
	stage, _ = filepath.Abs(stage)
	if filepath.Dir(stage) != cache || !strings.HasPrefix(filepath.Base(stage), ".extract-") {
		return errors.New("拒绝移除运行目录外的路径")
	}
	info, err := os.Lstat(stage)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("拒绝移除无效临时运行目录")
	}
	return os.RemoveAll(stage)
}

func prepareRuntime(cache string, bundle []byte, manifest bundleManifest) (string, error) {
	files, err := manifestFiles(manifest)
	if err != nil {
		return "", err
	}
	if len(manifest.BundleSHA256) != 64 {
		return "", errors.New("原生运行包标识无效")
	}
	if _, err := hex.DecodeString(manifest.BundleSHA256); err != nil {
		return "", err
	}
	target := filepath.Join(cache, "2.0.0-"+manifest.BundleSHA256[:16])
	if verifiedRuntime(target, files) {
		return target, nil
	}
	if err := os.MkdirAll(cache, 0700); err != nil {
		return "", err
	}
	stage, err := os.MkdirTemp(cache, ".extract-")
	if err != nil {
		return "", err
	}
	defer removeOwnStage(cache, stage)
	archive, err := zip.NewReader(bytes.NewReader(bundle), int64(len(bundle)))
	if err != nil {
		return "", err
	}
	seen := make(map[string]bool)
	for _, entry := range archive.File {
		if entry.FileInfo().IsDir() {
			continue
		}
		name, err := safeEntry(entry.Name)
		if err != nil || entry.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("原生运行包路径无效")
		}
		key := strings.ToLower(name)
		item, exists := files[key]
		if !exists || seen[key] || entry.UncompressedSize64 != uint64(item.Size) {
			return "", errors.New("原生运行包与文件清单不符")
		}
		seen[key] = true
		output := filepath.Join(stage, filepath.FromSlash(item.Path))
		if err := os.MkdirAll(filepath.Dir(output), 0700); err != nil {
			return "", err
		}
		input, err := entry.Open()
		if err != nil {
			return "", err
		}
		file, err := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			input.Close()
			return "", err
		}
		hash := sha256.New()
		count, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(input, item.Size+1))
		inputErr := input.Close()
		fileErr := file.Close()
		if copyErr != nil || inputErr != nil || fileErr != nil || count != item.Size || hex.EncodeToString(hash.Sum(nil)) != item.SHA256 {
			return "", errors.New("原生运行文件校验失败")
		}
	}
	if len(seen) != len(files) {
		return "", errors.New("原生运行包不完整")
	}
	if verifiedRuntime(target, files) {
		return target, nil
	}
	if _, err := os.Lstat(target); err == nil {
		var suffix [8]byte
		if _, err := rand.Read(suffix[:]); err != nil {
			return "", err
		}
		if err := os.Rename(target, target+".invalid-"+hex.EncodeToString(suffix[:])); err != nil {
			return "", errors.New("原生运行目录正在使用或无法恢复")
		}
	}
	if err := os.Rename(stage, target); err != nil {
		if verifiedRuntime(target, files) {
			return target, nil
		}
		return "", err
	}
	return target, nil
}
