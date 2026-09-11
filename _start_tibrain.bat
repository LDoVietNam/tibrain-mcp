@echo off
REM Start TiBrain on :3005 from its own directory (no rebuild).
cd /d "%~dp0"

REM POCKETMCP env: load từ router.env ở dưới — KHÔNG hardcode key trong script

REM Load secret store (TIBRAIN_MCP_BEARER_TOKEN etc.) without echoing values
for /f "usebackq tokens=1,* delims==" %%A in (`findstr /v /b "#" "Z:\00_SECRET\router.env" 2^>nul`) do (
    set "%%A=%%B"
)

echo [TiBrain] launching from %CD%
start "" /b tibrain.exe --port 3005 --host 0.0.0.0
timeout /t 6 /nobreak >nul
netstat -ano | findstr ":3005" | findstr LISTENING >nul && (
    echo [TiBrain] LISTENING on :3005
) || (
    echo [TiBrain] NOT LISTENING - check tibrain_err.log
)
