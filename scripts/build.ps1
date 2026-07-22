param(
    [switch]$SkipAndroid
)

$ErrorActionPreference = 'Stop'
$Root = Split-Path -Parent $PSScriptRoot
$Bin = Join-Path $Root 'bin'
$Dist = Join-Path $Root 'dist'
$UserEnv = Join-Path $env:USERPROFILE 'Env'
$GoRoot = if ($env:GOROOT) { $env:GOROOT } else { Join-Path $UserEnv 'GOROOT' }
$Go = Join-Path $GoRoot 'bin\go.exe'
$Gofmt = Join-Path $GoRoot 'bin\gofmt.exe'
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
$VsWhere = Join-Path ${env:ProgramFiles(x86)} 'Microsoft Visual Studio\Installer\vswhere.exe'

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

if (-not (Test-Path $Go)) { throw "Go toolchain not found: $Go" }
if (-not (Test-Path $VsWhere)) { throw 'Visual Studio Build Tools are required.' }
if (-not $SkipAndroid) {
    if (-not $JavaHome -or -not (Test-Path $JavaHome)) { throw 'JDK 17 was not found; set JAVA_HOME.' }
    if (-not (Test-Path $AndroidHome)) { throw 'Android SDK was not found; set ANDROID_HOME.' }
    if (-not $Gradle -or -not (Test-Path $Gradle)) { throw 'Gradle 8.9 was not found; add it to PATH.' }
}

New-Item -ItemType Directory -Force -Path $Bin, $Dist | Out-Null

Push-Location $Root
try {
    $GoFiles = Get-ChildItem cmd, internal -Recurse -Filter '*.go' | Select-Object -ExpandProperty FullName
    Invoke-Native { & $Gofmt -w @GoFiles } 'gofmt failed'
    Invoke-Native { & $Go test -tags ble ./... } 'Go tests failed'
    Invoke-Native { & $Go vet -unsafeptr=false -tags ble ./... } 'Go vet failed'

    Invoke-Native { & $Go build -trimpath -tags ble -ldflags '-s -w' -o (Join-Path $Bin 'ProximityUnlockSvc.exe') ./cmd/proximity-service } 'Service build failed'
    Invoke-Native { & $Go build -trimpath -tags ble -ldflags '-s -w -H=windowsgui' -o (Join-Path $Bin 'ProximityUnlockAgent.exe') ./cmd/proximity-agent } 'Agent build failed'
    Invoke-Native { & $Go build -trimpath -tags ble -ldflags '-s -w' -o (Join-Path $Bin 'proximityctl.exe') ./cmd/proximityctl } 'CLI build failed'
    Invoke-Native { & $Go build -trimpath -tags ble -ldflags '-s -w -H=windowsgui' -o (Join-Path $Bin 'setup.exe') ./cmd/setup } 'Setup build failed'
	Invoke-Native { & $Go build -trimpath -tags ble -ldflags '-s -w -H=windowsgui' -o (Join-Path $Bin 'uninstall.exe') ./cmd/setup } 'Uninstaller build failed'

    $CMake = & $VsWhere -latest -products * -find 'Common7\IDE\CommonExtensions\Microsoft\CMake\CMake\bin\cmake.exe' | Select-Object -First 1
    if (-not $CMake) { throw 'CMake component not found in Visual Studio Build Tools.' }
    Invoke-Native { & $CMake -S native\credential-provider -B native\credential-provider\out -A x64 } 'Credential Provider configuration failed'
    Invoke-Native { & $CMake --build native\credential-provider\out --config Release } 'Credential Provider build failed'
    Copy-Item -Force native\credential-provider\out\Release\ProximityUnlockCredentialProvider.dll $Bin

	$InstallerPayload = Join-Path $Root 'cmd\installer\payload'
	New-Item -ItemType Directory -Force -Path $InstallerPayload | Out-Null
	$InstallerFiles = @(
		'ProximityUnlockSvc.exe',
		'ProximityUnlockAgent.exe',
		'proximityctl.exe',
		'ProximityUnlockCredentialProvider.dll',
		'setup.exe',
		'uninstall.exe'
	)
	foreach ($name in $InstallerFiles) {
		Copy-Item -Force (Join-Path $Bin $name) (Join-Path $InstallerPayload $name)
	}
	Invoke-Native { & $Go build -trimpath -tags ble -ldflags '-s -w -H=windowsgui' -o (Join-Path $Bin 'ProximityUnlockInstaller.exe') ./cmd/installer } 'Single-file installer build failed'
	Copy-Item -Force (Join-Path $Bin 'ProximityUnlockInstaller.exe') (Join-Path $Dist 'ProximityUnlockInstaller.exe')

    if (-not $SkipAndroid) {
        $env:JAVA_HOME = $JavaHome
        $env:ANDROID_HOME = $AndroidHome
        $env:ANDROID_SDK_ROOT = $AndroidHome
        Push-Location android
        try {
			Invoke-Native { & $Gradle --no-daemon :app:testDebugUnitTest :app:assembleDebug } 'Android tests or APK build failed'
        } finally {
            Pop-Location
        }
        Copy-Item -Force android\app\build\outputs\apk\debug\app-debug.apk (Join-Path $Bin 'ProximityUnlock-android-debug.apk')
    }

    Copy-Item -Force scripts\recover.ps1 $Bin
    Copy-Item -Force scripts\uninstall.ps1 $Bin
    Copy-Item -Force scripts\enable-unlock.ps1 $Bin

    $artifacts = Get-ChildItem $Bin -File | Where-Object Name -ne 'SHA256SUMS.txt' | Sort-Object Name
    $lines = foreach ($artifact in $artifacts) {
        $hash = (Get-FileHash -Algorithm SHA256 $artifact.FullName).Hash.ToLowerInvariant()
        "$hash  $($artifact.Name)"
    }
    Set-Content -Encoding ascii -Path (Join-Path $Bin 'SHA256SUMS.txt') -Value $lines
    Write-Host "Build completed: $Bin"
} finally {
    Pop-Location
}
