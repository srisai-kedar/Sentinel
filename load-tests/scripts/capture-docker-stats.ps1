#!/usr/bin/env pwsh
# Capture Docker container CPU/memory during benchmarks.
# Usage: ./load-tests/scripts/capture-docker-stats.ps1 -DurationSec 90 -OutputFile load-tests/results/docker-stats.txt

param(
    [int]$DurationSec = 90,
    [string]$OutputFile = "load-tests/results/docker-stats.txt"
)

$containers = @(
    "sentinel-gateway-1-1",
    "sentinel-gateway-2-1",
    "sentinel-gateway-3-1",
    "sentinel-redis-1",
    "sentinel-nginx-1"
)

$dir = Split-Path $OutputFile -Parent
if ($dir -and -not (Test-Path $dir)) { New-Item -ItemType Directory -Path $dir -Force | Out-Null }

"Docker stats capture started $(Get-Date -Format o)" | Out-File $OutputFile
"Duration: ${DurationSec}s" | Out-File $OutputFile -Append
"" | Out-File $OutputFile -Append

$end = (Get-Date).AddSeconds($DurationSec)
while ((Get-Date) -lt $end) {
    "=== $(Get-Date -Format 'HH:mm:ss') ===" | Out-File $OutputFile -Append
    foreach ($c in $containers) {
        $line = docker stats $c --no-stream --format "{{.Name}} CPU={{.CPUPerc}} MEM={{.MemUsage}} MEM%={{.MemPerc}}" 2>$null
        if ($line) { $line | Out-File $OutputFile -Append }
    }
    "" | Out-File $OutputFile -Append
    Start-Sleep -Seconds 5
}

"Capture complete $(Get-Date -Format o)" | Out-File $OutputFile -Append
Write-Host "Saved to $OutputFile"
