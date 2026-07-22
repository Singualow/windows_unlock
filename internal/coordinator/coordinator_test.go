package coordinator

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/singu/proximity-unlock/internal/authorize"
	"github.com/singu/proximity-unlock/internal/ble"
	"github.com/singu/proximity-unlock/internal/config"
	"github.com/singu/proximity-unlock/internal/ipc"
	"github.com/singu/proximity-unlock/internal/protocol"
	"github.com/singu/proximity-unlock/internal/secret"
)

type testSigner struct{ key *ecdsa.PrivateKey }

func (s testSigner) Sign(data []byte) ([64]byte, error)   { return protocol.SignP256(s.key, data) }
func (s testSigner) PublicKey() (*ecdsa.PublicKey, error) { return &s.key.PublicKey, nil }

type testTransport struct{}

func (testTransport) Start(context.Context, ble.Handler) error { return nil }
func (testTransport) Exchange(context.Context, ble.Candidate, byte, []byte) ([]byte, error) {
	return nil, ble.ErrUnavailable
}
func (testTransport) Backend() string { return "test" }
func (testTransport) Close() error    { return nil }

type phoneTransport struct {
	phoneKey *ecdsa.PrivateKey
	pcPublic *ecdsa.PublicKey
	counter  uint64
}

func (p *phoneTransport) Start(context.Context, ble.Handler) error { return nil }
func (p *phoneTransport) Backend() string                          { return "fake-phone" }
func (p *phoneTransport) Close() error                             { return nil }
func (p *phoneTransport) Exchange(_ context.Context, _ ble.Candidate, messageType byte, wire []byte) ([]byte, error) {
	if messageType != protocol.MessageChallenge {
		return nil, ble.ErrUnavailable
	}
	challenge, err := protocol.ParseChallenge(wire)
	if err != nil || !protocol.VerifyP256(p.pcPublic, challenge.SigningBytes(), challenge.Signature[:]) {
		return nil, ble.ErrUnavailable
	}
	p.counter++
	response := protocol.NewResponse(challenge, p.counter, time.Now())
	response.Signature, err = protocol.SignP256(p.phoneKey, response.SigningBytes())
	if err != nil {
		return nil, err
	}
	return response.MarshalBinary(), nil
}

func TestPairingURIControl(t *testing.T) {
	pcKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	pcID := make([]byte, protocol.DeviceIDSize)
	_, _ = rand.Read(pcID)
	cfg := config.Default()
	cfg.TargetSID = "S-1-5-21-test"
	cfg.CanonicalUser = `MicrosoftAccount\test@example.com`
	cfg.PCID = base64.RawURLEncoding.EncodeToString(pcID)
	path := t.TempDir() + "/config.json"
	if err := config.Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	c := New(path, cfg, secret.NewMemoryStore(), testSigner{pcKey}, testTransport{}, authorize.New(nil))
	response := c.HandleControl(context.Background(), mustRequest("pair_start", nil))
	if !response.OK {
		t.Fatalf("pair_start: %s", response.Error)
	}
}

func mustRequest(op string, payload any) ipc.ControlRequest {
	data, _ := json.Marshal(payload)
	return ipc.ControlRequest{Version: 1, Op: op, Payload: data}
}

func TestSessionDoesNotStartActive(t *testing.T) {
	pcKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	cfg := config.Default()
	c := New("unused", cfg, secret.NewMemoryStore(), testSigner{pcKey}, testTransport{}, authorize.New(nil))
	status := c.Status(time.Now())
	if status.SessionActive {
		t.Fatal("service restart must fail closed until user session announces itself")
	}
}

func TestNonConsoleSessionAnnouncementIsRejected(t *testing.T) {
	pcKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	c := New("unused", config.Default(), secret.NewMemoryStore(), testSigner{pcKey}, testTransport{}, authorize.New(nil))
	c.SetSessionValidator(func(sessionID uint32) bool { return sessionID == 7 })
	response := c.HandleControl(context.Background(), mustRequest("session_active", map[string]any{"session_id": 8}))
	if response.OK || c.Status(time.Now()).SessionActive {
		t.Fatal("non-console session was accepted")
	}
}

func TestImmediateUnlockControlIsPersistedAndReported(t *testing.T) {
	pcKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	cfg := config.Default()
	path := t.TempDir() + "/config.json"
	if err := config.Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	c := New(path, cfg, secret.NewMemoryStore(), testSigner{pcKey}, testTransport{}, authorize.New(nil))
	response := c.HandleControl(context.Background(), mustRequest("set_immediate_unlock", map[string]any{"enabled": true}))
	if !response.OK || !c.Status(time.Now()).ImmediateUnlock {
		t.Fatalf("immediate-unlock control failed: %s", response.Error)
	}
	saved, err := config.Load(path)
	if err != nil || !saved.ImmediateUnlock {
		t.Fatal("immediate-unlock preference was not saved")
	}
}

