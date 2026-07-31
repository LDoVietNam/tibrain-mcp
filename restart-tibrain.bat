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

REM Start TiBrain
if exist Z:\03_DATA\bin\tibrain.exe (
    start /b Z:\03_DATA\bin\tibrain.exe
) else (
    echo [TiBrain] Binary not found, building first...
    cd /d Z:\01_PROJECTS\apps\tibrain
    set GOOS=windows
    set GOARCH=amd64
    go build -o Z:\03_DATA\bin\tibrain.exe .
    if errorlevel 1 (
        echo [TiBrain] Build failed!
        pause
        exit /b 1
    )
    start /b Z:\03_DATA\bin\tibrain.exe
)

pause