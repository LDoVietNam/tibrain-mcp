@echo off
REM TiBrain Service Stopper
echo [TiBrain] Stopping TiBrain service...

REM Kill any process listening on port 3005
for /f "tokens=5" %%a in ('netstat -ano ^| findstr ":3005 "') do (
    echo [TiBrain] Killing process PID %%a on port 3005
    taskkill /F /PID %%a 2>nul
)

REM Also check for tibrain.exe
tasklist /FI "IMAGENAME eq tibrain.exe" 2>NUL | find /I /N "tibrain.exe">NUL
if "%ERRORLEVEL%"=="0" (
    echo [TiBrain] Stopping tibrain.exe
    taskkill /F /IM tibrain.exe 2>nul
)

echo [TiBrain] Service stopped
endlocal