func TestFailureCooldownControlIsPersistedAndReported(t *testing.T) {
	pcKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	cfg := config.Default()
	path := t.TempDir() + "/config.json"
	if err := config.Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	c := New(path, cfg, secret.NewMemoryStore(), testSigner{pcKey}, testTransport{}, authorize.New(nil))
	response := c.HandleControl(context.Background(), mustRequest("set_failure_cooldown", map[string]any{"enabled": false}))
	if !response.OK || c.Status(time.Now()).FailureCooldown {
		t.Fatalf("failure-cooldown control failed: %s", response.Error)
	}
	saved, err := config.Load(path)
	if err != nil || saved.FailureCooldown {
		t.Fatal("failure-cooldown preference was not saved")
	}
}

func TestHighSensitivityControlIsPersistedAndReported(t *testing.T) {
	pcKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	cfg := config.Default()
	path := t.TempDir() + "/config.json"
	if err := config.Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	c := New(path, cfg, secret.NewMemoryStore(), testSigner{pcKey}, testTransport{}, authorize.New(nil))
	response := c.HandleControl(context.Background(), mustRequest("set_high_sensitivity", map[string]any{"enabled": true}))
	if !response.OK || !c.Status(time.Now()).HighSensitivity {
		t.Fatalf("high-sensitivity control failed: %s", response.Error)
	}
	saved, err := config.Load(path)
	if err != nil || !saved.HighSensitivity {
		t.Fatal("high-sensitivity preference was not saved")
	}
	settings := settingsFor(saved)
	if settings.ProofTimeout != 4*time.Second || settings.LockWindow != 2*time.Second || settings.RequiredNearSamples != 1 {
		t.Fatalf("unexpected high-sensitivity profile: %+v", settings)
	}
	if settings.UnlockRSSI != saved.Thresholds.HighSensitivityRSSI || settings.LockRSSI != saved.Thresholds.HighSensitivityRSSI-8 {
		t.Fatalf("dedicated high-sensitivity threshold was not applied: %+v", settings)
	}
	if challengeIntervalFor(saved, false) != time.Second || challengeIntervalFor(saved, true) != 200*time.Millisecond {
		t.Fatal("high-sensitivity challenge intervals were not shortened")
	}
	if authenticationTimeoutFor(saved) != 1500*time.Millisecond {
		t.Fatal("high-sensitivity authentication timeout was not shortened")
	}
}

func TestHighSensitivityThresholdControlIsPersistedAndReported(t *testing.T) {
	pcKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	cfg := config.Default()
	path := t.TempDir() + "/config.json"
	if err := config.Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	c := New(path, cfg, secret.NewMemoryStore(), testSigner{pcKey}, testTransport{}, authorize.New(nil))
	response := c.HandleControl(context.Background(), mustRequest("set_thresholds", map[string]any{
		"unlock_rssi": -64, "lock_rssi": -80, "high_sensitivity_rssi": -49,
	}))
	if !response.OK {
		t.Fatalf("high-sensitivity threshold control failed: %s", response.Error)
	}
	status := c.Status(time.Now())
	if status.UnlockRSSI != -64 || status.LockRSSI != -80 || status.HighSensitivityRSSI != -49 {
		t.Fatalf("threshold status was not updated: %+v", status)
	}
	saved, err := config.Load(path)
	if err != nil || saved.Thresholds.HighSensitivityRSSI != -49 {
		t.Fatal("high-sensitivity threshold was not saved")
	}
}

func TestLegacyThresholdControlPreservesHighSensitivityThreshold(t *testing.T) {
	pcKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	cfg := config.Default()
	path := t.TempDir() + "/config.json"
	if err := config.Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	c := New(path, cfg, secret.NewMemoryStore(), testSigner{pcKey}, testTransport{}, authorize.New(nil))
	response := c.HandleControl(context.Background(), mustRequest("set_thresholds", map[string]any{
		"unlock_rssi": -63, "lock_rssi": -79,
	}))
	if !response.OK {
		t.Fatalf("legacy threshold control failed: %s", response.Error)
	}
	if got := c.Status(time.Now()).HighSensitivityRSSI; got != cfg.Thresholds.HighSensitivityRSSI {
		t.Fatalf("legacy threshold control changed dedicated threshold to %d", got)
	}
}

func TestTransientTransportFailuresDoNotTriggerSecurityCooldown(t *testing.T) {
	pcKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	c := New("unused", config.Default(), secret.NewMemoryStore(), testSigner{pcKey}, testTransport{}, authorize.New(nil))
	for i := 0; i < 5; i++ {
		c.recordAuthenticationResult(authError("transport_timeout", "BLE 挑战响应超时", false, context.DeadlineExceeded))
	}
	status := c.Status(time.Now())
	if !status.CooldownUntil.IsZero() || status.LastAuthFailureCode != "transport_timeout" {
		t.Fatalf("transient transport failure triggered cooldown: %+v", status)
	}
}

