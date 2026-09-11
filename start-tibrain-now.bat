@echo off
REM TiBrain launcher - bypass Bash tool restriction by double-click
REM Chay: Z:\01_PROJECTS\apps\products\tibrain\start-tibrain-now.bat

cd /d "%~dp0"

REM Kill old if exists on 3005
for /f "tokens=5" %%a in ('netstat -ano ^| findstr ":3005" ^| findstr LISTENING') do (
    echo [launcher] Killing old PID %%a
    taskkill /F /PID %%a >nul 2>&1
)

REM Build if binary missing
if not exist "Z:\03_DATA\bin\tibrain.exe" (
    echo [launcher] Building tibrain.exe...
    go build -o Z:\03_DATA\bin\tibrain.exe .
    if errorlevel 1 (
        echo [launcher] BUILD FAILED
        pause
        exit /b 1
    )
)

REM Start in background
echo [launcher] Starting TiBrain on :3005...
start "" /b "Z:\03_DATA\bin\tibrain.exe" --port 3005 --host 0.0.0.0
timeout /t 3 >nul

REM Verify
curl -s -m 5 http://localhost:3005/health && echo. || echo [launcher] Health check FAILED
curl -s -m 5 http://localhost:3004/healthz && echo. || echo [launcher] TiRouter 3004 unreachable
pause