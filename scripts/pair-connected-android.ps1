[CmdletBinding()]
param(
    [string] $Adb = '',
    [string] $Control = 'C:\Program Files\ProximityUnlock\proximityctl.exe'
)

$ErrorActionPreference = 'Stop'

if (-not $Adb) {
    $AndroidHome = if ($env:ANDROID_HOME) { $env:ANDROID_HOME } elseif ($env:ANDROID_SDK_ROOT) {
        $env:ANDROID_SDK_ROOT
    } else { Join-Path $env:USERPROFILE 'Env\ANDROID\sdk' }
    $Adb = Join-Path $AndroidHome 'platform-tools\adb.exe'
    if (-not (Test-Path -LiteralPath $Adb)) {
        $Adb = Get-Command adb.exe -ErrorAction SilentlyContinue | Select-Object -First 1 -ExpandProperty Source
    }
}

if (-not $Adb -or -not (Test-Path -LiteralPath $Adb)) { throw 'adb.exe was not found; set ANDROID_HOME or add adb to PATH.' }
if (-not (Test-Path -LiteralPath $Control)) { throw 'proximityctl.exe was not found.' }

$devices = @(& $Adb devices | Select-String '\sdevice$')
if ($LASTEXITCODE -ne 0 -or $devices.Count -ne 1) {
    throw "Expected exactly one authorized Android device, found $($devices.Count)."
}

& $Control revoke | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Failed to revoke the stale PC pairing.' }

# Feed the command its final Enter and capture (but never print) the URI.
$pairOutput = @('') | & $Control pair 2>&1
if ($LASTEXITCODE -ne 0) { throw 'Failed to start PC pairing.' }
$uri = $pairOutput | Where-Object { $_ -is [string] -and $_.StartsWith('proximityunlock://pair?') } | Select-Object -First 1
if (-not $uri) { throw 'The pairing command did not return a URI.' }

$remoteUri = "'" + $uri + "'"
$remoteStart = "am start -W -a android.intent.action.VIEW -d $remoteUri -n com.singu.proximityunlock/.MainActivity"
& $Adb shell $remoteStart | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Failed to deliver the pairing URI to Android.' }

Start-Sleep -Seconds 2
& $Adb shell uiautomator dump /sdcard/proximity-pair.xml | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Failed to inspect the Android pairing screen.' }
[xml] $ui = (& $Adb exec-out cat /sdcard/proximity-pair.xml) -join "`n"
$button = $ui.SelectSingleNode("//node[@text='保存配对并启动']")
if (-not $button) { throw 'The Android pairing button was not found.' }
if ($button.bounds -notmatch '^\[(\d+),(\d+)\]\[(\d+),(\d+)\]$') {
    throw 'The Android pairing button bounds were invalid.'
}
$x = ([int] $Matches[1] + [int] $Matches[3]) / 2
$y = ([int] $Matches[2] + [int] $Matches[4]) / 2
& $Adb shell input tap ([int] $x) ([int] $y) | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Failed to tap the Android pairing button.' }

$deadline = (Get-Date).AddSeconds(45)
do {
    Start-Sleep -Seconds 1
    $statusText = & $Control status
    $status = $statusText | ConvertFrom-Json
    if ($status.paired) {
        Write-Output 'PAIRING_SUCCEEDED'
        Write-Output $statusText
        exit 0
    }
} while ((Get-Date) -lt $deadline)

throw 'Pairing did not complete within 45 seconds.'
