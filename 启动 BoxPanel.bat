@echo off
chcp 65001 >nul 2>&1
title BoxPanel
cd /d "%~dp0"

if not exist "bin\boxpanel.exe" (
    echo.
    echo [ERROR] bin\boxpanel.exe not found!
    echo.
    echo Please build first:
    echo   build.cmd
    echo.
    pause
    exit /b 1
)

if not exist "sing-box.exe" (
    echo.
    echo [WARN] sing-box.exe not found, proxy core will not work.
    echo Download from https://github.com/SagerNet/sing-box/releases
    echo.
    choice /c yn /m "Continue anyway? y=yes n=no"
    if errorlevel 2 exit /b 0
)

echo.
echo  BoxPanel - sing-box management panel
echo  ------------------------------------
echo  URL:    http://127.0.0.1:7820
echo  Close:  Close this window or Ctrl+C
echo.

bin\boxpanel.exe

echo.
echo BoxPanel has exited.
pause