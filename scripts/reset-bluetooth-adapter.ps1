[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'

if (-not ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole(
    [Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'This script must run as administrator.'
}

$adapters = @(Get-PnpDevice -Class Bluetooth -PresentOnly |
    Where-Object { $_.InstanceId -like 'USB\*' -and $_.FriendlyName -match 'Adapter' })
if ($adapters.Count -ne 1) {
    throw "Expected exactly one physical Bluetooth adapter, found $($adapters.Count)."
}

$adapter = $adapters[0]
Write-Output "Resetting $($adapter.FriendlyName) [$($adapter.InstanceId)]"
Disable-PnpDevice -InstanceId $adapter.InstanceId -Confirm:$false
Start-Sleep -Seconds 2
Enable-PnpDevice -InstanceId $adapter.InstanceId -Confirm:$false
Start-Sleep -Seconds 4

Get-PnpDevice -InstanceId $adapter.InstanceId |
    Select-Object Status, FriendlyName, InstanceId |
    Format-List
