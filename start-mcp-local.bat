@echo off
REM =============================================================================
REM Local MCP Server Start Script
REM =============================================================================
REM Purpose: Khoi dong MCP wrapper server cho local shell tools
REM Tools: health-check, go-build, go-test, go-lint, node-test
REM =============================================================================

cd /d "%~dp0"

REM Kiem tra Node.js
where node >nul 2>&1
if errorlevel 1 (
    echo [MCP-Local] LOI: Node.js not found!
    echo [MCP-Local] Vui long cai dat Node.js truoc
    pause
    exit /b 1
)

REM Kiem tra port 3002
netstat -ano | findstr ":3002" | findstr LISTENING >nul 2>&1
if %errorlevel% equ 0 (
    echo [MCP-Local] MCP server da chay tren port 3002
    echo [MCP-Local] Su dung 'stop-mcp-local.bat' de dung
    pause
    exit /b 0
)

REM Chuyen vao .claude directory
cd "%~dp0.claude"

REM Khoi dong MCP wrapper
echo [MCP-Local] Dang khoi dong MCP wrapper server...
start /b "" cmd /c "node scripts/mcp-wrapper.js"

REM Wait va kiem tra
timeout /t 2 /nobreak >nul

REM Test connection
curl -s -m 3 http://localhost:3002/health >nul 2>&1
if errorlevel 1 (
    echo [MCP-Local] LOI: Khong the khoi dong MCP server
    echo [MCP-Local] Kiem tra: node scripts/mcp-wrapper.js
) else (
    echo [MCP-Local] MCP server dang chay tai http://localhost:3002
)

pause
