$ErrorActionPreference = 'Continue'
$Setup = Join-Path $env:ProgramFiles 'ProximityUnlock\setup.exe'
if (Test-Path $Setup) {
    & $Setup recover
} else {
    Remove-Item -Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Authentication\Credential Providers\{C81FCF2E-B9D0-4EAF-8D35-55F750D2561B}' -Recurse -Force -ErrorAction SilentlyContinue
    Remove-Item -Path 'HKLM:\SOFTWARE\Classes\CLSID\{C81FCF2E-B9D0-4EAF-8D35-55F750D2561B}' -Recurse -Force -ErrorAction SilentlyContinue
    Stop-Service -Name ProximityUnlockSvc -Force -ErrorAction SilentlyContinue
}
Write-Host 'Recovery complete. Use the normal Windows PIN/password/Hello tile.'
