package coordinator

import (
	"context"
	"crypto/ecdsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/singu/proximity-unlock/internal/authorize"
	"github.com/singu/proximity-unlock/internal/ble"
	"github.com/singu/proximity-unlock/internal/config"
	"github.com/singu/proximity-unlock/internal/ipc"
	"github.com/singu/proximity-unlock/internal/protocol"
	"github.com/singu/proximity-unlock/internal/proximity"
	"github.com/singu/proximity-unlock/internal/secret"
	"github.com/singu/proximity-unlock/internal/windowskey"
)

type Signer interface {
	Sign([]byte) ([protocol.SignatureSize]byte, error)
	PublicKey() (*ecdsa.PublicKey, error)
}

type Coordinator struct {
	mu sync.Mutex

	configPath string
	config     config.Config
	secrets    secret.Store
	signer     Signer
	transport  ble.Transport
	state      *proximity.State
	authorizer *authorize.Manager
	replay     *protocol.ReplayGuard

	activeSession uint32
	sessionActive bool
	locked        bool
	challengeBusy bool
	lastChallenge time.Time
	pairing       *pendingPairing
	sessionValid  func(uint32) bool
	lastCPEvent   string
	lastCPEventAt time.Time
}

func (c *Coordinator) SetSessionValidator(validator func(uint32) bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sessionValid = validator
}

type pendingPairing struct {
	secret    [32]byte
	expiresAt time.Time
}

type Status struct {
	Configured        bool               `json:"configured"`
	Paired            bool               `json:"paired"`
	CredentialValid   bool               `json:"credential_valid"`
	Mode              string             `json:"mode"`
	AutoLock          bool               `json:"auto_lock"`
	ImmediateUnlock   bool               `json:"immediate_unlock"`
	PausedUntil       time.Time          `json:"paused_until,omitempty"`
	SessionActive     bool               `json:"session_active"`
	Locked            bool               `json:"locked"`
	MedianRSSI        int                `json:"median_rssi,omitempty"`
	HasRSSI           bool               `json:"has_rssi"`
	LastAuthenticated time.Time          `json:"last_authenticated,omitempty"`
	ShouldLock        bool               `json:"should_lock"`
	BLEBackend        string             `json:"ble_backend"`
	Authorization     authorize.Snapshot `json:"authorization"`
	CooldownUntil     time.Time          `json:"cooldown_until,omitempty"`
	LastCPEvent       string             `json:"last_credential_provider_event,omitempty"`
	LastCPEventAt     time.Time          `json:"last_credential_provider_event_at,omitempty"`
}

func New(configPath string, cfg config.Config, secrets secret.Store, signer Signer, transport ble.Transport, authorizer *authorize.Manager) *Coordinator {
	settings := settingsFor(cfg)
	return &Coordinator{
		configPath: configPath,
		config:     cfg,
		secrets:    secrets,
		signer:     signer,
		transport:  transport,
		state:      proximity.NewState(settings),
		authorizer: authorizer,
		replay:     protocol.NewReplayGuard(time.Minute),
	}
}

func settingsFor(cfg config.Config) proximity.Settings {
	settings := proximity.DefaultSettings()
	settings.UnlockRSSI = cfg.Thresholds.UnlockRSSI
	settings.LockRSSI = cfg.Thresholds.LockRSSI
	settings.UnlockWindow = time.Duration(cfg.Thresholds.UnlockWindowMS) * time.Millisecond
	settings.LockWindow = time.Duration(cfg.Thresholds.LockWindowMS) * time.Millisecond
	settings.ProofTimeout = time.Duration(cfg.Thresholds.ProofTimeoutMS) * time.Millisecond
	settings.ManualHoldAway = time.Duration(cfg.Thresholds.ManualHoldAwayMS) * time.Millisecond
	settings.ImmediateUnlock = cfg.ImmediateUnlock
	return settings
}

func (c *Coordinator) Start(ctx context.Context) error {
	return c.transport.Start(ctx, c.handleCandidate)
}

func (c *Coordinator) Close() error { return c.transport.Close() }

func (c *Coordinator) MarkSessionActive(sessionID uint32) {
	c.mu.Lock()
	c.activeSession = sessionID
	c.sessionActive = true
	c.locked = false
	c.mu.Unlock()
	c.state.OnUnlock(time.Now())
	c.authorizer.Cancel()
}

func (c *Coordinator) MarkLocked(sessionID uint32, now time.Time) {
	c.mu.Lock()
	if c.sessionActive && sessionID == c.activeSession {
		c.locked = true
		c.mu.Unlock()
		c.state.OnLock(now)
		return
	}
	c.mu.Unlock()
}

