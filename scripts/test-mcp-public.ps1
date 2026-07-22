# test-mcp-public.ps1
# Smoke-test the PUBLIC TiBrain MCP gateway (through Cloudflare Tunnel).
# Requires TIBRAIN_MCP_BEARER_TOKEN and assumes the tunnel + DNS are live.
# If the tunnel/DNS is not available, this reports BLOCKED and does NOT claim success.

$ErrorActionPreference = "Stop"
$Base = "https://mcp.trepremium.online/mcp"
$Token = $env:TIBRAIN_MCP_BEARER_TOKEN
if (-not $Token) { Write-Error "Set TIBRAIN_MCP_BEARER_TOKEN first."; exit 1 }

try {
    $h = Invoke-RestMethod -Uri "https://mcp.trepremium.online/health" -TimeoutSec 5
    Write-Host "public /health OK"
} catch {
    Write-Warning "BLOCKED - MANUAL VERIFICATION REQUIRED: public endpoint not reachable ($_)"
    Write-Warning "Ensure the tunnel is started (scripts/start-tibrain-tunnel.ps1) and DNS is routed."
    exit 2
}

$headers = @{
    "Content-Type"  = "application/json"
    "Accept"        = "application/json, text/event-stream"
    "Authorization" = "Bearer $Token"
}

function Post-JSONRPC($method, $params, $id) {
    $body = @{ jsonrpc = "2.0"; method = $method; params = $params; id = $id } | ConvertTo-Json -Compress
    return Invoke-RestMethod -Uri $Base -Method Post -Headers $headers -Body $body -TimeoutSec 10
}

$r = Post-JSONRPC "initialize" @{
    protocolVersion = "2024-11-05"
    capabilities = @{}
    clientInfo = @{ name = "tibrain-public-smoke"; version = "1.0" }
} 1
if (-not $r.result) { Write-Error "public initialize failed: $($r | ConvertTo-Json)"; exit 1 }
Write-Host "public initialize OK -> $($r.result.serverInfo.name)"

$r = Post-JSONRPC "tools/list" @{} 2
Write-Host "public tools/list OK -> $($r.result.tools.Count) tools"

Write-Host "`nPUBLIC MCP SMOKE TESTS PASSED"
