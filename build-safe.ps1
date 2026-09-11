#!/usr/bin/env pwsh
# Safe build script for TiBrain - uses Go from Z:\09_TOOLS\go
# Usage: .\build-safe.ps1

$ErrorActionPreference = "Stop"

$PROJECT_DIR = "Z:\01_PROJECTS\apps\products\tibrain"
$RELEASES_DIR = "$PROJECT_DIR\bin\releases"
$BINARY_NAME = "tibrain.exe"
$GO_VERSION = "1.26.6"
$GO_BIN = "Z:\09_TOOLS\go\bin\go.exe"

Write-Host "=== TiBrain Safe Build ===" -ForegroundColor Cyan

# 1. Set environment
$env:GOTOOLCHAIN = "auto"

# 2. Verify Go installation
$goVersion = & $GO_BIN version 2>$null
Write-Host "Go binary: $GO_BIN" -ForegroundColor Gray
Write-Host "Go version: $goVersion" -ForegroundColor Gray

# 3. Navigate to project
Set-Location $PROJECT_DIR

# 4. Ensure go.mod and go.work match installed Go version
$goModContent = Get-Content "go.mod" -Raw
if ($goModContent -notmatch "go $GO_VERSION") {
    Write-Host "Updating go.mod to match Go $GO_VERSION..." -ForegroundColor Yellow
    (Get-Content "go.mod") -replace "go \d+\.\d+\.\d+", "go $GO_VERSION" | Set-Content "go.mod"
}

$goWorkContent = Get-Content "go.work" -Raw
if ($goWorkContent -notmatch "go $GO_VERSION") {
    Write-Host "Updating go.work to match Go $GO_VERSION..." -ForegroundColor Yellow
    (Get-Content "go.work") -replace "go \d+\.\d+\.\d+", "go $GO_VERSION" | Set-Content "go.work"
}

# 5. Ensure releases directory exists
if (-not (Test-Path $RELEASES_DIR)) {
    New-Item -ItemType Directory -Path $RELEASES_DIR -Force | Out-Null
}

# 6. Build using explicit Go path - output to releases folder
$outputPath = Join-Path $RELEASES_DIR $BINARY_NAME
Write-Host "Building to: $outputPath" -ForegroundColor Green
& $GO_BIN build -o $outputPath .

# 7. Verify
if (Test-Path $outputPath) {
    $size = [math]::Round((Get-Item $outputPath).Length / 1MB, 2)
    Write-Host "Build successful: $outputPath ($size MB)" -ForegroundColor Green
} else {
    Write-Error "Build failed: binary not found"
    exit 1
}

Write-Host "=== Build Complete ===" -ForegroundColor Cyan