func (c *Coordinator) MarkUnlocked(sessionID uint32) { c.MarkSessionActive(sessionID) }

func (c *Coordinator) MarkLoggedOff(sessionID uint32) {
	c.mu.Lock()
	if c.activeSession == sessionID {
		c.sessionActive = false
		c.locked = false
		c.activeSession = 0
	}
	c.mu.Unlock()
	c.authorizer.Cancel()
}

func (c *Coordinator) MarkResume(now time.Time) {
	c.mu.Lock()
	locked := c.locked && c.sessionActive
	c.mu.Unlock()
	if locked {
		c.state.OnResume(now)
	}
}

func (c *Coordinator) Status(now time.Time) Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	rssi, hasRSSI := c.state.MedianRSSI(now)
	pausedUntil := time.Time{}
	if c.config.PausedUntil != nil {
		pausedUntil = *c.config.PausedUntil
	}
	shouldLock := c.config.AutoLock && c.sessionActive && !c.locked && !pausedUntil.After(now) && c.state.ShouldAutoLock(now)
	return Status{
		Configured:        c.config.TargetSID != "" && c.config.PCID != "",
		Paired:            c.config.PhoneID != "",
		CredentialValid:   c.config.CredentialValid,
		Mode:              c.config.Mode,
		AutoLock:          c.config.AutoLock,
		ImmediateUnlock:   c.config.ImmediateUnlock,
		PausedUntil:       pausedUntil,
		SessionActive:     c.sessionActive,
		Locked:            c.locked,
		MedianRSSI:        rssi,
		HasRSSI:           hasRSSI,
		LastAuthenticated: c.state.LastProof(),
		ShouldLock:        shouldLock,
		BLEBackend:        c.transport.Backend(),
		Authorization:     c.authorizer.Snapshot(now),
		CooldownUntil:     c.state.CooldownUntil(),
		LastCPEvent:       c.lastCPEvent,
		LastCPEventAt:     c.lastCPEventAt,
	}
}

