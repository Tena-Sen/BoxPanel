@echo off
chcp 65001 >nul 2>&1
title sbpanel build
setlocal enabledelayedexpansion

if not exist "%GOROOT%\bin\go.exe" (
  echo [X] Go not in GOROOT. Set GOROOT env or add Go to PATH.
  echo     Download: https://go.dev/dl/
  pause
  exit /b 1
)

cd /d "%~dp0"

echo === Building backend (Windows amd64) ===
"%GOROOT%\bin\go.exe" build -ldflags="-s -w" -o bin\sbpanel.exe .\cmd\panel
if errorlevel 1 (
  echo [X] Go build failed
  pause
  exit /b 1
)

echo === Building frontend ===
pushd frontend
if not exist node_modules (
  call npm install --legacy-peer-deps
  if errorlevel 1 (
    echo [X] npm install failed
    popd
    pause
    exit /b 1
  )
)
call npm run build
if errorlevel 1 (
  echo [X] frontend build failed
  popd
  pause
  exit /b 1
)
popd

echo.
echo === Build complete ===
echo   bin\sbpanel.exe   (Windows binary)
echo   internal\web\dist\  (embedded frontend)
echo.
echo Run with: 启动 sbpanel.bat
pause
