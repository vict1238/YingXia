@echo off
setlocal
cd /d "%~dp0"
where go >nul 2>nul
if errorlevel 1 (
 echo This developer build script requires Go 1.26 or newer. Users need the EXE and Microsoft WebView2 Runtime.
 pause
 exit /b 1
)
set GOOS=windows
set GOARCH=amd64
set CGO_ENABLED=0
set GOTOOLCHAIN=local
if not exist dist mkdir dist
go build -mod=vendor -trimpath -ldflags="-H windowsgui -s -w" -o dist\YingXia_v1.3.0_Windows_x64.exe .
if errorlevel 1 (pause & exit /b 1)
echo Built: dist\YingXia_v1.3.0_Windows_x64.exe
echo Uses an owned Windows window with WebView2. Microsoft WebView2 Runtime is not bundled.
pause
