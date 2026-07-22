# status-tibrain-tunnel.ps1
# Report status of the managed tunnel, local TiBrain health, public health and DNS.

$RepoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$PidFile = Join-Path $RepoRoot "deploy\cloudflare\tunnel.pid"
$BaseURL = "https://mcp.trepremium.online"

function Line($label, $val) { Write-Host ("{0,-18}: {1}" -f $label, $val) }

# Process
if (Test-Path $PidFile) {
    $pid = (Get-Content $PidFile -Raw).Trim()
    $proc = Get-Process -Id $pid -ErrorAction SilentlyContinue
    if ($proc -and $proc.Name -eq "cloudflared") {
        Line "tunnel process" "RUNNING (pid $pid)"
    } else {
        Line "tunnel process" "pid $pid NOT running (stale pid file)"
    }
} else {
    Line "tunnel process" "no managed pid file"
}

# Local health
try {
    $h = Invoke-RestMethod -Uri "http://127.0.0.1:1810/health" -TimeoutSec 3
    Line "local /health" "OK"
} catch {
    Line "local /health" "UNREACHABLE"
}

# Public health (requires tunnel + DNS)
try {
    $ph = Invoke-RestMethod -Uri "$BaseURL/health" -TimeoutSec 5 -SkipHttpErrorCheck
    Line "public /health" "OK"
} catch {
    Line "public /health" "UNREACHABLE (tunnel/DNS not ready: $_)"
}

# DNS resolution
try {
    $ips = Resolve-DnsName -Name $BaseURL.Replace("https://","") -ErrorAction Stop | Select-Object -ExpandProperty IPAddress
    Line "DNS" ($ips -join ", ")
} catch {
    Line "DNS" "not resolved"
}
