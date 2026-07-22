$ErrorActionPreference = 'Stop'
$Root = Split-Path -Parent $PSScriptRoot
$UserEnv = Join-Path $env:USERPROFILE 'Env'
$GoRoot = if ($env:GOROOT) { $env:GOROOT } else { Join-Path $UserEnv 'GOROOT' }
$Go = Join-Path $GoRoot 'bin\go.exe'
$JavaHome = if ($env:JAVA_HOME) { $env:JAVA_HOME } else {
    Get-ChildItem (Join-Path $UserEnv 'ANDROID\jdk') -Directory -ErrorAction SilentlyContinue |
        Sort-Object Name -Descending | Select-Object -First 1 -ExpandProperty FullName
}
$AndroidHome = if ($env:ANDROID_HOME) { $env:ANDROID_HOME } elseif ($env:ANDROID_SDK_ROOT) {
    $env:ANDROID_SDK_ROOT
} else { Join-Path $UserEnv 'ANDROID\sdk' }
$Gradle = Get-Command gradle.bat -ErrorAction SilentlyContinue | Select-Object -First 1 -ExpandProperty Source
if (-not $Gradle) {
    $Gradle = Get-ChildItem (Join-Path $UserEnv 'ANDROID\gradle-home\wrapper\dists\gradle-8.9-bin\*\gradle-8.9\bin\gradle.bat') `
        -ErrorAction SilentlyContinue | Select-Object -First 1 -ExpandProperty FullName
}

if (-not (Test-Path $Go)) { throw "Go toolchain not found: $Go" }
if (-not $JavaHome -or -not (Test-Path $JavaHome)) { throw 'JDK 17 was not found; set JAVA_HOME.' }
if (-not (Test-Path $AndroidHome)) { throw 'Android SDK was not found; set ANDROID_HOME.' }
if (-not $Gradle -or -not (Test-Path $Gradle)) { throw 'Gradle 8.9 was not found; add it to PATH.' }

function Invoke-Native {
    param(
        [Parameter(Mandatory)] [scriptblock] $Command,
        [Parameter(Mandatory)] [string] $FailureMessage
    )
    & $Command
    if ($LASTEXITCODE -ne 0) {
        throw "$FailureMessage (exit code $LASTEXITCODE)."
    }
}

Push-Location $Root
try {
    Invoke-Native { & $Go test -tags ble ./... } 'Go tests failed'
    Invoke-Native { & $Go vet -unsafeptr=false -tags ble ./... } 'Go vet failed'

    $CgoEnabled = (& $Go env CGO_ENABLED).Trim()
    if ($LASTEXITCODE -ne 0) { throw 'Unable to query Go cgo support.' }
    if ($CgoEnabled -eq '1') {
        Invoke-Native { & $Go test -race ./internal/authorize ./internal/config ./internal/coordinator ./internal/protocol ./internal/proximity } 'Go race tests failed'
    } else {
        Write-Warning 'Skipping Go race detector because this Go environment has CGO_ENABLED=0.'
    }

    $env:JAVA_HOME = $JavaHome
    $env:ANDROID_HOME = $AndroidHome
    $env:ANDROID_SDK_ROOT = $AndroidHome
    Push-Location android
    try {
		Invoke-Native { & $Gradle --no-daemon :app:testDebugUnitTest :app:assembleDebug } 'Android tests or APK build failed'
    } finally {
        Pop-Location
    }
} finally {
    Pop-Location
}
