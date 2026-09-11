# Chạy TiBrain để debug kết nối PocketMCP (filter log, không rebuild)
# Secret: load từ Z:\00_SECRET\router.env — KHÔNG hardcode trong file này
if (Test-Path "Z:\00_SECRET\router.env") {
    Get-Content "Z:\00_SECRET\router.env" | Where-Object { $_ -match '^(POCKETMCP_API_KEY|POCKETMCP_HOST)=' } | ForEach-Object {
        $parts = $_ -split '=', 2
        Set-Item -Path ("Env:" + $parts[0]) -Value $parts[1]
    }
    Write-Host "[run_tibrain] Loaded POCKETMCP_* from router.env"
} else {
    Write-Warning "[run_tibrain] Z:\00_SECRET\router.env not found - POCKETMCP env vars not set"
}

& Z:/01_PROJECTS/apps/products/tibrain/tibrain.exe 2>&1 | Select-String 'pocketmcp|header|connect|Failed to connect|authorization'
