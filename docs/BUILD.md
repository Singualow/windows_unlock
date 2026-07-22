# Build and safe installation

## Toolchain

- Windows 11 x64
- Go `1.26.4` (`GOROOT`, `PATH`, or `%USERPROFILE%\Env\GOROOT`)
- Android SDK 35, JDK 17, and Gradle 8.9 (`ANDROID_HOME`, `JAVA_HOME`,
  `PATH`, or the matching directories under `%USERPROFILE%\Env\ANDROID`)
- Visual Studio 2022 Build Tools with the C++ desktop workload and Windows SDK

From an ordinary PowerShell prompt:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\build.ps1
```

The build runs Go tests/vet, compiles the real WinRT BLE tag, builds the x64
Credential Provider with `/W4 /WX`, builds the Android debug APK, packages all
Windows payloads into the single GUI `bin\ProximityUnlockInstaller.exe`, and
writes SHA-256 hashes under `bin`. Then create the stable personal-signed APK:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\build-personal-apk.ps1
```

## End-user installation

The end user does not run PowerShell, build scripts, `proximityctl`, or setup
commands.

1. Keep Windows PIN/password/Hello enabled as a recovery method.
2. Double-click `bin\ProximityUnlockInstaller.exe`, approve UAC, and complete
   the Windows secure credential dialog with the Microsoft account **password**
   (not the PIN). The installer starts the tray automatically.
3. Sideload `bin\ProximityUnlock-android-personal.apk` on the Android 15 phone
   and grant Nearby devices and notification permissions.
4. From the tray choose **配对手机…**, scan the two-minute QR code, and wait for
   the tray status to show the phone signal.
5. Keep the phone unlocked in strict mode, then check **启用 Windows 锁屏自动解锁**
   in the tray and approve UAC. Registration is refused unless the real BLE
   service has a fresh authenticated phone proof.
6. Optionally check **锁屏后立即解锁（降低安全性）** to skip the default ten-second
   away-and-return rule after `Win+L`. RSSI and a fresh signature are still
   required.

For the stable, personal-signed APK used for updates, run
`scripts\build-personal-apk.ps1` and install
`bin\ProximityUnlock-android-personal.apk`. Its P-256 signing key is under the
Git-ignored `.local` directory; the password is DPAPI-protected to the current
Windows user. Back up that directory together with the Windows profile if you
need to preserve Android update continuity. The debug APK is only for initial
development tests.

After changing the Microsoft account password, choose **更新 Windows 密码…**
from the tray and approve the system dialog.

## Recovery

The provider never filters built-in Windows providers. If Bluetooth or the
service fails, select PIN/password/Hello normally. Uncheck **启用 Windows 锁屏自动解锁**
in the tray to unregister only the custom provider. Choose **卸载软件…** to
remove LSA secrets, the CNG identity key, pairing data, service, startup agent,
and installed files.
