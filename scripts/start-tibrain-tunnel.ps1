# start-tibrain-tunnel.ps1
# Start the Cloudflare Tunnel for TiBrain MCP, idempotently.
# Prerequisites (checked, not created):
#   - cloudflared is on PATH
#   - deploy/cloudflare/config.example.yml copied to config.yml with real tunnel id
#   - TiBrain already listening on 127.0.0.1:1810 (we verify /health)
# The script never edits DNS or deletes tunnel records.

$ErrorActionPreference = "Stop"

$RepoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$CfDir   = Join-Path $RepoRoot "deploy\cloudflare"
$CfgFile = Join-Path $CfDir "config.yml"
$PidFile = Join-Path $CfDir "tunnel.pid"
$LogDir  = Join-Path $RepoRoot ".runtime\logs"
$LogFile = Join-Path $LogDir "cloudflared.log"

function Fail($msg) { Write-Error $msg; exit 1 }

# 1. cloudflared present?
if (-not (Get-Command cloudflared -ErrorAction SilentlyContinue)) {
    Fail "cloudflared not found on PATH. Install it first: https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/downloads/"
}

# 2. config + credentials present?
if (-not (Test-Path $CfgFile)) {
    Fail "Tunnel config not found at $CfgFile. Copy config.example.yml to config.yml and fill the tunnel id / credentials path."
}

# 3. Port 1810 + TiBrain health.
try {
    $h = Invoke-RestMethod -Uri "http://127.0.0.1:1810/health" -TimeoutSec 3
    Write-Host "TiBrain health OK: $($h | ConvertTo-Json -Compress)"
} catch {
    Fail "TiBrain /health not reachable on 127.0.0.1:1810. Start TiBrain before the tunnel."
}

# 4. Idempotency: if a managed tunnel PID is recorded and alive, do nothing.
if (Test-Path $PidFile) {
    $oldPid = (Get-Content $PidFile -Raw).Trim()
    $proc = Get-Process -Id $oldPid -ErrorAction SilentlyContinue
    if ($proc -and $proc.Name -eq "cloudflared") {
        Write-Host "Tunnel already running (pid $oldPid). Nothing to do."
        exit 0
    }
}

# 5. Start tunnel (named tunnel from config) in background, log to file.
New-Item -ItemType Directory -Force -Path $LogDir | Out-Null
$proc = Start-Process -FilePath "cloudflared" -ArgumentList @("tunnel", "--config", $CfgFile, "run") `
    -RedirectStandardOutput $LogFile -RedirectStandardError $LogFile -PassThru -WindowStyle Hidden
$proc.Id | Out-File -FilePath $PidFile -NoNewline
Write-Host "Cloudflare Tunnel started (pid $($proc.Id)). Log: $LogFile"

# 6. Optional: print the DNS route command (does NOT execute it).
Write-Host "If the hostname is not yet routed, run manually:"
Write-Host "  cloudflared tunnel route dns <TUNNEL_NAME> mcp.trepremium.online"
