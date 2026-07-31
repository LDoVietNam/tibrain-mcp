#!/data/data/com.termux/files/usr/bin/bash
# build-termux.sh — Build TiBrain binary for Termux Android (ARM64)
# Usage: ./build-termux.sh
set -e

echo "=== TiBrain Termux Build ==="
echo "Checking Go installation..."

if ! command -v go &>/dev/null; then
    echo "Go not found. Installing via pkg..."
    pkg update && pkg install -y golang
fi

go version

echo "Setting CGO_ENABLED=0 for cross-compile (no C dependencies)"
export CGO_ENABLED=0
export GOOS=android
export GOARCH=arm64

echo "Building binary..."
go build -o tibrain-android-arm64 -ldflags="-s -w -X main.version=1.0.0-termux" main.go

echo "Build complete: tibrain-android-arm64"
echo ""
echo "To run on Termux:"
echo "  ./tibrain-android-arm64 --port 3005"
echo "Or install to PATH:"
echo "  cp tibrain-android-arm64 \$PREFIX/bin/tibrain"