func TestAuthenticationRecoveryLogMatchesRateLimitedFailure(t *testing.T) {
	pcKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	c := New("unused", config.Default(), secret.NewMemoryStore(), testSigner{pcKey}, testTransport{}, authorize.New(nil))
	var entries []AuthenticationLogEntry
	c.SetAuthenticationLogger(func(entry AuthenticationLogEntry) { entries = append(entries, entry) })

	c.recordAuthenticationResult(authError("transport_timeout", "BLE 挑战响应超时", false, context.DeadlineExceeded))
	c.recordAuthenticationResult(nil)
	c.recordAuthenticationResult(authError("transport_timeout", "BLE 挑战响应超时", false, context.DeadlineExceeded))
	c.recordAuthenticationResult(nil)

	if len(entries) != 2 || !entries[0].Warning || entries[1].Code != "authentication_recovered" {
		t.Fatalf("expected one failure/recovery pair, got %#v", entries)
	}
}

func TestSecurityFailuresStillTriggerCooldownWhenEnabled(t *testing.T) {
	pcKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	c := New("unused", config.Default(), secret.NewMemoryStore(), testSigner{pcKey}, testTransport{}, authorize.New(nil))
	for i := 0; i < 3; i++ {
		c.recordAuthenticationResult(authError("signature_invalid", "手机签名验证失败", true, errors.New("invalid signature")))
	}
	if !c.Status(time.Now()).CooldownUntil.After(time.Now()) {
		t.Fatal("security failures did not trigger cooldown")
	}
}

func TestAuthorizationIsBoundToCurrentLockedSession(t *testing.T) {
	pcKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	cfg := config.Default()
	cfg.TargetSID = "S-1-5-21-test"
	cfg.CanonicalUser = `MicrosoftAccount\test@example.com`
	cfg.CredentialValid = true
	store := secret.NewMemoryStore()
	if err := store.Put(secret.CredentialName(cfg.TargetSID), []byte("test-password")); err != nil {
		t.Fatal(err)
	}
	manager := authorize.New(nil)
	c := New("unused", cfg, store, testSigner{pcKey}, testTransport{}, manager)
	now := time.Now()
	c.MarkSessionActive(7)
	c.MarkLocked(7, now)
	if err := manager.Grant(8, now); err != nil {
		t.Fatal(err)
	}
	if got := c.HandleAuth(context.Background(), "CONSUME"); got.Status != ipc.AuthUnavailable {
		t.Fatal("authorization for another session was accepted")
	}

	if err := manager.Grant(7, now); err != nil {
		t.Fatal(err)
	}
	got := c.HandleAuth(context.Background(), "CONSUME")
	if got.Status != ipc.AuthAvailable || got.Password != "test-password" {
		t.Fatal("authorization for the current locked session was rejected")
	}
}

func TestFakePhoneChallengeGrantsSingleUseAuthorization(t *testing.T) {
	pcKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	phoneKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	var pcID, phoneID [protocol.DeviceIDSize]byte
	_, _ = rand.Read(pcID[:])
	_, _ = rand.Read(phoneID[:])
	cfg := config.Default()
	cfg.TargetSID = "S-1-5-21-test"
	cfg.CanonicalUser = `MicrosoftAccount\test@example.com`
	cfg.CredentialValid = true
	cfg.PCID = base64.RawURLEncoding.EncodeToString(pcID[:])
	cfg.PhoneID = base64.RawURLEncoding.EncodeToString(phoneID[:])
	cfg.PhoneStrictKey = base64.RawURLEncoding.EncodeToString(elliptic.Marshal(elliptic.P256(), phoneKey.X, phoneKey.Y))
	store := secret.NewMemoryStore()
	if err := store.Put(secret.CredentialName(cfg.TargetSID), []byte("test-password")); err != nil {
		t.Fatal(err)
	}
	manager := authorize.New(nil)
	transport := &phoneTransport{phoneKey: phoneKey, pcPublic: &pcKey.PublicKey}
	c := New("unused", cfg, store, testSigner{pcKey}, transport, manager)
	c.MarkSessionActive(11)
	c.MarkLocked(11, time.Now())
	if err := c.authenticate(context.Background(), ble.Candidate{Address: "fake"}); err != nil {
		t.Fatal(err)
	}
	if c.Status(time.Now()).LastAuthenticated.IsZero() {
		t.Fatal("successful phone proof was not exposed in service status")
	}
	if peek := c.HandleAuth(context.Background(), "PEEK"); peek.Status != ipc.AuthAvailable || peek.TargetSID != cfg.TargetSID {
		t.Fatal("fresh phone proof did not expose the expected credential tile")
	}
	if consume := c.HandleAuth(context.Background(), "CONSUME"); consume.Status != ipc.AuthAvailable || consume.Password != "test-password" {
		t.Fatal("fresh phone proof did not yield the stored credential")
	}
	if replay := c.HandleAuth(context.Background(), "CONSUME"); replay.Status != ipc.AuthUnavailable {
		t.Fatal("authorization was not single-use")
	}
}