func (c *Coordinator) HandleControl(_ context.Context, request ipc.ControlRequest) ipc.ControlResponse {
	now := time.Now()
	switch request.Op {
	case "status":
		return ipc.ControlResponse{OK: true, Payload: c.Status(now)}
	case "session_active":
		var payload struct {
			SessionID uint32 `json:"session_id"`
		}
		if json.Unmarshal(request.Payload, &payload) != nil || payload.SessionID == 0 {
			return ipc.ControlResponse{Error: "invalid session"}
		}
		c.mu.Lock()
		validator := c.sessionValid
		c.mu.Unlock()
		if validator != nil && !validator(payload.SessionID) {
			return ipc.ControlResponse{Error: "session is not the active local console"}
		}
		c.MarkSessionActive(payload.SessionID)
		return ipc.ControlResponse{OK: true}
	case "set_mode":
		var payload struct {
			Mode string `json:"mode"`
		}
		if json.Unmarshal(request.Payload, &payload) != nil {
			return ipc.ControlResponse{Error: "invalid mode request"}
		}
		if _, err := protocol.ParseMode(payload.Mode); err != nil {
			return ipc.ControlResponse{Error: err.Error()}
		}
		if err := c.updateConfig(func(cfg *config.Config) { cfg.Mode = payload.Mode }); err != nil {
			return ipc.ControlResponse{Error: err.Error()}
		}
		return ipc.ControlResponse{OK: true}
	case "set_auto_lock":
		var payload struct {
			Enabled *bool `json:"enabled"`
		}
		if json.Unmarshal(request.Payload, &payload) != nil || payload.Enabled == nil {
			return ipc.ControlResponse{Error: "invalid auto-lock request"}
		}
		if err := c.updateConfig(func(cfg *config.Config) { cfg.AutoLock = *payload.Enabled }); err != nil {
			return ipc.ControlResponse{Error: err.Error()}
		}
		return ipc.ControlResponse{OK: true}
	case "set_immediate_unlock":
		var payload struct {
			Enabled *bool `json:"enabled"`
		}
		if json.Unmarshal(request.Payload, &payload) != nil || payload.Enabled == nil {
			return ipc.ControlResponse{Error: "invalid immediate-unlock request"}
		}
		if err := c.updateConfig(func(cfg *config.Config) { cfg.ImmediateUnlock = *payload.Enabled }); err != nil {
			return ipc.ControlResponse{Error: err.Error()}
		}
		c.state.SetImmediateUnlock(*payload.Enabled)
		return ipc.ControlResponse{OK: true}
	case "set_thresholds":
		var payload struct {
			UnlockRSSI int `json:"unlock_rssi"`
			LockRSSI   int `json:"lock_rssi"`
		}
		if json.Unmarshal(request.Payload, &payload) != nil {
			return ipc.ControlResponse{Error: "invalid threshold request"}
		}
		if err := c.updateConfig(func(cfg *config.Config) {
			cfg.Thresholds.UnlockRSSI = payload.UnlockRSSI
			cfg.Thresholds.LockRSSI = payload.LockRSSI
		}); err != nil {
			return ipc.ControlResponse{Error: err.Error()}
		}
		c.mu.Lock()
		settings := settingsFor(c.config)
		c.mu.Unlock()
		c.state.UpdateSettings(settings)
		return ipc.ControlResponse{OK: true}
	case "pause":
		var payload struct {
			Seconds int `json:"seconds"`
		}
		if json.Unmarshal(request.Payload, &payload) != nil || payload.Seconds < 0 || payload.Seconds > 86400 {
			return ipc.ControlResponse{Error: "invalid pause duration"}
		}
		until := now.Add(time.Duration(payload.Seconds) * time.Second)
		if payload.Seconds == 0 {
			until = time.Time{}
		}
		if err := c.updateConfig(func(cfg *config.Config) {
			if until.IsZero() {
				cfg.PausedUntil = nil
			} else {
				cfg.PausedUntil = &until
			}
		}); err != nil {
			return ipc.ControlResponse{Error: err.Error()}
		}
		return ipc.ControlResponse{OK: true, Payload: map[string]any{"paused_until": until}}
	case "pair_start":
		uri, err := c.startPairing(now)
		if err != nil {
			return ipc.ControlResponse{Error: err.Error()}
		}
		return ipc.ControlResponse{OK: true, Payload: map[string]any{"uri": uri, "expires_at": now.Add(2 * time.Minute)}}
	case "revoke":
		if err := c.revoke(); err != nil {
			return ipc.ControlResponse{Error: err.Error()}
		}
		return ipc.ControlResponse{OK: true}
	case "reload":
		next, err := config.Load(c.configPath)
		if err != nil {
			return ipc.ControlResponse{Error: err.Error()}
		}
		c.mu.Lock()
		if next.TargetSID != c.config.TargetSID || next.PCID != c.config.PCID {
			c.mu.Unlock()
			return ipc.ControlResponse{Error: "identity fields cannot be changed while the service is running"}
		}
		c.config = next
		c.mu.Unlock()
		c.state.UpdateSettings(settingsFor(next))
		if next.CredentialValid {
			c.authorizer.Enable()
		} else {
			c.authorizer.Disable()
		}
		return ipc.ControlResponse{OK: true}
	default:
		return ipc.ControlResponse{Error: "unknown operation"}
	}
}

func (c *Coordinator) HandleAuth(_ context.Context, command string) ipc.AuthResponse {
	now := time.Now()
	if strings.HasPrefix(command, "DIAG ") {
		c.mu.Lock()
		c.lastCPEvent = strings.TrimSpace(strings.TrimPrefix(command, "DIAG "))
		c.lastCPEventAt = now
		c.mu.Unlock()
		return ipc.AuthResponse{Status: ipc.AuthUnavailable}
	}
	switch command {
	case "PEEK":
		if grant, ok := c.authorizer.Peek(now); ok {
			c.mu.Lock()
			validSession := c.sessionActive && c.locked && c.activeSession == grant.SessionID
			username := c.config.CanonicalUser
			targetSID := c.config.TargetSID
			c.mu.Unlock()
			if !validSession {
				c.authorizer.Cancel()
				return ipc.AuthResponse{Status: ipc.AuthUnavailable}
			}
			return ipc.AuthResponse{Status: ipc.AuthAvailable, Username: username, TargetSID: targetSID}
		}
		return ipc.AuthResponse{Status: ipc.AuthUnavailable}
	case "CONSUME":
		grant, err := c.authorizer.Consume(now)
		if err != nil {
			return ipc.AuthResponse{Status: ipc.AuthUnavailable}
		}
		c.mu.Lock()
		cfg := c.config
		validSession := c.sessionActive && c.locked && c.activeSession == grant.SessionID
		c.mu.Unlock()
		if !validSession || !cfg.CredentialValid {
			return ipc.AuthResponse{Status: ipc.AuthUnavailable}
		}
		password, err := c.secrets.Get(secret.CredentialName(cfg.TargetSID))
		if err != nil {
			return ipc.AuthResponse{Status: ipc.AuthError}
		}
		defer protocol.Zero(password)
		return ipc.AuthResponse{Status: ipc.AuthAvailable, Username: cfg.CanonicalUser, Password: string(password), TargetSID: cfg.TargetSID}
	case "INVALID":
		c.invalidateCredential()
		return ipc.AuthResponse{Status: ipc.AuthInvalid}
	case "SUCCESS":
		c.authorizer.Cancel()
		return ipc.AuthResponse{Status: ipc.AuthUnavailable}
	default:
		return ipc.AuthResponse{Status: ipc.AuthError}
	}
}

