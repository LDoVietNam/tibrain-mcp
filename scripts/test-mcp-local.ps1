# test-mcp-local.ps1
# Smoke-test the local TiBrain MCP gateway over Streamable HTTP.
# Requires TIBRAIN_MCP_BEARER_TOKEN to be set in the environment.

$ErrorActionPreference = "Stop"
$Base = "http://127.0.0.1:1810/mcp"
$Token = $env:TIBRAIN_MCP_BEARER_TOKEN
if (-not $Token) { Write-Error "Set TIBRAIN_MCP_BEARER_TOKEN first."; exit 1 }

$headers = @{
    "Content-Type"  = "application/json"
    "Accept"        = "application/json, text/event-stream"
    "Authorization" = "Bearer $Token"
}

function Post-JSONRPC($method, $params, $id) {
    $body = @{ jsonrpc = "2.0"; method = $method; params = $params; id = $id } | ConvertTo-Json -Compress
    $r = Invoke-RestMethod -Uri $Base -Method Post -Headers $headers -Body $body -TimeoutSec 10
    return $r
}

# 1. initialize
$r = Post-JSONRPC "initialize" @{
    protocolVersion = "2024-11-05"
    capabilities = @{}
    clientInfo = @{ name = "tibrain-smoke-test"; version = "1.0" }
} 1
if (-not $r.result) { Write-Error "initialize failed: $($r | ConvertTo-Json)"; exit 1 }
Write-Host "initialize OK -> server: $($r.result.serverInfo.name) $($r.result.serverInfo.version)"

# 2. initialized notification
$initNote = @{ jsonrpc = "2.0"; method = "notifications/initialized" } | ConvertTo-Json -Compress
Invoke-RestMethod -Uri $Base -Method Post -Headers $headers -Body $initNote -TimeoutSec 10 | Out-Null

# 3. tools/list
$r = Post-JSONRPC "tools/list" @{} 2
if (-not $r.result) { Write-Error "tools/list failed: $($r | ConvertTo-Json)"; exit 1 }
$tools = $r.result.tools
Write-Host "tools/list OK -> $($tools.Count) tools"
$tools | ForEach-Object { Write-Host ("  - {0}" -f $_.name) }

# 4. ping
$r = Post-JSONRPC "ping" @{} 3
if ($r.result) { Write-Host "ping OK" } else { Write-Error "ping failed"; exit 1 }

Write-Host "`nALL LOCAL MCP SMOKE TESTS PASSED"
