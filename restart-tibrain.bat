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

if not exist Z:\03_DATA\bin\tibrain.exe (
    echo [TiBrain] Binary not found, building first...
    cd /d Z:\01_PROJECTS\apps\tibrain
    set GOOS=windows
    set GOARCH=amd64
    set GOGC=off
    go build -p=1 -o Z:\03_DATA\bin\tibrain.exe .
    if errorlevel 1 (
        echo [TiBrain] Build failed!
        pause
        exit /b 1
    )
    echo [TiBrain] Build successful: Z:\03_DATA\bin\tibrain.exe
)

REM Start TiBrain
start /b Z:\03_DATA\bin\tibrain.exe --port 3005 --host 127.0.0.1

REM Wait and verify
timeout /t 3 /nobreak >nul
curl -s -m 5 http://localhost:3005/health >nul 2>&1 && (
    echo [TiBrain] MCP server running at http://localhost:3005/mcp
    echo [TiBrain] SSE endpoint: http://localhost:3005/mcp/sse
    echo [TiBrain] Health check: OK
) || (
    echo [TiBrain] WARNING: Health check failed. Check stdout.log
)

pause