func (c *Coordinator) invalidateCredential() {
	c.authorizer.Disable()
	c.mu.Lock()
	c.config.CredentialValid = false
	next := c.config
	targetSID := c.config.TargetSID
	c.mu.Unlock()
	// Remove the rejected password first. Even if persisting config fails, a
	// service restart cannot retrieve and submit the stale credential again.
	_ = c.secrets.Delete(secret.CredentialName(targetSID))
	_ = config.Save(c.configPath, next)
}

func (c *Coordinator) handleCandidate(ctx context.Context, candidate ble.Candidate) {
	c.mu.Lock()
	cfg := c.config
	pairing := c.pairing
	if pairing != nil && time.Now().After(pairing.expiresAt) {
		protocol.Zero(pairing.secret[:])
		c.pairing = nil
		pairing = nil
	}
	c.mu.Unlock()
	pcID, err := decodeID(cfg.PCID)
	if err != nil {
		return
	}
	if pairing != nil && protocol.VerifyPairingAdvertisement(candidate.ServiceData, pairing.secret[:], pcID) {
		c.completePairing(ctx, candidate, pcID, pairing)
		return
	}
	if cfg.PhoneID == "" || cfg.PresenceSecretID == "" {
		return
	}
	presenceKey, err := c.secrets.Get(secret.PresenceName(cfg.PresenceSecretID))
	if err != nil {
		return
	}
	ad, err := protocol.ParseAdvertisement(candidate.ServiceData)
	valid := err == nil && ad.Verify(presenceKey)
	protocol.Zero(presenceKey)
	if !valid {
		return
	}
	now := time.Now()
	c.state.ObserveRSSI(candidate.RSSI, now)
	c.mu.Lock()
	paused := cfg.PausedUntil != nil && cfg.PausedUntil.After(now)
	shouldChallenge := c.sessionActive && !paused && !c.challengeBusy && c.state.CanAuthenticate(now) && now.Sub(c.lastChallenge) >= 3*time.Second && (!c.locked || c.state.ShouldAttemptUnlock(now))
	if shouldChallenge {
		c.challengeBusy = true
		c.lastChallenge = now
	}
	c.mu.Unlock()
	if !shouldChallenge {
		return
	}
	go func() {
		defer func() {
			c.mu.Lock()
			c.challengeBusy = false
			c.mu.Unlock()
		}()
		if err := c.authenticate(ctx, candidate); err != nil {
			c.state.RecordFailure(time.Now())
		}
	}()
}

func (c *Coordinator) authenticate(parent context.Context, candidate ble.Candidate) error {
	c.mu.Lock()
	cfg := c.config
	sessionID := c.activeSession
	locked := c.locked
	c.mu.Unlock()
	mode, err := protocol.ParseMode(cfg.Mode)
	if err != nil {
		return err
	}
	pcID, err := decodeID(cfg.PCID)
	if err != nil {
		return err
	}
	phoneID, err := decodeID(cfg.PhoneID)
	if err != nil {
		return err
	}
	challenge, err := protocol.NewChallenge(mode, pcID, phoneID, sessionID, cfg.TargetSID, time.Now())
	if err != nil {
		return err
	}
	challenge.Signature, err = c.signer.Sign(challenge.SigningBytes())
	if err != nil {
		return err
	}
	wire, err := challenge.MarshalBinary()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	responseWire, err := c.transport.Exchange(ctx, candidate, protocol.MessageChallenge, wire)
	if err != nil {
		return err
	}
	response, err := protocol.ParseResponse(responseWire)
	if err != nil || !response.Matches(challenge, time.Now()) {
		return errors.New("phone response did not match challenge")
	}
	keyData := cfg.PhoneStrictKey
	if mode == protocol.ModeConvenience {
		keyData = cfg.PhoneRelaxedKey
	}
	publicBytes, err := base64.RawURLEncoding.DecodeString(keyData)
	if err != nil {
		return err
	}
	publicKey, err := windowskey.ParsePublic(publicBytes)
	if err != nil || !protocol.VerifyP256(publicKey, response.SigningBytes(), response.Signature[:]) {
		return errors.New("phone signature verification failed")
	}
	if err := c.replay.Accept(response.Nonce, response.Counter, time.Now()); err != nil {
		return err
	}
	c.state.RecordProof(time.Now())
	if locked {
		return c.authorizer.Grant(sessionID, time.Now())
	}
	return nil
}

