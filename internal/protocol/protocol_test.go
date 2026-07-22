package protocol

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"errors"
	"testing"
	"time"
)

func TestAdvertisementAuthentication(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	ad, err := NewAdvertisement(key)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := ParseAdvertisement(ad.MarshalBinary())
	if err != nil {
		t.Fatal(err)
	}
	if !decoded.Verify(key) {
		t.Fatal("valid rolling advertisement was rejected")
	}
	decoded.Tag[0] ^= 1
	if decoded.Verify(key) {
		t.Fatal("tampered rolling advertisement was accepted")
	}
}

func TestChallengeResponseAndReplay(t *testing.T) {
	pcKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	phoneKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	now := time.Unix(1_800_000_000, 0)
	var pcID, phoneID [DeviceIDSize]byte
	copy(pcID[:], []byte("pc-identity-00001"))
	copy(phoneID[:], []byte("phone-identity01"))
	c, err := NewChallenge(ModeStrict, pcID, phoneID, 1, "S-1-5-21-test", now)
	if err != nil {
		t.Fatal(err)
	}
	c.Signature, err = SignP256(pcKey, c.SigningBytes())
	if err != nil || !VerifyP256(&pcKey.PublicKey, c.SigningBytes(), c.Signature[:]) {
		t.Fatal("PC signature failed")
	}
	wire, err := c.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseChallenge(wire)
	if err != nil || parsed.Nonce != c.Nonce {
		t.Fatal("challenge round trip failed")
	}
	r := NewResponse(c, 1, now.Add(time.Second))
	r.Signature, err = SignP256(phoneKey, r.SigningBytes())
	if err != nil || !r.Matches(c, now.Add(2*time.Second)) || !VerifyP256(&phoneKey.PublicKey, r.SigningBytes(), r.Signature[:]) {
		t.Fatal("valid response failed")
	}
	guard := NewReplayGuard(time.Minute)
	if err := guard.Accept(r.Nonce, r.Counter, now); err != nil {
		t.Fatal(err)
	}
	if err := guard.Accept(r.Nonce, r.Counter+1, now); !errors.Is(err, ErrReplay) {
		t.Fatalf("want replay error, got %v", err)
	}
	second := r.Nonce
	second[0] ^= 1
	if err := guard.Accept(second, r.Counter, now); !errors.Is(err, ErrCounterRollback) {
		t.Fatalf("want counter rollback error, got %v", err)
	}
	if r.Matches(c, now.Add(6*time.Second)) {
		t.Fatal("expired response was accepted")
	}
}

func TestPresenceKeyBindsBothIdentities(t *testing.T) {
	secret := make([]byte, 32)
	_, _ = rand.Read(secret)
	var pcID, phoneA, phoneB [DeviceIDSize]byte
	_, _ = rand.Read(pcID[:])
	_, _ = rand.Read(phoneA[:])
	phoneB = phoneA
	phoneB[0] ^= 1
	first, err := DerivePresenceKey(secret, pcID, phoneA)
	if err != nil {
		t.Fatal(err)
	}
	again, _ := DerivePresenceKey(secret, pcID, phoneA)
	other, _ := DerivePresenceKey(secret, pcID, phoneB)
	if !Equal(first, again) || Equal(first, other) {
		t.Fatal("presence key did not bind the PC and phone identities")
	}
}

func TestFragmentReassemblyOutOfOrder(t *testing.T) {
	payload := make([]byte, 421)
	_, _ = rand.Read(payload)
	fragments, err := Fragment(1, payload, 80)
	if err != nil {
		t.Fatal(err)
	}
	r := new(Reassembler)
	var got []byte
	for i := len(fragments) - 1; i >= 0; i-- {
		var complete bool
		got, complete, err = r.Add(fragments[i])
		if err != nil {
			t.Fatal(err)
		}
		if complete && i != 0 {
			t.Fatal("completed before all fragments arrived")
		}
	}
	if !Equal(got, payload) {
		t.Fatal("reassembled payload differs")
	}
}

func TestPairingURI(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	p := PairingURI{ExpiresAt: now.Add(2 * time.Minute)}
	_, _ = rand.Read(p.PCID[:])
	_, _ = rand.Read(p.PairingSecret[:])
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	p.PCPublicKey = elliptic.Marshal(elliptic.P256(), key.X, key.Y)
	got, err := ParsePairingURI(p.String(), now)
	if err != nil {
		t.Fatal(err)
	}
	if got.PCID != p.PCID || got.PairingSecret != p.PairingSecret {
		t.Fatal("pairing URI did not round trip")
	}
}
