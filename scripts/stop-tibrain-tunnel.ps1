# stop-tibrain-tunnel.ps1
# Stop ONLY the tunnel process this script started (recorded in tunnel.pid).
# It intentionally does NOT kill every cloudflared process on the machine.

$ErrorActionPreference = "Stop"
$RepoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$PidFile = Join-Path $RepoRoot "deploy\cloudflare\tunnel.pid"

if (-not (Test-Path $PidFile)) {
    Write-Host "No managed tunnel PID recorded. Nothing to stop."
    exit 0
}
$pid = (Get-Content $PidFile -Raw).Trim()
$proc = Get-Process -Id $pid -ErrorAction SilentlyContinue
if (-not $proc) {
    Write-Host "Recorded tunnel pid $pid no longer running. Cleaning up pid file."
    Remove-Item $PidFile -Force
    exit 0
}
if ($proc.Name -ne "cloudflared") {
    Write-Warning "PID $pid is not cloudflared (it is $($proc.Name)). Refusing to kill it."
    exit 1
}
$proc | Stop-Process -Force
Remove-Item $PidFile -Force
Write-Host "Stopped managed tunnel (pid $pid)."
