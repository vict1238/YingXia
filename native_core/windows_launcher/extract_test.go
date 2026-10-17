// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func testBundle(t *testing.T) ([]byte, bundleManifest) {
	t.Helper()
	var data bytes.Buffer
	writer := zip.NewWriter(&data)
	manifest := bundleManifest{Version: appVersion}
	for _, name := range []string{"yingxia_native.exe", "yingxia_backend.exe", "flutter_windows.dll", "data/app.so"} {
		body := []byte("synthetic test fixture " + name)
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(body); err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(body)
		manifest.Files = append(manifest.Files, bundleFile{name, int64(len(body)), hex.EncodeToString(digest[:])})
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data.Bytes())
	manifest.BundleSHA256 = hex.EncodeToString(digest[:])
	return data.Bytes(), manifest
}

func TestBundleCacheExtractionAndRepair(t *testing.T) {
	bundle, manifest := testBundle(t)
	cache := filepath.Join(t.TempDir(), "cache")
	runtime, err := prepareRuntime(cache, bundle, manifest)
	if err != nil {
		t.Fatal(err)
	}
	files, err := manifestFiles(manifest)
	if err != nil || !verifiedRuntime(runtime, files) {
		t.Fatal("extracted runtime not verified", err)
	}
	marker := filepath.Join(cache, "unrelated-data.txt")
	if err := os.WriteFile(marker, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtime, "yingxia_native.exe"), []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	restored, err := prepareRuntime(cache, bundle, manifest)
	if err != nil || restored != runtime || !verifiedRuntime(restored, files) {
		t.Fatal("cache not repaired", err)
	}
	if body, err := os.ReadFile(marker); err != nil || string(body) != "preserve" {
		t.Fatal("unrelated data modified")
	}
	quarantined, err := filepath.Glob(runtime + ".invalid-*")
	if err != nil || len(quarantined) != 1 {
		t.Fatal("damaged cache not preserved", err)
	}
	if _, err := prepareRuntime(cache, bundle, manifest); err != nil {
		t.Fatal(err)
	}
}

func TestBundleRejectsTraversalDuplicateAndCorruption(t *testing.T) {
	for _, name := range []string{"../outside", "a/../b", "C:stream", "/absolute", "..\\outside", ""} {
		t.Run(name, func(t *testing.T) {
			if _, err := safeEntry(name); err == nil {
				t.Fatal("unsafe entry accepted")
			}
		})
	}
	bundle, manifest := testBundle(t)
	manifest.Files = append(manifest.Files, manifest.Files[0])
	if _, err := prepareRuntime(filepath.Join(t.TempDir(), "cache"), bundle, manifest); err == nil {
		t.Fatal("duplicate accepted")
	}
	bundle, manifest = testBundle(t)
	manifest.Files[0].SHA256 = "0000000000000000000000000000000000000000000000000000000000000000"
	if _, err := prepareRuntime(filepath.Join(t.TempDir(), "cache"), bundle, manifest); err == nil {
		t.Fatal("corrupt content accepted")
	}
}

func TestRemoveOwnStageGuard(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "cache")
	if err := os.MkdirAll(cache, 0700); err != nil {
		t.Fatal(err)
	}
	if err := removeOwnStage(cache, filepath.Dir(cache)); err == nil {
		t.Fatal("parent removal accepted")
	}
	if err := removeOwnStage(cache, cache); err == nil {
		t.Fatal("cache root removal accepted")
	}
	stage, err := os.MkdirTemp(cache, ".extract-")
	if err != nil {
		t.Fatal(err)
	}
	if err := removeOwnStage(cache, stage); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stage); !os.IsNotExist(err) {
		t.Fatal("own stage not removed")
	}
}
