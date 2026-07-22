package config

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/singu/proximity-unlock/internal/protocol"
)

const CurrentVersion = 1

type Thresholds struct {
	UnlockRSSI       int `json:"unlock_rssi"`
	LockRSSI         int `json:"lock_rssi"`
	UnlockWindowMS   int `json:"unlock_window_ms"`
	LockWindowMS     int `json:"lock_window_ms"`
	ProofTimeoutMS   int `json:"proof_timeout_ms"`
	ManualHoldAwayMS int `json:"manual_hold_away_ms"`
}

type Config struct {
	Version          int        `json:"version"`
	TargetSID        string     `json:"target_sid"`
	CanonicalUser    string     `json:"canonical_user"`
	PCID             string     `json:"pc_id"`
	PhoneID          string     `json:"phone_id,omitempty"`
	PhoneStrictKey   string     `json:"phone_strict_public_key,omitempty"`
	PhoneRelaxedKey  string     `json:"phone_relaxed_public_key,omitempty"`
	PresenceSecretID string     `json:"presence_secret_id,omitempty"`
	Mode             string     `json:"mode"`
	AutoLock         bool       `json:"auto_lock"`
	ImmediateUnlock  bool       `json:"immediate_unlock"`
	PausedUntil      *time.Time `json:"paused_until,omitempty"`
	CredentialValid  bool       `json:"credential_valid"`
	Thresholds       Thresholds `json:"thresholds"`
}

func Default() Config {
	return Config{
		Version:  CurrentVersion,
		Mode:     protocol.ModeStrict.String(),
		AutoLock: true,
		Thresholds: Thresholds{
			UnlockRSSI:       -65,
			LockRSSI:         -80,
			UnlockWindowMS:   3000,
			LockWindowMS:     20000,
			ProofTimeoutMS:   20000,
			ManualHoldAwayMS: 10000,
		},
	}
}

func (c Config) Validate() error {
	if c.Version != CurrentVersion {
		return fmt.Errorf("unsupported config version %d", c.Version)
	}
	if _, err := protocol.ParseMode(c.Mode); err != nil {
		return err
	}
	if c.Thresholds.UnlockRSSI <= c.Thresholds.LockRSSI || c.Thresholds.UnlockRSSI > -20 || c.Thresholds.LockRSSI < -120 {
		return errors.New("invalid RSSI thresholds or hysteresis")
	}
	if c.Thresholds.UnlockWindowMS < 1000 || c.Thresholds.LockWindowMS < 5000 || c.Thresholds.ProofTimeoutMS < 5000 || c.Thresholds.ManualHoldAwayMS < 5000 {
		return errors.New("unsafe timing threshold")
	}
	if c.PCID != "" {
		decoded, err := base64.RawURLEncoding.DecodeString(c.PCID)
		if err != nil || len(decoded) != protocol.DeviceIDSize {
			return errors.New("invalid PC identifier")
		}
	}
	pairedFields := []string{c.PhoneID, c.PhoneStrictKey, c.PhoneRelaxedKey, c.PresenceSecretID}
	pairedCount := 0
	for _, value := range pairedFields {
		if value != "" {
			pairedCount++
		}
	}
	if pairedCount != 0 && pairedCount != len(pairedFields) {
		return errors.New("incomplete phone pairing configuration")
	}
	if pairedCount == len(pairedFields) {
		phoneID, err := base64.RawURLEncoding.DecodeString(c.PhoneID)
		if err != nil || len(phoneID) != protocol.DeviceIDSize {
			return errors.New("invalid phone identifier")
		}
		for _, encoded := range []string{c.PhoneStrictKey, c.PhoneRelaxedKey} {
			publicKey, err := base64.RawURLEncoding.DecodeString(encoded)
			if err != nil || len(publicKey) != 65 || publicKey[0] != 4 {
				return errors.New("invalid phone public key")
			}
		}
	}
	return nil
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Default(), nil
	}
	if err != nil {
		return Config{}, err
	}
	var result Config
	if err := json.Unmarshal(data, &result); err != nil {
		return Config{}, err
	}
	if result.Version == 0 {
		var raw map[string]json.RawMessage
		_ = json.Unmarshal(data, &raw)
		result = migrateV0(result, raw)
	}
	if err := result.Validate(); err != nil {
		return Config{}, err
	}
	return result, nil
}

func migrateV0(value Config, raw map[string]json.RawMessage) Config {
	defaults := Default()
	value.Version = CurrentVersion
	if value.Mode == "" {
		value.Mode = defaults.Mode
	}
	if _, present := raw["auto_lock"]; !present {
		value.AutoLock = defaults.AutoLock
	}
	if value.Thresholds.UnlockRSSI == 0 {
		value.Thresholds.UnlockRSSI = defaults.Thresholds.UnlockRSSI
	}
	if value.Thresholds.LockRSSI == 0 {
		value.Thresholds.LockRSSI = defaults.Thresholds.LockRSSI
	}
	if value.Thresholds.UnlockWindowMS == 0 {
		value.Thresholds.UnlockWindowMS = defaults.Thresholds.UnlockWindowMS
	}
	if value.Thresholds.LockWindowMS == 0 {
		value.Thresholds.LockWindowMS = defaults.Thresholds.LockWindowMS
	}
	if value.Thresholds.ProofTimeoutMS == 0 {
		value.Thresholds.ProofTimeoutMS = defaults.Thresholds.ProofTimeoutMS
	}
	if value.Thresholds.ManualHoldAwayMS == 0 {
		value.Thresholds.ManualHoldAwayMS = defaults.Thresholds.ManualHoldAwayMS
	}
	return value
}

func Save(path string, value Config) error {
	if err := value.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".config-*.tmp")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempName, path)
}

func ProgramDataPath() string {
	base := os.Getenv("ProgramData")
	if base == "" {
		base = `C:\ProgramData`
	}
	return filepath.Join(base, "ProximityUnlock", "config.json")
}
