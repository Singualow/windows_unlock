$ErrorActionPreference = 'Stop'
$Root = Split-Path -Parent $PSScriptRoot
$Ready = Join-Path $Root '.local\ble-diagnostic.ready'
$Done = Join-Path $Root '.local\ble-diagnostic.done'
Remove-Item $Ready, $Done -Force -ErrorAction SilentlyContinue

try {
    Stop-Service ProximityUnlockSvc -Force
    Set-Content -Encoding ascii -Path $Ready -Value 'ready'
    $deadline = (Get-Date).AddSeconds(60)
    while (-not (Test-Path $Done)) {
        if ((Get-Date) -gt $deadline) { throw 'Timed out waiting for the BLE probe.' }
        Start-Sleep -Milliseconds 250
    }
} finally {
    Start-Service ProximityUnlockSvc
    Remove-Item $Ready, $Done -Force -ErrorAction SilentlyContinue
}
