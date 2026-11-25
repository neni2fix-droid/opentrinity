$ErrorActionPreference = 'Stop'
$env:CGO_ENABLED = '0'
$dir = 'C:\Users\ai9\Documents\opentrinity\.phase0\spikes\sqlite'
Set-Location $dir

Write-Output "=== go env ==="
go version
go env GOOS GOARCH CGO_ENABLED GOMODCACHE GOPROXY

Write-Output "=== go get modernc.org/sqlite ==="
go get modernc.org/sqlite 2>&1
Write-Output "=== go mod tidy ==="
go mod tidy 2>&1

Write-Output "=== go.mod ==="
Get-Content go.mod

Write-Output "=== driver version in build list ==="
go list -m modernc.org/sqlite 2>&1

Write-Output "=== run spike ==="
go run . 2>&1

Write-Output "=== SPIKE EXIT: $LASTEXITCODE ==="
