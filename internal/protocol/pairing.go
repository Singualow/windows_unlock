package protocol

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"time"
)

const PairingScheme = "proximityunlock"

type PairingURI struct {
	PCID          [DeviceIDSize]byte
	PCPublicKey   []byte // SEC1 uncompressed P-256 public key.
	PairingSecret [32]byte
	ExpiresAt     time.Time
}

func (p PairingURI) String() string {
	query := url.Values{}
	query.Set("v", strconv.Itoa(int(Version)))
	query.Set("pc", base64.RawURLEncoding.EncodeToString(p.PCID[:]))
	query.Set("pub", base64.RawURLEncoding.EncodeToString(p.PCPublicKey))
	query.Set("secret", base64.RawURLEncoding.EncodeToString(p.PairingSecret[:]))
	query.Set("exp", strconv.FormatInt(p.ExpiresAt.Unix(), 10))
	return (&url.URL{Scheme: PairingScheme, Host: "pair", RawQuery: query.Encode()}).String()
}

func ParsePairingURI(raw string, now time.Time) (PairingURI, error) {
	var result PairingURI
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != PairingScheme || u.Host != "pair" {
		return result, errors.New("invalid pairing URI")
	}
	version, err := strconv.Atoi(u.Query().Get("v"))
	if err != nil || byte(version) != Version {
		return result, errors.New("unsupported pairing URI version")
	}
	pcID, err := base64.RawURLEncoding.DecodeString(u.Query().Get("pc"))
	if err != nil || len(pcID) != DeviceIDSize {
		return result, errors.New("invalid PC identifier")
	}
	copy(result.PCID[:], pcID)
	result.PCPublicKey, err = base64.RawURLEncoding.DecodeString(u.Query().Get("pub"))
	if err != nil || len(result.PCPublicKey) != 65 || result.PCPublicKey[0] != 4 {
		return PairingURI{}, errors.New("invalid PC public key")
	}
	secret, err := base64.RawURLEncoding.DecodeString(u.Query().Get("secret"))
	if err != nil || len(secret) != len(result.PairingSecret) {
		return PairingURI{}, errors.New("invalid pairing secret")
	}
	copy(result.PairingSecret[:], secret)
	expires, err := strconv.ParseInt(u.Query().Get("exp"), 10, 64)
	if err != nil {
		return PairingURI{}, errors.New("invalid pairing expiry")
	}
	result.ExpiresAt = time.Unix(expires, 0)
	if !result.ExpiresAt.After(now) || result.ExpiresAt.After(now.Add(2*time.Minute+5*time.Second)) {
		return PairingURI{}, ErrExpired
	}
	return result, nil
}

func DerivePresenceKey(pairingSecret []byte, pcID, phoneID [DeviceIDSize]byte) ([]byte, error) {
	if len(pairingSecret) != 32 {
		return nil, fmt.Errorf("pairing secret length is %d, want 32", len(pairingSecret))
	}
	salt := sha256.Sum256(append(append([]byte("ProximityUnlock/pair/v1"), pcID[:]...), phoneID[:]...))
	return HKDFSHA256(pairingSecret, salt[:], []byte("presence-key"), 32)
}

func PairingAdvertisementToken(pairingSecret []byte, pcID [DeviceIDSize]byte) [8]byte {
	mac := HMACSHA256(pairingSecret, []byte("ProximityUnlock/pair-ad/v1"), pcID[:])
	var token [8]byte
	copy(token[:], mac[:8])
	return token
}
