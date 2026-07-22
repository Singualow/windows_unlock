# BLE protocol v1

## Advertising

The phone advertises 13 bytes as BLE manufacturer data under the personal-use
company identifier `0xFFFF` (the Windows tinygo WinRT backend does not expose
service-data sections):

```text
version(1) || random_salt(4) || HMAC-SHA256(presence_key,
"ProximityUnlock/ad/v1" || random_salt)[0:8]
```

Pairing advertisements use `version | 0x80`, the first four bytes of the PC ID,
and an eight-byte pairing HMAC. Bluetooth addresses and device names are never
trusted.

## GATT

- Service: `9b7c6a10-5d57-4c2e-8e2a-4ed2f5f7a001`
- Challenge write: `...a002`
- Response notify: `...a003`
- Pairing write/notify: `...a004`

Messages use deterministic big-endian binary bodies. ECDSA signatures use
64-byte IEEE P1363 `r || s`. GATT values are fragmented with a three-byte
`message_type || zero_based_index || fragment_count` header and capped at 180
bytes per fragment.

Each unlock challenge binds protocol version, mode, PC ID, phone ID, target SID
hash, Windows session, issue/expiry times, and a 256-bit nonce. The phone first
verifies the PC signature, then signs the challenge digest, nonce, monotonic
counter, identities, mode, and signing time. The PC rejects reused nonces and
non-increasing counters.
