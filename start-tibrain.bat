@echo off
REM TiBrain Service Starter - Kills old process on port 3005 before starting
setlocal enabledelayedexpansion

echo [TiBrain] Checking for existing processes on port 3005...

REM Kill any process listening on port 3005
for /f "tokens=5" %%a in ('netstat -ano ^| findstr ":3005 "') do (
    echo [TiBrain] Killing process PID %%a on port 3005
    taskkill /F /PID %%a 2>nul
)

REM Also check for tibrain.exe
tasklist /FI "IMAGENAME eq tibrain.exe" 2>NUL | find /I /N "tibrain.exe">NUL
if "%ERRORLEVEL%"=="0" (
    echo [TiBrain] Stopping existing tibrain.exe
    taskkill /F /IM tibrain.exe 2>nul
)

REM Wait a moment
timeout /t 2 /nobreak >nul

echo [TiBrain] Starting TiBrain service...
if exist Z:\03_DATA\bin\tibrain.exe (
    start /b Z:\03_DATA\bin\tibrain.exe
) else (
    echo [TiBrain] Binary not found at Z:\03_DATA\bin\tibrain.exe
    echo [TiBrain] Building first...
    set GOOS=windows
    set GOARCH=amd64
    go build -o Z:\03_DATA\bin\tibrain.exe .
    if errorlevel 1 (
        echo [TiBrain] Build failed!
        exit /b 1
    )
    start /b Z:\03_DATA\bin\tibrain.exe
)

echo [TiBrain] Service started on port 3005
endlocal
