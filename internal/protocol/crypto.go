package protocol

import (
	"crypto/ecdsa"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"hash"
	"math/big"
)

func RandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return b, nil
}

// HKDFSHA256 is a small RFC 5869 implementation kept here so the PC and phone
// protocol does not depend on an additional crypto package.
func HKDFSHA256(ikm, salt, info []byte, length int) ([]byte, error) {
	if length < 0 || length > 255*sha256.Size {
		return nil, errors.New("invalid HKDF output length")
	}
	if salt == nil {
		salt = make([]byte, sha256.Size)
	}
	extract := hmac.New(sha256.New, salt)
	_, _ = extract.Write(ikm)
	prk := extract.Sum(nil)
	defer Zero(prk)

	result := make([]byte, 0, length)
	var previous []byte
	for counter := byte(1); len(result) < length; counter++ {
		expand := hmac.New(sha256.New, prk)
		_, _ = expand.Write(previous)
		_, _ = expand.Write(info)
		_, _ = expand.Write([]byte{counter})
		previous = expand.Sum(nil)
		result = append(result, previous...)
	}
	Zero(previous)
	return result[:length], nil
}

func HMACSHA256(key []byte, parts ...[]byte) [sha256.Size]byte {
	var out [sha256.Size]byte
	m := hmac.New(sha256.New, key)
	writeParts(m, parts...)
	copy(out[:], m.Sum(nil))
	return out
}

func writeParts(h hash.Hash, parts ...[]byte) {
	for _, part := range parts {
		_, _ = h.Write(part)
	}
}

func SignP256(privateKey *ecdsa.PrivateKey, message []byte) ([SignatureSize]byte, error) {
	var signature [SignatureSize]byte
	digest := sha256.Sum256(message)
	r, s, err := ecdsa.Sign(rand.Reader, privateKey, digest[:])
	if err != nil {
		return signature, err
	}
	r.FillBytes(signature[:32])
	s.FillBytes(signature[32:])
	return signature, nil
}

func VerifyP256(publicKey *ecdsa.PublicKey, message []byte, signature []byte) bool {
	if publicKey == nil || len(signature) != SignatureSize {
		return false
	}
	digest := sha256.Sum256(message)
	r := new(big.Int).SetBytes(signature[:32])
	s := new(big.Int).SetBytes(signature[32:])
	return ecdsa.Verify(publicKey, digest[:], r, s)
}

func Equal(a, b []byte) bool {
	return len(a) == len(b) && subtle.ConstantTimeCompare(a, b) == 1
}

func Zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
