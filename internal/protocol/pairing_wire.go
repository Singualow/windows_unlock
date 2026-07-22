package protocol

import (
	"errors"
)

const (
	MessageChallenge byte = 1
	MessagePairing   byte = 2

	pairingChallengeBodySize = 4 + 1 + DeviceIDSize + NonceSize
	pairingChallengeSize     = pairingChallengeBodySize + 32
	pairingResponseBodySize  = 4 + 1 + DeviceIDSize + DeviceIDSize + 65 + 65 + NonceSize
	pairingResponseSize      = pairingResponseBodySize + 32
)

var (
	pairingChallengeMagic = [4]byte{'P', 'U', 'Q', '1'}
	pairingResponseMagic  = [4]byte{'P', 'U', 'P', '1'}
)

type PairingChallenge struct {
	Version byte
	PCID    [DeviceIDSize]byte
	Nonce   [NonceSize]byte
	MAC     [32]byte
}

func NewPairingChallenge(pcID [DeviceIDSize]byte, secret []byte) (PairingChallenge, error) {
	var result PairingChallenge
	if len(secret) != 32 {
		return result, errors.New("invalid pairing secret")
	}
	result.Version = Version
	result.PCID = pcID
	if _, err := randRead(result.Nonce[:]); err != nil {
		return PairingChallenge{}, err
	}
	result.MAC = HMACSHA256(secret, result.signingBytes())
	return result, nil
}

func (p PairingChallenge) signingBytes() []byte {
	b := make([]byte, pairingChallengeBodySize)
	copy(b, pairingChallengeMagic[:])
	b[4] = p.Version
	copy(b[5:5+DeviceIDSize], p.PCID[:])
	copy(b[5+DeviceIDSize:], p.Nonce[:])
	return b
}

func (p PairingChallenge) MarshalBinary() []byte {
	b := make([]byte, pairingChallengeSize)
	copy(b, p.signingBytes())
	copy(b[pairingChallengeBodySize:], p.MAC[:])
	return b
}

func ParsePairingChallenge(b, secret []byte) (PairingChallenge, error) {
	var result PairingChallenge
	if len(b) != pairingChallengeSize || string(b[:4]) != string(pairingChallengeMagic[:]) || b[4] != Version {
		return result, errors.New("invalid pairing challenge")
	}
	result.Version = b[4]
	copy(result.PCID[:], b[5:5+DeviceIDSize])
	copy(result.Nonce[:], b[5+DeviceIDSize:pairingChallengeBodySize])
	copy(result.MAC[:], b[pairingChallengeBodySize:])
	expected := HMACSHA256(secret, result.signingBytes())
	if !Equal(result.MAC[:], expected[:]) {
		return PairingChallenge{}, errors.New("pairing challenge MAC failed")
	}
	return result, nil
}

type PairingResponse struct {
	Version          byte
	PCID             [DeviceIDSize]byte
	PhoneID          [DeviceIDSize]byte
	StrictPublicKey  [65]byte
	RelaxedPublicKey [65]byte
	Nonce            [NonceSize]byte
	MAC              [32]byte
}

func (p PairingResponse) signingBytes() []byte {
	b := make([]byte, pairingResponseBodySize)
	offset := 0
	offset += copy(b[offset:], pairingResponseMagic[:])
	b[offset] = p.Version
	offset++
	offset += copy(b[offset:], p.PCID[:])
	offset += copy(b[offset:], p.PhoneID[:])
	offset += copy(b[offset:], p.StrictPublicKey[:])
	offset += copy(b[offset:], p.RelaxedPublicKey[:])
	copy(b[offset:], p.Nonce[:])
	return b
}

func (p PairingResponse) MarshalBinary(secret []byte) ([]byte, error) {
	if p.Version != Version || len(secret) != 32 || p.StrictPublicKey[0] != 4 || p.RelaxedPublicKey[0] != 4 {
		return nil, errors.New("invalid pairing response fields")
	}
	p.MAC = HMACSHA256(secret, p.signingBytes())
	b := make([]byte, pairingResponseSize)
	copy(b, p.signingBytes())
	copy(b[pairingResponseBodySize:], p.MAC[:])
	return b, nil
}

func ParsePairingResponse(b, secret []byte, challenge PairingChallenge) (PairingResponse, error) {
	var result PairingResponse
	if len(b) != pairingResponseSize || string(b[:4]) != string(pairingResponseMagic[:]) || b[4] != Version {
		return result, errors.New("invalid pairing response")
	}
	offset := 4
	result.Version = b[offset]
	offset++
	offset += copy(result.PCID[:], b[offset:offset+DeviceIDSize])
	offset += copy(result.PhoneID[:], b[offset:offset+DeviceIDSize])
	offset += copy(result.StrictPublicKey[:], b[offset:offset+65])
	offset += copy(result.RelaxedPublicKey[:], b[offset:offset+65])
	offset += copy(result.Nonce[:], b[offset:offset+NonceSize])
	copy(result.MAC[:], b[offset:])
	if result.PCID != challenge.PCID || result.Nonce != challenge.Nonce || result.StrictPublicKey[0] != 4 || result.RelaxedPublicKey[0] != 4 {
		return PairingResponse{}, errors.New("pairing response is not bound to challenge")
	}
	expected := HMACSHA256(secret, result.signingBytes())
	if !Equal(result.MAC[:], expected[:]) {
		return PairingResponse{}, errors.New("pairing response MAC failed")
	}
	return result, nil
}

func PairingAdvertisement(secret []byte, pcID [DeviceIDSize]byte) []byte {
	token := PairingAdvertisementToken(secret, pcID)
	b := make([]byte, AdvertisementSize)
	b[0] = 0x80 | Version
	copy(b[1:5], pcID[:4])
	copy(b[5:], token[:])
	return b
}

func VerifyPairingAdvertisement(data, secret []byte, pcID [DeviceIDSize]byte) bool {
	if len(data) != AdvertisementSize || data[0] != 0x80|Version || !Equal(data[1:5], pcID[:4]) {
		return false
	}
	token := PairingAdvertisementToken(secret, pcID)
	return Equal(data[5:], token[:])
}
