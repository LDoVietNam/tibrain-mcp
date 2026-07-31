#!/usr/bin/env pwsh
$ErrorActionPreference = "Stop"

$TIBRAIN_DIR = "Z:\01_PROJECTS\apps\tibrain"
$BINARY_NAME = "tibrain.exe"

Write-Host "Building TiBrain..." -ForegroundColor Green

# Check if Go is installed
try {
    $goVersion = go version 2>$null
    if (-not $goVersion) {
        Write-Error "Go is not installed or not in PATH"
        exit 1
    }
    Write-Host "Go version: $goVersion" -ForegroundColor Cyan
} catch {
    Write-Error "Go is not installed or not in PATH"
    exit 1
}

# Build binary
Write-Host "Building binary..." -ForegroundColor Green
go build -o "$TIBRAIN_DIR\$BINARY_NAME" -ldflags "-s -w" "$TIBRAIN_DIR\main.go"

if (Test-Path "$TIBRAIN_DIR\$BINARY_NAME") {
    Write-Host "Build successful: $TIBRAIN_DIR\$BINARY_NAME" -ForegroundColor Green
    $size = (Get-Item "$TIBRAIN_DIR\$BINARY_NAME").Length
    Write-Host "Size: $([math]::Round($size / 1MB, 2)) MB" -ForegroundColor Cyan
} else {
    Write-Error "Build failed: binary not found"
    exit 1
}

# Run tests
Write-Host "Running tests..." -ForegroundColor Green
go test ./... 2>$null

Write-Host "Build complete!" -ForegroundColor Green
