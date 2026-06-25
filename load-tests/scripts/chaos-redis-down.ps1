# Chaos test: verify fail-closed behavior when Redis is unavailable.
# Prerequisites: docker compose stack running with SENTINEL_USE_REDIS=true.
#
# Usage: ./load-tests/scripts/chaos-redis-down.ps1

$ErrorActionPreference = "Stop"
$base = "http://localhost"

Write-Host "=== Sentinel Chaos: Redis failure (fail-closed) ==="

# Baseline: should work
$before = curl.exe -s -o NUL -w "%{http_code}" "$base/api/test"
Write-Host "Before Redis stop: GET /api/test => $before"
if ($before -ne "200") {
    Write-Host "WARN: expected 200 before chaos; got $before (may already be rate-limited)"
}

Write-Host "Stopping Redis..."
docker stop sentinel-redis-1 | Out-Null
Start-Sleep -Seconds 3

$during = curl.exe -s -o NUL -w "%{http_code}" "$base/api/test"
Write-Host "During Redis down: GET /api/test => $during"

$health = curl.exe -s "$base/health"
Write-Host "Health response: $health"

Write-Host "Starting Redis..."
docker start sentinel-redis-1 | Out-Null
Start-Sleep -Seconds 3

$after = curl.exe -s -o NUL -w "%{http_code}" "$base/api/test"
Write-Host "After Redis recovery: GET /api/test => $after"

if ($during -eq "503") {
    Write-Host "PASS: fail-closed returned 503 when Redis unavailable"
    exit 0
} else {
    Write-Host "FAIL: expected 503 during Redis outage, got $during"
    exit 1
}
