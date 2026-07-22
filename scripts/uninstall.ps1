$ErrorActionPreference = 'Stop'
$Uninstaller = Join-Path $env:ProgramFiles 'ProximityUnlock\uninstall.exe'
if (-not (Test-Path $Uninstaller)) { throw 'Proximity Unlock is not installed.' }
& $Uninstaller
if ($LASTEXITCODE -ne 0) { throw 'Uninstall reported an error. Run recover.ps1 before rebooting.' }
