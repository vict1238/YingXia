@echo off
setlocal
chcp 65001 >nul
cd /d "%~dp0"
title YingXia v1.0 - Local Application Service

echo ==================================================
echo   YingXia v1.0 - Windows source edition
 echo   Python 3.10+ is required. No pip packages needed.
echo ==================================================
echo.

py -3 -c "import sys; sys.exit(0 if sys.version_info >= (3,10) else 1)" >nul 2>&1
if not errorlevel 1 (
    py -3 main.py %*
    goto finished
)
python -c "import sys; sys.exit(0 if sys.version_info >= (3,10) else 1)" >nul 2>&1
if not errorlevel 1 (
    python main.py %*
    goto finished
)
echo Python 3.10 or newer was not found.
echo Install 64-bit Python 3.12 or 3.13 from the official website:
echo https://www.python.org/downloads/windows/
echo Enable "Add python.exe to PATH" during installation.
echo Then double-click START.bat again.
echo See START_HERE.html for Chinese instructions.
echo.
pause
exit /b 1

:finished
if errorlevel 1 (
    echo.
    echo Application stopped with an error.
    echo Please keep the error text and check START_HERE.html.
    pause
)
endlocal
