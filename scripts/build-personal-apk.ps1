$ErrorActionPreference = 'Stop'
$Root = Split-Path -Parent $PSScriptRoot
$Local = Join-Path $Root '.local'
$Bin = Join-Path $Root 'bin'
$Dist = Join-Path $Root 'dist'
$KeyStore = Join-Path $Local 'proximity-unlock-personal.p12'
$ProtectedSecret = Join-Path $Local 'proximity-unlock-personal.dpapi'
$UserEnv = Join-Path $env:USERPROFILE 'Env'
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
$KeyTool = Join-Path $JavaHome 'bin\keytool.exe'

if (-not $JavaHome -or -not (Test-Path $KeyTool)) { throw 'JDK 17 was not found; set JAVA_HOME.' }
if (-not (Test-Path $AndroidHome)) { throw 'Android SDK was not found; set ANDROID_HOME.' }
if (-not $Gradle -or -not (Test-Path $Gradle)) { throw 'Gradle 8.9 was not found; add it to PATH.' }

function Invoke-Native {
    param([scriptblock] $Command, [string] $FailureMessage)
    & $Command
    if ($LASTEXITCODE -ne 0) { throw "$FailureMessage (exit code $LASTEXITCODE)." }
}

New-Item -ItemType Directory -Force -Path $Local, $Bin, $Dist | Out-Null
if (-not (Test-Path $KeyStore)) {
    $random = New-Object byte[] 32
    [System.Security.Cryptography.RandomNumberGenerator]::Fill($random)
    $password = [Convert]::ToBase64String($random)
    [Array]::Clear($random, 0, $random.Length)
    Invoke-Native {
        & $KeyTool -genkeypair -noprompt -keystore $KeyStore -storetype PKCS12 -storepass $password `
            -keypass $password -alias proximityunlock -keyalg EC -groupname secp256r1 -sigalg SHA256withECDSA `
            -validity 3650 -dname 'CN=Proximity Unlock Personal,OU=Personal,O=Proximity Unlock'
    } 'Personal Android signing key creation failed'
    $plain = [Text.Encoding]::UTF8.GetBytes($password)
    $protected = [System.Security.Cryptography.ProtectedData]::Protect(
        $plain, $null, [System.Security.Cryptography.DataProtectionScope]::CurrentUser)
    [Array]::Clear($plain, 0, $plain.Length)
    [IO.File]::WriteAllBytes($ProtectedSecret, $protected)
    [Array]::Clear($protected, 0, $protected.Length)
    $password = $null
}

if (-not (Test-Path $ProtectedSecret)) { throw 'The DPAPI-protected signing password is missing.' }
$protected = [IO.File]::ReadAllBytes($ProtectedSecret)
$plain = [System.Security.Cryptography.ProtectedData]::Unprotect(
    $protected, $null, [System.Security.Cryptography.DataProtectionScope]::CurrentUser)
$password = [Text.Encoding]::UTF8.GetString($plain)
[Array]::Clear($plain, 0, $plain.Length)
[Array]::Clear($protected, 0, $protected.Length)

try {
    $env:JAVA_HOME = $JavaHome
    $env:ANDROID_HOME = $AndroidHome
    $env:ANDROID_SDK_ROOT = $AndroidHome
    $env:PROXIMITY_UNLOCK_KEYSTORE = $KeyStore
    $env:PROXIMITY_UNLOCK_STORE_PASSWORD = $password
    $env:PROXIMITY_UNLOCK_KEY_ALIAS = 'proximityunlock'
    $env:PROXIMITY_UNLOCK_KEY_PASSWORD = $password
    Push-Location (Join-Path $Root 'android')
    try {
        Invoke-Native { & $Gradle --no-daemon :app:testReleaseUnitTest :app:assembleRelease } 'Personal Android APK build failed'
    } finally {
        Pop-Location
    }
    Copy-Item -Force (Join-Path $Root 'android\app\build\outputs\apk\release\app-release.apk') `
        (Join-Path $Bin 'ProximityUnlock-android-personal.apk')
	Copy-Item -Force (Join-Path $Root 'android\app\build\outputs\apk\release\app-release.apk') `
		(Join-Path $Dist 'ProximityUnlock-Android.apk')
} finally {
    $password = $null
    Remove-Item Env:PROXIMITY_UNLOCK_STORE_PASSWORD -ErrorAction SilentlyContinue
    Remove-Item Env:PROXIMITY_UNLOCK_KEY_PASSWORD -ErrorAction SilentlyContinue
}

Write-Host "Personal signed APK: $(Join-Path $Bin 'ProximityUnlock-android-personal.apk')"
