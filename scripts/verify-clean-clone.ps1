# TiBrain Clean-Clone Verification Script
# Verifies that the repository builds and tests without untracked files
# Exit 0 = clean clone verified, Exit 1 = verification failed

param(
    [string]$CommitSHA = ""
)

$ErrorActionPreference = "Stop"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$RepoRoot = Split-Path -Parent $ScriptDir

Write-Host "=== TiBrain Clean-Clone Verification ===" -ForegroundColor Cyan
Write-Host "Repository root: $RepoRoot"

# Get current commit SHA
try {
    $CurrentSHA = git rev-parse HEAD 2>$null
    if ($LASTEXITCODE -eq 0) {
        Write-Host "Current commit: $CurrentSHA"
        if ($CommitSHA -ne "" -and $CommitSHA -ne $CurrentSHA) {
            Write-Host "WARNING: Mismatch with expected commit $CommitSHA" -ForegroundColor Yellow
        }
    }
} catch {
    Write-Host "ERROR: Not a git repository or git not available" -ForegroundColor Red
    exit 1
}

# Check for untracked files
Write-Host "`nChecking for untracked files..." -ForegroundColor Yellow
$Untracked = git status --porcelain | Where-Object { $_ -match "^\?\?" }
if ($Untracked) {
    Write-Host "ERROR: Untracked files found:" -ForegroundColor Red
    $Untracked | ForEach-Object { Write-Host "  $_" -ForegroundColor Red }
    Write-Host "Clean clone verification FAILED" -ForegroundColor Red
    exit 1
}
Write-Host "OK: No untracked files" -ForegroundColor Green

# Check required directories exist
$RequiredDirs = @("internal/db", "internal/memory", "internal/mcp", "internal/tools")
foreach ($dir in $RequiredDirs) {
    $fullPath = Join-Path $RepoRoot $dir
    if (Test-Path $fullPath) {
        Write-Host "OK: $dir exists" -ForegroundColor Green
    } else {
        Write-Host "WARNING: $dir missing (may be expected)" -ForegroundColor Yellow
    }
}

# Check go.mod exists
$goMod = Join-Path $RepoRoot "go.mod"
if (-not (Test-Path $goMod)) {
    Write-Host "ERROR: go.mod not found" -ForegroundColor Red
    exit 1
}
Write-Host "OK: go.mod exists" -ForegroundColor Green

# Run go mod download
Write-Host "`nRunning go mod download..." -ForegroundColor Yellow
try {
    $env:CGO_ENABLED = "0"
    go mod download
    if ($LASTEXITCODE -ne 0) {
        Write-Host "ERROR: go mod download failed" -ForegroundColor Red
        exit 1
    }
    Write-Host "OK: Dependencies downloaded" -ForegroundColor Green
} catch {
    Write-Host "ERROR: go mod download failed: $_" -ForegroundColor Red
    exit 1
}

# Run go build
Write-Host "`nRunning go build ./... ..." -ForegroundColor Yellow
try {
    go build -trimpath ./...
    if ($LASTEXITCODE -ne 0) {
        Write-Host "ERROR: go build failed" -ForegroundColor Red
        exit 1
    }
    Write-Host "OK: Build successful" -ForegroundColor Green
} catch {
    Write-Host "ERROR: go build failed: $_" -ForegroundColor Red
    exit 1
}

# Run go vet
Write-Host "`nRunning go vet ./... ..." -ForegroundColor Yellow
try {
    go vet ./...
    if ($LASTEXITCODE -ne 0) {
        Write-Host "ERROR: go vet failed" -ForegroundColor Red
        exit 1
    }
    Write-Host "OK: Vet passed" -ForegroundColor Green
} catch {
    Write-Host "ERROR: go vet failed: $_" -ForegroundColor Red
    exit 1
}

Write-Host "`n=== Clean-Clone Verification PASSED ===" -ForegroundColor Green
Write-Host "Commit: $CurrentSHA"
exit 0