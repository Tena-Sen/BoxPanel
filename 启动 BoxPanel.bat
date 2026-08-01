@echo off
chcp 65001 >nul 2>&1
title BoxPanel
cd /d "%~dp0"

if not exist "boxpanel.exe" (
    echo.
    echo [ERROR] boxpanel.exe not found!
    echo.
    echo Please build first:
    echo   go build -o boxpanel.exe ./cmd/panel
    echo.
    pause
    exit /b 1
)

echo.
echo  BoxPanel - sing-box management panel
echo  ------------------------------------
echo  URL:    http://127.0.0.1:7820
echo  Close:  Close this window or Ctrl+C
echo.

boxpanel.exe

echo.
echo BoxPanel has exited.
pause