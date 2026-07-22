$ErrorActionPreference = 'Stop'
$Setup = Join-Path $env:ProgramFiles 'ProximityUnlock\setup.exe'
if (-not (Test-Path $Setup)) { throw 'Stage 1 is not installed.' }
& $Setup enable-credential-provider
if ($LASTEXITCODE -ne 0) { throw 'Credential Provider registration was refused.' }
Write-Host 'Credential Provider registered. Keep Windows PIN/password/Hello configured as recovery methods.'
