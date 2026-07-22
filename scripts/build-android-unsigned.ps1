$ErrorActionPreference = 'Stop'

$Root = Split-Path -Parent $PSScriptRoot
$AndroidRoot = Join-Path $Root 'android'
$UserEnv = Join-Path $env:USERPROFILE 'Env'
$JavaHome = if ($env:JAVA_HOME) { $env:JAVA_HOME } else {
    Get-ChildItem (Join-Path $UserEnv 'ANDROID\jdk') -Directory -ErrorAction SilentlyContinue |
        Sort-Object Name -Descending | Select-Object -First 1 -ExpandProperty FullName
}
$AndroidHome = if ($env:ANDROID_HOME) { $env:ANDROID_HOME } elseif ($env:ANDROID_SDK_ROOT) {
    $env:ANDROID_SDK_ROOT
} else {
    Join-Path $UserEnv 'ANDROID\sdk'
}
$Gradle = Join-Path $AndroidRoot 'gradlew.bat'

if (-not $JavaHome -or -not (Test-Path (Join-Path $JavaHome 'bin\java.exe'))) {
    throw 'JDK 17 was not found; set JAVA_HOME.'
}
if (-not (Test-Path $AndroidHome)) { throw 'Android SDK was not found; set ANDROID_HOME.' }
if (-not (Test-Path $Gradle)) { throw 'The repository Gradle wrapper is missing.' }

$env:JAVA_HOME = $JavaHome
$env:ANDROID_HOME = $AndroidHome
$env:ANDROID_SDK_ROOT = $AndroidHome

Push-Location $AndroidRoot
try {
    & $Gradle --no-daemon :app:testReleaseUnitTest :app:lintRelease :app:assembleRelease
    if ($LASTEXITCODE -ne 0) { throw "Android release build failed (exit code $LASTEXITCODE)." }
} finally {
    Pop-Location
}

$MetadataPath = Join-Path $AndroidRoot 'app\build\outputs\apk\release\output-metadata.json'
$Metadata = Get-Content -Raw $MetadataPath | ConvertFrom-Json
if ($Metadata.elements.Count -ne 1) { throw 'Expected exactly one universal release APK.' }
$UnsignedApk = Join-Path (Split-Path $MetadataPath) $Metadata.elements[0].outputFile
if (-not (Test-Path $UnsignedApk)) { throw "Unsigned APK not found: $UnsignedApk" }

Write-Host "Unsigned release APK (do not distribute): $UnsignedApk"
Write-Host 'Use the protected release signer and independently verify the certificate before publishing.'
