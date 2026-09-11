# TiBrain Full Startup Script
# Chạy script này trong PowerShell (Run as Administrator nếu cần)

# Luôn chạy từ thư mục chứa script (tránh lệch CWD khi gọi từ nơi khác)
Set-Location -Path $PSScriptRoot

Write-Host "=== TiBrain Startup Sequence ===" -ForegroundColor Cyan

# Step 1: Health check current
Write-Host "`n[1/7] Checking current health on port 3005..." -ForegroundColor Yellow
try {
    $response = Invoke-WebRequest -Uri "http://localhost:3005/api/health" -Method GET -TimeoutSec 5 -UseBasicParsing
    Write-Host "  TiBrain already running! Status: $($response.StatusCode) - $($response.Content)" -ForegroundColor Green
    Write-Host "`n=== ALREADY RUNNING - NO RESTART NEEDED ===" -ForegroundColor Green
    exit 0
} catch {
    Write-Host "  Not running or unreachable: $($_.Exception.Message)" -ForegroundColor Yellow
}

# Step 2: Kill existing process on port 3005 + đảm bảo binary cũ không file-locked
Write-Host "`n[2/7] Checking for existing process on port 3005..." -ForegroundColor Yellow
$netstat = netstat -ano | findstr :3005
if ($netstat) {
    Write-Host "  Found existing process(es):" -ForegroundColor Yellow
    Write-Host "  $netstat"
    $lines = $netstat -split "`n"
    foreach ($line in $lines) {
        if ($line.Trim()) {
            $parts = $line.Trim() -split '\s+'
            $procId = $parts[-1]
            if ($procId -match '^\d+$') {
                Write-Host "  Killing PID $procId..." -ForegroundColor Red
                taskkill /F /PID $procId 2>$null
            }
        }
    }
    Start-Sleep -Seconds 1
} else {
    Write-Host "  No existing process on port 3005" -ForegroundColor Green
}

# Kill mọi process tibrain.exe còn sót (Windows file-lock: không build đè được exe đang chạy)
$tibrainProcs = Get-Process -Name "tibrain" -ErrorAction SilentlyContinue
if ($tibrainProcs) {
    foreach ($proc in $tibrainProcs) {
        Write-Host "  Killing leftover tibrain.exe (PID $($proc.Id))..." -ForegroundColor Red
        Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue
    }
    Start-Sleep -Seconds 1
}

# Step 3: Check for conflict markers
Write-Host "`n[3/7] Checking for git conflict markers in main.go and config.yaml..." -ForegroundColor Yellow
$conflictFound = $false
foreach ($file in @("main.go", "config.yaml")) {
    if (Test-Path $file) {
        $content = Get-Content $file -Raw
        if ($content -match '<<<<<<<|=======|>>>>>>>') {
            Write-Host "  CONFLICT MARKERS FOUND in $file!" -ForegroundColor Red
            $conflictFound = $true
        } else {
            Write-Host "  $file: OK (no conflict markers)" -ForegroundColor Green
        }
    }
}
if ($conflictFound) {
    Write-Host "`n=== BUILD BLOCKED: Resolve git conflicts first ===" -ForegroundColor Red
    exit 1
}

# Step 4: Build
Write-Host "`n[4/7] Building TiBrain (go build -o tibrain.exe .)..." -ForegroundColor Yellow
$buildResult = go build -o tibrain.exe . 2>&1
if ($LASTEXITCODE -ne 0) {
    Write-Host "  BUILD FAILED:" -ForegroundColor Red
    Write-Host "  $buildResult" -ForegroundColor Red
    Write-Host "`n=== BUILD FAILED ===" -ForegroundColor Red
    exit 1
} else {
    Write-Host "  Build SUCCESS" -ForegroundColor Green
    if ($buildResult) { Write-Host "  Output: $buildResult" }
}

# Step 5: Start server in background
Write-Host "`n[5/7] Starting TiBrain server (background)..." -ForegroundColor Yellow
$process = Start-Process -FilePath ".\tibrain.exe" -ArgumentList "--port","3005","--host","127.0.0.1" -WindowStyle Hidden -PassThru
Write-Host "  Started with PID: $($process.Id)" -ForegroundColor Green

# Step 6: Wait and verify health
Write-Host "`n[6/7] Waiting 3 seconds for server to start..." -ForegroundColor Yellow
Start-Sleep -Seconds 3

Write-Host "`n[7/7] Verifying health endpoints..." -ForegroundColor Yellow

# Check /api/health
try {
    $health = Invoke-WebRequest -Uri "http://localhost:3005/api/health" -Method GET -TimeoutSec 5 -UseBasicParsing
    Write-Host "  /api/health: $($health.StatusCode) - $($health.Content)" -ForegroundColor Green
} catch {
    Write-Host "  /api/health: FAILED - $($_.Exception.Message)" -ForegroundColor Red
}

# Check /mcp/sse bằng curl.exe: Invoke-WebRequest buffer toàn bộ response,
# SSE stream không kết thúc nên luôn timeout/FAILED giả dù server chạy tốt
Write-Host "  /mcp/sse: " -NoNewline
$curlExe = Get-Command curl.exe -ErrorAction SilentlyContinue
if ($curlExe) {
    # --max-time giới hạn thời gian stream; server OK nếu có HTTP header event đầu tiên
    $sseOutput = & curl.exe -s -m 5 -N -H "Accept: text/event-stream" http://localhost:3005/mcp/sse 2>&1
    if ($LASTEXITCODE -eq 0 -or $LASTEXITCODE -eq 28) {
        # exit 28 = timeout vì stream vẫn đang chạy (chính là hành vi SSE đúng)
        if ($sseOutput -match 'event:|data:') {
            Write-Host "OK - Connected (SSE stream active)" -ForegroundColor Green
        } else {
            Write-Host "FAILED - kết nối được nhưng không thấy SSE event" -ForegroundColor Red
        }
    } else {
        Write-Host "FAILED - curl exit code: $LASTEXITCODE" -ForegroundColor Red
    }
} else {
    Write-Host "SKIPPED - curl.exe not found" -ForegroundColor Yellow
}

# Check TiRouter on 3004
try {
    $tirouter = Invoke-WebRequest -Uri "http://localhost:3004/healthz" -Method GET -TimeoutSec 5 -UseBasicParsing
    Write-Host "  TiRouter (3004): $($tirouter.StatusCode) - $($tirouter.Content)" -ForegroundColor Green
} catch {
    Write-Host "  TiRouter (3004): FAILED - $($_.Exception.Message)" -ForegroundColor Yellow
}

Write-Host "`n=== STARTUP COMPLETE ===" -ForegroundColor Cyan
Write-Host "TiBrain PID: $($process.Id)" -ForegroundColor Cyan
Write-Host "Logs will appear in the background process window" -ForegroundColor Cyan