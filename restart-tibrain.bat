@echo off
REM =============================================================================
REM TiBrain Restart Script - Theo AGENTS.md
REM =============================================================================
REM Purpose: Clean port 3005 và restart TiBrain
REM Port: 3005 | TiRouter: 3004
REM =============================================================================

echo [TiBrain] Cleaning port 3005 before restart...

REM Find và kill process đang dùng port 3005
for /f "tokens=5" %%a in ('netstat -ano ^| findstr :3005') do (
    echo [TiBrain] Killing process %%a on port 3005
    taskkill /F /PID %%a >nul 2>&1
)

REM Wait 2 seconds để port release
timeout /t 2 /nobreak >nul

echo [TiBrain] Starting TiBrain on port 3005...

REM Build with GOGC=off workaround for Go 1.26.6 + modernc.org/sqlite compiler bug
REM GOGC=off disables GC during build, -p=1 forces single-threaded compilation
REM This prevents segfault in SSA/DSE passes (signal 0xc000001d)
REM
REM Lưu ý 2026-09-12: binary mới KHÔNG đọc flag --port/--host (main.go đọc env
REM TIBRAIN_PORT/TIBRAIN_HOST). Phải set env TRƯỚC khi start.

if not exist Z:\03_DATA\bin\tibrain.exe (
    echo [TiBrain] Binary not found, building first...
    cd /d Z:\01_PROJECTS\apps\products\tibrain
    set GOOS=windows
    set GOARCH=amd64
    set GOGC=off
    go build -p=1 -o Z:\03_DATA\bin\tibrain.exe .
    if errorlevel 1 (
        echo [TiBrain] Build failed!
        exit /b 1
    )
    echo [TiBrain] Build successful: Z:\03_DATA\bin\tibrain.exe
)

REM Start TiBrain — port/host qua env (binary bỏ flag --port/--host)
set TIBRAIN_PORT=3005
set TIBRAIN_HOST=127.0.0.1

REM Auth: đọc bearer token từ Z:\00_SECRET\router.env (single source of truth
REM theo security rules — KHÔNG hardcode token trong script)
REM PowerShell used because router.env is UTF-16 and findstr fails silently
set "ROUTER_ENV=Z:\00_SECRET\router.env"
set "TIBRAIN_MCP_BEARER_TOKEN="
if exist "%ROUTER_ENV%" (
    for /f "usebackq delims=" %%a in (`powershell -NoProfile -Command "$c=Get-Content '%ROUTER_ENV%' -Raw; $l=($c -split '\r?\n') | Where-Object { $_ -match '^TIBRAIN_MCP_BEARER_TOKEN=' }; if ($l) { ($l -split '=',2)[1] }"`) do (
        set "TIBRAIN_MCP_BEARER_TOKEN=%%a"
    )
)
if not defined TIBRAIN_MCP_BEARER_TOKEN (
    echo [TiBrain] WARNING: TIBRAIN_MCP_BEARER_TOKEN khong co trong router.env — MCP /mcp se tu choi moi request - fail-closed.
)

start /b Z:\03_DATA\bin\tibrain.exe

REM Wait and verify
timeout /t 3 /nobreak >nul
curl -s -m 5 http://localhost:3005/health >nul 2>&1 && (
    echo [TiBrain] MCP server running at http://localhost:3005/mcp
    echo [TiBrain] SSE endpoint: http://localhost:3005/mcp/sse
    echo [TiBrain] Health check: OK
) || (
    echo [TiBrain] WARNING: Health check failed. Check stdout.log
    exit /b 1
)

exit /b 0