@echo off
setlocal
cd /d "%~dp0"
py -3 -m unittest discover -s tests -t . -v
if errorlevel 1 echo Tests did not all pass. Read the output above.
pause
