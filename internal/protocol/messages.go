package protocol

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"time"
)

var (
	challengeMagic = [4]byte{'P', 'U', 'C', '1'}
	responseMagic  = [4]byte{'P', 'U', 'R', '1'}
)

const (
	challengeBodySize = 4 + 1 + 1 + 8 + 8 + DeviceIDSize + DeviceIDSize + 4 + SIDHashSize + NonceSize
	challengeSize     = challengeBodySize + SignatureSize
	responseBodySize  = 4 + 1 + 1 + 8 + 8 + DeviceIDSize + DeviceIDSize + NonceSize + sha256.Size
	responseSize      = responseBodySize + SignatureSize
)

type Challenge struct {
	Version       byte
	Mode          Mode
	IssuedUnixMS  int64
	ExpiresUnixMS int64
	PCID          [DeviceIDSize]byte
	PhoneID       [DeviceIDSize]byte
	SessionID     uint32
	TargetSIDHash [SIDHashSize]byte
	Nonce         [NonceSize]byte
	Signature     [SignatureSize]byte
}

func NewChallenge(mode Mode, pcID, phoneID [DeviceIDSize]byte, sessionID uint32, sid string, now time.Time) (Challenge, error) {
	var c Challenge
	if !mode.Valid() {
		return c, errors.New("invalid mode")
	}
	c.Version = Version
	c.Mode = mode
	c.IssuedUnixMS = now.UnixMilli()
	c.ExpiresUnixMS = now.Add(5 * time.Second).UnixMilli()
	c.PCID = pcID
	c.PhoneID = phoneID
	c.SessionID = sessionID
	c.TargetSIDHash = sha256.Sum256([]byte(sid))
	if _, err := randRead(c.Nonce[:]); err != nil {
		return Challenge{}, err
	}
	return c, nil
}

func (c Challenge) signingBytes() []byte {
	b := make([]byte, challengeBodySize)
	offset := 0
	offset += copy(b[offset:], challengeMagic[:])
	b[offset] = c.Version
	offset++
	b[offset] = byte(c.Mode)
	offset++
	binary.BigEndian.PutUint64(b[offset:], uint64(c.IssuedUnixMS))
	offset += 8
	binary.BigEndian.PutUint64(b[offset:], uint64(c.ExpiresUnixMS))
	offset += 8
	offset += copy(b[offset:], c.PCID[:])
	offset += copy(b[offset:], c.PhoneID[:])
	binary.BigEndian.PutUint32(b[offset:], c.SessionID)
	offset += 4
	offset += copy(b[offset:], c.TargetSIDHash[:])
	copy(b[offset:], c.Nonce[:])
	return b
}

func (c Challenge) SigningBytes() []byte { return c.signingBytes() }

func (c Challenge) MarshalBinary() ([]byte, error) {
	if err := c.Validate(time.Now(), false); err != nil && !errors.Is(err, ErrExpired) {
		return nil, err
	}
	b := make([]byte, challengeSize)
	copy(b, c.signingBytes())
	copy(b[challengeBodySize:], c.Signature[:])
	return b, nil
}

func ParseChallenge(b []byte) (Challenge, error) {
	var c Challenge
	if len(b) != challengeSize || string(b[:4]) != string(challengeMagic[:]) {
		return c, errors.New("invalid challenge encoding")
	}
	offset := 4
	c.Version = b[offset]
	offset++
	c.Mode = Mode(b[offset])
	offset++
	c.IssuedUnixMS = int64(binary.BigEndian.Uint64(b[offset:]))
	offset += 8
	c.ExpiresUnixMS = int64(binary.BigEndian.Uint64(b[offset:]))
	offset += 8
	offset += copy(c.PCID[:], b[offset:offset+DeviceIDSize])
	offset += copy(c.PhoneID[:], b[offset:offset+DeviceIDSize])
	c.SessionID = binary.BigEndian.Uint32(b[offset:])
	offset += 4
	offset += copy(c.TargetSIDHash[:], b[offset:offset+SIDHashSize])
	offset += copy(c.Nonce[:], b[offset:offset+NonceSize])
	copy(c.Signature[:], b[offset:])
	return c, nil
}

