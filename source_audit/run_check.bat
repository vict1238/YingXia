@echo off
chcp 65001 >nul
cd /d "%~dp0"
echo Source preflight 0.1 - NOT a video player.
where py >nul 2>nul
if %errorlevel% equ 0 (
    py -3 source_check.py --details %*
) else (
    where python >nul 2>nul
    if errorlevel 1 (
        echo Python not found. Install Python 3.10 or later from python.org.
        pause
        exit /b 1
    )
    python source_check.py --details %*
)
echo.
echo Results are in local_check_resultseport.txt and report.json.
pause
