package windowskey

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"errors"
)

func MarshalPublic(key *ecdsa.PublicKey) ([]byte, error) {
	if key == nil || key.Curve != elliptic.P256() || !key.Curve.IsOnCurve(key.X, key.Y) {
		return nil, errors.New("not a valid P-256 public key")
	}
	return elliptic.Marshal(elliptic.P256(), key.X, key.Y), nil
}

func ParsePublic(data []byte) (*ecdsa.PublicKey, error) {
	x, y := elliptic.Unmarshal(elliptic.P256(), data)
	if x == nil || y == nil {
		return nil, errors.New("invalid SEC1 P-256 public key")
	}
	return &ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y}, nil
}