var ErrExpired = errors.New("message expired")

func (c Challenge) Validate(now time.Time, checkTime bool) error {
	if c.Version != Version || !c.Mode.Valid() || c.ExpiresUnixMS <= c.IssuedUnixMS || c.ExpiresUnixMS-c.IssuedUnixMS > 5000 {
		return errors.New("invalid challenge fields")
	}
	if checkTime && (now.UnixMilli() < c.IssuedUnixMS-1000 || now.UnixMilli() > c.ExpiresUnixMS) {
		return ErrExpired
	}
	return nil
}

type Response struct {
	Version         byte
	Mode            Mode
	SignedUnixMS    int64
	Counter         uint64
	PCID            [DeviceIDSize]byte
	PhoneID         [DeviceIDSize]byte
	Nonce           [NonceSize]byte
	ChallengeDigest [sha256.Size]byte
	Signature       [SignatureSize]byte
}

func NewResponse(c Challenge, counter uint64, now time.Time) Response {
	return Response{
		Version:         Version,
		Mode:            c.Mode,
		SignedUnixMS:    now.UnixMilli(),
		Counter:         counter,
		PCID:            c.PCID,
		PhoneID:         c.PhoneID,
		Nonce:           c.Nonce,
		ChallengeDigest: sha256.Sum256(c.signingBytes()),
	}
}

func (r Response) SigningBytes() []byte {
	b := make([]byte, responseBodySize)
	offset := 0
	offset += copy(b[offset:], responseMagic[:])
	b[offset] = r.Version
	offset++
	b[offset] = byte(r.Mode)
	offset++
	binary.BigEndian.PutUint64(b[offset:], uint64(r.SignedUnixMS))
	offset += 8
	binary.BigEndian.PutUint64(b[offset:], r.Counter)
	offset += 8
	offset += copy(b[offset:], r.PCID[:])
	offset += copy(b[offset:], r.PhoneID[:])
	offset += copy(b[offset:], r.Nonce[:])
	copy(b[offset:], r.ChallengeDigest[:])
	return b
}

func (r Response) MarshalBinary() []byte {
	b := make([]byte, responseSize)
	copy(b, r.SigningBytes())
	copy(b[responseBodySize:], r.Signature[:])
	return b
}

func ParseResponse(b []byte) (Response, error) {
	var r Response
	if len(b) != responseSize || string(b[:4]) != string(responseMagic[:]) {
		return r, errors.New("invalid response encoding")
	}
	offset := 4
	r.Version = b[offset]
	offset++
	r.Mode = Mode(b[offset])
	offset++
	r.SignedUnixMS = int64(binary.BigEndian.Uint64(b[offset:]))
	offset += 8
	r.Counter = binary.BigEndian.Uint64(b[offset:])
	offset += 8
	offset += copy(r.PCID[:], b[offset:offset+DeviceIDSize])
	offset += copy(r.PhoneID[:], b[offset:offset+DeviceIDSize])
	offset += copy(r.Nonce[:], b[offset:offset+NonceSize])
	offset += copy(r.ChallengeDigest[:], b[offset:offset+sha256.Size])
	copy(r.Signature[:], b[offset:])
	return r, nil
}

func (r Response) Matches(c Challenge, now time.Time) bool {
	digest := sha256.Sum256(c.signingBytes())
	return r.Version == Version && r.Mode == c.Mode && r.PCID == c.PCID && r.PhoneID == c.PhoneID &&
		r.Nonce == c.Nonce && r.ChallengeDigest == digest && r.SignedUnixMS >= c.IssuedUnixMS-1000 &&
		r.SignedUnixMS <= c.ExpiresUnixMS && now.UnixMilli() <= c.ExpiresUnixMS
}

var randRead = func(b []byte) (int, error) {
	data, err := RandomBytes(len(b))
	if err != nil {
		return 0, err
	}
	return copy(b, data), nil
}
