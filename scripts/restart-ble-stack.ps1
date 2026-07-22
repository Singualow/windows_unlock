[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'

if (-not ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole(
    [Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'This script must run as administrator.'
}

Restart-Service -Name lfsvc -Force
Restart-Service -Name bthserv -Force
Start-Sleep -Seconds 2

Get-Service -Name lfsvc, bthserv, 'BluetoothUserService*' |
    Select-Object Status, Name, StartType |
    Format-Table -AutoSize