func (c *Coordinator) startPairing(now time.Time) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	pcID, err := decodeID(c.config.PCID)
	if err != nil {
		return "", err
	}
	publicKey, err := c.signer.PublicKey()
	if err != nil {
		return "", err
	}
	publicBytes, err := windowskey.MarshalPublic(publicKey)
	if err != nil {
		return "", err
	}
	secretBytes, err := protocol.RandomBytes(32)
	if err != nil {
		return "", err
	}
	pairing := &pendingPairing{expiresAt: now.Add(2 * time.Minute)}
	copy(pairing.secret[:], secretBytes)
	protocol.Zero(secretBytes)
	if c.pairing != nil {
		protocol.Zero(c.pairing.secret[:])
	}
	c.pairing = pairing
	uri := protocol.PairingURI{PCID: pcID, PCPublicKey: publicBytes, PairingSecret: pairing.secret, ExpiresAt: pairing.expiresAt}
	return uri.String(), nil
}

func (c *Coordinator) completePairing(ctx context.Context, candidate ble.Candidate, pcID [protocol.DeviceIDSize]byte, pending *pendingPairing) {
	challenge, err := protocol.NewPairingChallenge(pcID, pending.secret[:])
	if err != nil {
		return
	}
	exchangeCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	wire, err := c.transport.Exchange(exchangeCtx, candidate, protocol.MessagePairing, challenge.MarshalBinary())
	if err != nil {
		return
	}
	response, err := protocol.ParsePairingResponse(wire, pending.secret[:], challenge)
	if err != nil {
		return
	}
	if time.Now().After(pending.expiresAt) {
		return
	}
	if _, err := windowskey.ParsePublic(response.StrictPublicKey[:]); err != nil {
		return
	}
	if _, err := windowskey.ParsePublic(response.RelaxedPublicKey[:]); err != nil {
		return
	}
	presenceKey, err := protocol.DerivePresenceKey(pending.secret[:], pcID, response.PhoneID)
	if err != nil {
		return
	}
	secretIDBytes, _ := protocol.RandomBytes(16)
	secretID := base64.RawURLEncoding.EncodeToString(secretIDBytes)
	protocol.Zero(secretIDBytes)
	if err := c.secrets.Put(secret.PresenceName(secretID), presenceKey); err != nil {
		protocol.Zero(presenceKey)
		return
	}
	protocol.Zero(presenceKey)
	err = c.updateConfig(func(cfg *config.Config) {
		cfg.PhoneID = base64.RawURLEncoding.EncodeToString(response.PhoneID[:])
		cfg.PhoneStrictKey = base64.RawURLEncoding.EncodeToString(response.StrictPublicKey[:])
		cfg.PhoneRelaxedKey = base64.RawURLEncoding.EncodeToString(response.RelaxedPublicKey[:])
		cfg.PresenceSecretID = secretID
	})
	if err != nil {
		_ = c.secrets.Delete(secret.PresenceName(secretID))
		return
	}
	c.state.ResetDeviceEvidence()
	c.replay.ResetCounter(0)
	c.mu.Lock()
	if c.pairing == pending {
		protocol.Zero(c.pairing.secret[:])
		c.pairing = nil
	}
	c.mu.Unlock()
}

func (c *Coordinator) revoke() error {
	c.mu.Lock()
	secretID := c.config.PresenceSecretID
	c.mu.Unlock()
	if secretID != "" {
		if err := c.secrets.Delete(secret.PresenceName(secretID)); err != nil {
			return err
		}
	}
	c.authorizer.Cancel()
	c.replay.ResetCounter(0)
	err := c.updateConfig(func(cfg *config.Config) {
		cfg.PhoneID = ""
		cfg.PhoneStrictKey = ""
		cfg.PhoneRelaxedKey = ""
		cfg.PresenceSecretID = ""
	})
	if err == nil {
		c.state.ResetDeviceEvidence()
	}
	return err
}

func (c *Coordinator) updateConfig(mutator func(*config.Config)) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	next := c.config
	mutator(&next)
	if err := config.Save(c.configPath, next); err != nil {
		return err
	}
	c.config = next
	return nil
}

func decodeID(value string) ([protocol.DeviceIDSize]byte, error) {
	var result [protocol.DeviceIDSize]byte
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(decoded) != len(result) {
		return result, fmt.Errorf("invalid device identifier")
	}
	copy(result[:], decoded)
	return result, nil
}
