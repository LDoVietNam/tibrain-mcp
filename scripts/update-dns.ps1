$token = $env:CF_API_TOKEN
$zoneId = $env:CF_ZONE_ID
$recordId = $env:CF_RECORD_ID
$newTunnelId = $env:TUNNEL_ID

$body = @{
    type = "CNAME"
    name = "mcp.trepremium.online"
    content = "$newTunnelId.cfargotunnel.com"
    ttl = 1
    proxied = $true
} | ConvertTo-Json

$headers = @{
    "Authorization" = "Bearer $token"
    "Content-Type" = "application/json"
}

Invoke-RestMethod -Uri "https://api.cloudflare.com/client/v4/zones/$zoneId/dns_records/$recordId" `
  -Method PUT `
  -Headers $headers `
  -Body $body