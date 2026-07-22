package protocol

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
)

var advertisementContext = []byte("ProximityUnlock/ad/v1")

type Advertisement struct {
	Version byte
	Salt    [4]byte
	Tag     [8]byte
}

func NewAdvertisement(presenceKey []byte) (Advertisement, error) {
	var result Advertisement
	result.Version = Version
	if len(presenceKey) < 16 {
		return result, errors.New("presence key must contain at least 16 bytes")
	}
	if _, err := rand.Read(result.Salt[:]); err != nil {
		return result, err
	}
	mac := HMACSHA256(presenceKey, advertisementContext, result.Salt[:])
	copy(result.Tag[:], mac[:len(result.Tag)])
	return result, nil
}

func ParseAdvertisement(data []byte) (Advertisement, error) {
	var result Advertisement
	if len(data) != AdvertisementSize {
		return result, errors.New("invalid advertisement length")
	}
	result.Version = data[0]
	copy(result.Salt[:], data[1:5])
	copy(result.Tag[:], data[5:13])
	if result.Version != Version {
		return Advertisement{}, errors.New("unsupported advertisement version")
	}
	return result, nil
}

func (a Advertisement) MarshalBinary() []byte {
	data := make([]byte, AdvertisementSize)
	data[0] = a.Version
	copy(data[1:5], a.Salt[:])
	copy(data[5:13], a.Tag[:])
	return data
}

func (a Advertisement) Verify(presenceKey []byte) bool {
	if a.Version != Version || len(presenceKey) < 16 {
		return false
	}
	mac := HMACSHA256(presenceKey, advertisementContext, a.Salt[:])
	return Equal(a.Tag[:], mac[:len(a.Tag)])
}

// SaltUint32 is useful for diagnostics without logging the rolling tag.
func (a Advertisement) SaltUint32() uint32 { return binary.BigEndian.Uint32(a.Salt[:]) }
