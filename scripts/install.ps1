$ErrorActionPreference = 'Stop'
$Bin = Join-Path (Split-Path -Parent $PSScriptRoot) 'bin'
$Config = Join-Path $env:ProgramData 'ProximityUnlock\config.json'

if (-not (Test-Path $Config)) {
    Write-Host 'Initializing PC identity and Windows credential...'
    & (Join-Path $Bin 'proximityctl.exe') initialize
    if ($LASTEXITCODE -ne 0) { throw 'Initialization failed; nothing was installed.' }
}

& (Join-Path $Bin 'proximityctl.exe') self-test
if ($LASTEXITCODE -ne 0) { throw 'Security self-test failed; nothing was installed.' }
& (Join-Path $Bin 'setup.exe') install
if ($LASTEXITCODE -ne 0) { throw 'Stage 1 installation failed.' }

Write-Host 'Stage 1 complete. Install the Android APK, pair the phone, and only then run enable-unlock.ps1.'
