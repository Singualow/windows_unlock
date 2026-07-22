package protocol

import "errors"

const (
	Version byte = 1

	ServiceUUID   = "9b7c6a10-5d57-4c2e-8e2a-4ed2f5f7a001"
	ChallengeUUID = "9b7c6a10-5d57-4c2e-8e2a-4ed2f5f7a002"
	ResponseUUID  = "9b7c6a10-5d57-4c2e-8e2a-4ed2f5f7a003"
	PairingUUID   = "9b7c6a10-5d57-4c2e-8e2a-4ed2f5f7a004"
	// A compact vendor-specific 16-bit UUID keeps the rolling token within
	// the 31-byte legacy advertisement limit. GATT still uses ServiceUUID.
	AdvertisementUUID             = "0000fff0-0000-1000-8000-00805f9b34fb"
	AdvertisementCompanyID uint16 = 0xffff

	AdvertisementSize = 13
	NonceSize         = 32
	DeviceIDSize      = 16
	SIDHashSize       = 32
	SignatureSize     = 64 // IEEE P1363: r || s for P-256.
)

type Mode byte

const (
	ModeStrict      Mode = 1
	ModeConvenience Mode = 2
)

func (m Mode) Valid() bool { return m == ModeStrict || m == ModeConvenience }

func (m Mode) String() string {
	switch m {
	case ModeStrict:
		return "strict"
	case ModeConvenience:
		return "convenience"
	default:
		return "unknown"
	}
}

func ParseMode(v string) (Mode, error) {
	switch v {
	case "strict":
		return ModeStrict, nil
	case "convenience":
		return ModeConvenience, nil
	default:
		return 0, errors.New("unsupported unlock mode")
	}
}
