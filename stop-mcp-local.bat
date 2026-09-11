@echo off
REM =============================================================================
REM Local MCP Server Stop Script
REM =============================================================================
REM Purpose: Dung MCP wrapper server
REM =============================================================================

cd /d "%~dp0"
cd ".claude"

REM Find and kill node process running mcp-wrapper
for /f "tokens=5" %%a in ('netstat -ano ^| findstr ":3002" ^| findstr LISTENING') do (
    echo [MCP-Local] Dang dung process %%a chay MCP server...
    taskkill /F /PID %%a >nul 2>&1
)

timeout /t 1 /nobreak >nul

REM Verify stopped
netstat -ano | findstr ":3002" | findstr LISTENING >nul 2>&1
if %errorlevel% neq 0 (
    echo [MCP-Local] MCP server da dung
) else (
    echo [MCP-Local] CAN NOT stop MCP server
    echo [MCP-Local] Maybe process already stopped
)

pause