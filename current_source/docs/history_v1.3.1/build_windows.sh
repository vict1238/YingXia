#!/usr/bin/env sh
set -eu
cd "$(dirname "$0")"
mkdir -p dist
# Runtime requirement: Microsoft WebView2 Runtime on the Windows machine.
GOTOOLCHAIN=local GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -mod=vendor -trimpath -ldflags='-H windowsgui -s -w' -o dist/YingXia_v1.3.1_Windows_x64.exe .
