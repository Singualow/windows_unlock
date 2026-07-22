# Security model

## Trust boundary

The Android phone proves possession of a non-exportable P-256 key. The Windows
service validates the proof and creates a five-second, single-use authorization.
The Credential Provider can consume that authorization once; it cannot scan
Bluetooth or manufacture an authorization itself.

The system password is stored as LSA private data under
`L$ProximityUnlock/Credential/<SID>`. Only LocalSystem receives access to the
credential IPC pipe. Public keys, thresholds, and non-secret identifiers live
in `%ProgramData%\ProximityUnlock\config.json`.

## Fail-closed behavior

- Service restart, phone/app absence, invalid HMAC/signature, replay, expired
  challenge, wrong mode, stale password, or Bluetooth failure produces no tile.
  If the service disappears while the desktop is open and auto-lock is enabled,
  the per-user agent locks the console after 20 seconds.
- Three authentication failures in one minute start a five-minute cooldown.
- A password rejection disables automatic unlock until the password is
  re-enrolled after a normal Windows login.
- Boot logon, logoff, RDP, UAC, and CredUI are unsupported.
- By default, `Win+L` while already near is not reversed until the phone leaves
  for ten seconds and returns. The tray's lower-security immediate-unlock mode
  removes that manual-lock hold but still requires the RSSI threshold, three
  recent advertisements, a fresh challenge, and a valid phone signature.
  Resume from sleep gets one fresh challenge window.

## Residual risks

- BLE RSSI is not cryptographic distance bounding. A capable relay can make a
  remote phone appear nearby.
- Convenience mode permits a stolen but locked phone to sign in the background.
- Immediate-unlock mode can reverse an intentional `Win+L` within a few seconds
  whenever the configured phone is already nearby and able to sign.
- A compromised Windows administrator or LocalSystem process can retrieve the
  Microsoft account password despite LSA storage.
- The release APK is signed by a local personal P-256 certificate; the DLL and
  Windows executables are not commercially code-signed. They are intended for
  this one PC and phone only.

Never disable all built-in Credential Providers and never test the first
registration without knowing the current Windows password/PIN.
