# Proximity Unlock

Proximity Unlock is a personal, single-PC proof-of-possession unlock system for
Windows 11 and Android 15. The Windows coordinator is written in Go, the phone
companion is a Kotlin BLE peripheral, and the Windows LogonUI bridge is a small
native V2 Credential Provider.

> [!WARNING]
> This project intentionally does **not** replace or filter Windows password,
> PIN, or Windows Hello providers. Do not register the Credential Provider until
> the service self-test and phone pairing both pass. Bluetooth RSSI is not secure
> distance bounding, and relay attacks remain possible.

## Safety boundaries

- Automatic unlock is allowed only for an already logged-in, locked local
  console session. Boot logon, logoff, RDP, UAC, and CredUI are excluded.
- By default, a deliberate `Win+L` while the phone is nearby stays locked until
  the phone has been absent for at least ten seconds and then returns. The tray
  can enable an explicitly lower-security immediate-unlock mode that skips only
  this away-and-return requirement.
- The Microsoft account password is stored only as Windows LSA private data.
  It is never placed in `config.json` or logs.
- The Android app has strict and convenience keys. Strict mode cannot sign
  while Android is locked; convenience mode can.

## Repository layout

- `cmd/proximity-service`: LocalSystem Windows service and coordinator.
- `cmd/proximityctl`: enrollment, status, calibration, and safe installation CLI.
- `cmd/proximity-agent`: per-user auto-lock and notification agent.
- `cmd/installer`: single-file GUI installer that embeds all Windows payloads.
- `internal/`: shared protocol, crypto, state machine, secure storage, and IPC.
- `android/`: Android 15 BLE peripheral companion.
- `native/credential-provider/`: x64 V2 Credential Provider.
- `scripts/`: build, install, recovery, and uninstall scripts.

Built Windows binaries and both Android APK variants are placed in `bin/`.
End users double-click `ProximityUnlockInstaller.exe`; it requests elevation,
uses the Windows secure credential dialog, installs the service, and starts the
tray. Pairing, distance calibration, password updates, immediate-unlock mode,
device revocation, Credential Provider registration, and uninstall are all
available from the tray—no terminal commands are required. Credential Provider
registration remains gated on a fresh authenticated phone proof, and the tray
never disables PIN, password, or Windows Hello providers.

See [docs/SECURITY.md](docs/SECURITY.md) before installing and
[docs/BUILD.md](docs/BUILD.md) for the exact toolchain commands.
