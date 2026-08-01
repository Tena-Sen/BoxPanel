#!/bin/sh
# sbpanel cross-platform build script
# Usage: ./build.sh [platform]   e.g. ./build.sh linux-amd64
set -e

PLATFORM="${1:-$(uname | tr '[:upper:]' '[:lower:]')-amd64}"
echo "=== Building for $PLATFORM ==="

cd "$(dirname "$0")"

# 1. Build frontend
if [ ! -d "frontend/node_modules" ]; then
  echo "--- npm install ---"
  (cd frontend && npm install --legacy-peer-deps)
fi
echo "--- vite build ---"
(cd frontend && npm run build)

# 2. Build backend for target platform
echo "--- go build ---"
GOOS="${PLATFORM%%-*}" GOARCH="${PLATFORM##*-}" go build -ldflags="-s -w" -o "bin/sbpanel-$PLATFORM" ./cmd/panel
echo "  -> bin/sbpanel-$PLATFORM"

echo ""
echo "=== Build complete ==="
echo "Run: ./bin/sbpanel-$PLATFORM"
