try {
    $response = Invoke-WebRequest -Uri "http://localhost:3005/api/health" -Method GET -TimeoutSec 5 -UseBasicParsing
    Write-Host "Health check: $($response.StatusCode) - $($response.Content)"
} catch {
    Write-Host "Health check FAILED: $($_.Exception.Message)"
}