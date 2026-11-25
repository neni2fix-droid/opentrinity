$ErrorActionPreference = 'Stop'
$env:OT_USER_DATA = 'C:\Users\ai9\Documents\opentrinity\.demo-userdata'
$exe = 'C:\Users\ai9\Documents\opentrinity\dist\opentrinity.exe'
$line = cmd.exe /c "echo SHOW | `"$exe`" gateway key show 2>nul"
$key = ($line -split ':\s*', 2)[-1].Trim()
if (-not $key) { throw 'gateway key unavailable' }
[Console]::Out.Write($key)
