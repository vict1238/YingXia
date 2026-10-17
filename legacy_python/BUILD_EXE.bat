@echo off
setlocal
cd /d "%~dp0"
echo This optional build downloads PyInstaller from PyPI.
echo It builds a Windows folder distribution locally, not a signed installer.
echo Normal use only requires START.bat; this step is optional.
choice /C YN /M "Continue with the local EXE build"
if errorlevel 2 exit /b 0
py -3 -m venv .build-venv
if errorlevel 1 goto error
.build-venv\Scripts\python.exe -m pip install --index-url https://pypi.org/simple pyinstaller==6.22.3
if errorlevel 1 goto error
.build-venv\Scripts\python.exe build_windows.py
if errorlevel 1 goto error
echo.
echo Build complete: dist\YingXia\YingXia.exe
echo Keep the entire dist\YingXia folder, not just the EXE.
echo This build has not been tested on your Windows PC yet.
pause
exit /b 0
:error
echo Build failed. Do not disable antivirus or TLS verification.
echo The original source edition can still be started with START.bat.
pause
exit /b 1
