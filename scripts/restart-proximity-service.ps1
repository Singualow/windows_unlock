[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'

if (-not ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole(
    [Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'This script must run as administrator.'
}

Restart-Service -Name ProximityUnlockSvc -Force
(Get-Service -Name ProximityUnlockSvc).WaitForStatus(
    [System.ServiceProcess.ServiceControllerStatus]::Running,
    [TimeSpan]::FromSeconds(15))
Get-Service -Name ProximityUnlockSvc | Select-Object Status, Name, StartType | Format-List
