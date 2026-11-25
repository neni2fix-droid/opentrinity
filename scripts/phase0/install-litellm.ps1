$ErrorActionPreference = 'Stop'
$py  = 'C:\Users\ai9\AppData\Local\Programs\Python\Python311\python.exe'
$venv = 'C:\Users\ai9\Documents\opentrinity\.phase0\venv'

Write-Output "=== creating venv with $py ==="
& $py -m venv $venv
if (-not (Test-Path "$venv\Scripts\python.exe")) { throw "venv python missing" }

$vpy = "$venv\Scripts\python.exe"
Write-Output "=== venv python ==="
& $vpy -c "import sys; print(sys.version); print(sys.executable)"

Write-Output "=== upgrading pip/setuptools/wheel ==="
& $vpy -m pip install --upgrade pip setuptools wheel 2>&1 | Select-Object -Last 5

Write-Output "=== installing litellm[proxy] ==="
& $vpy -m pip install "litellm[proxy]" 2>&1 | Select-Object -Last 25

Write-Output "=== resolved versions ==="
& $vpy -c "import litellm; print('LITELLM_VERSION=' + litellm.__version__)"
& $vpy -m pip show litellm 2>&1 | Select-String -Pattern '^(Name|Version|License)'
Write-Output "=== DONE ==="
