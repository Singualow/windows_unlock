$ErrorActionPreference = 'Stop'
$Root = Split-Path -Parent $PSScriptRoot
$Bin = Join-Path $Root 'bin'
$Install = Join-Path $env:ProgramFiles 'ProximityUnlock'
$Names = @(
    'ProximityUnlockSvc.exe',
    'ProximityUnlock.exe',
    'proximityctl.exe',
    'ProximityUnlockCredentialProvider.dll',
    'setup.exe',
    'uninstall.exe'
)

$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = [Security.Principal.WindowsPrincipal]::new($identity)
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'This update must run elevated.'
}

foreach ($name in $Names) {
    if (-not (Test-Path (Join-Path $Bin $name))) { throw "Missing build artifact: $name" }
}

$backups = @()
try {
    Stop-Service ProximityUnlockSvc -Force
    Get-Process ProximityUnlockAgent -ErrorAction SilentlyContinue | Stop-Process -Force
    Get-Process ProximityUnlock -ErrorAction SilentlyContinue | Stop-Process -Force
    foreach ($name in $Names) {
        $destination = Join-Path $Install $name
        $backup = "$destination.update-backup"
        if (Test-Path $destination) {
            Copy-Item -Force $destination $backup
            $backups += $backup
        }
        Copy-Item -Force (Join-Path $Bin $name) $destination
        if ((Get-FileHash $destination).Hash -ne (Get-FileHash (Join-Path $Bin $name)).Hash) {
            throw "Hash verification failed after updating $name"
        }
    }
} catch {
    foreach ($name in $Names) {
        $destination = Join-Path $Install $name
        $backup = "$destination.update-backup"
        if (Test-Path $backup) { Copy-Item -Force $backup $destination }
    }
    throw
} finally {
    Start-Service ProximityUnlockSvc
}

$config = Get-Content -Raw (Join-Path $env:ProgramData 'ProximityUnlock\config.json') | ConvertFrom-Json
$runKey = "Registry::HKEY_USERS\$($config.target_sid)\Software\Microsoft\Windows\CurrentVersion\Run"
New-Item -Path $runKey -Force | Out-Null
Remove-ItemProperty -Path $runKey -Name 'ProximityUnlockAgent' -ErrorAction SilentlyContinue
Set-ItemProperty -Path $runKey -Name 'ProximityUnlock' -Value ('"' + (Join-Path $Install 'ProximityUnlock.exe') + '" --background')
Remove-Item -Force (Join-Path $Install 'ProximityUnlockAgent.exe') -ErrorAction SilentlyContinue

foreach ($backup in $backups) { Remove-Item -Force $backup -ErrorAction SilentlyContinue }
Write-Host 'Installed Windows service, Tauri tray, management UI, Credential Provider, and uninstaller updated successfully.'
