package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateV0Defaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"target_sid":"S-1-5-21-test"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	want := Default()
	if got.Version != CurrentVersion || got.Mode != want.Mode || got.AutoLock != want.AutoLock || got.FailureCooldown != want.FailureCooldown || got.Thresholds.UnlockRSSI != want.Thresholds.UnlockRSSI || got.Thresholds.LockRSSI != want.Thresholds.LockRSSI || got.Thresholds.HighSensitivityRSSI != got.Thresholds.UnlockRSSI {
		t.Fatalf("v0 defaults were not migrated: %+v", got)
	}
}

func TestMigrateV1EnablesFailureCooldownByDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	legacy := `{"version":1,"mode":"strict","auto_lock":false,"thresholds":{"unlock_rssi":-65,"lock_rssi":-80,"unlock_window_ms":3000,"lock_window_ms":20000,"proof_timeout_ms":20000,"manual_hold_away_ms":10000}}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != CurrentVersion || !got.FailureCooldown || got.Thresholds.HighSensitivityRSSI != -65 {
		t.Fatalf("v1 cooldown default was not migrated: %+v", got)
	}
}

func TestMigrateV2PreservesPreviousHighSensitivityLockBoundary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	legacy := `{"version":2,"mode":"strict","auto_lock":true,"failure_cooldown_enabled":true,"thresholds":{"unlock_rssi":-58,"lock_rssi":-80,"unlock_window_ms":3000,"lock_window_ms":20000,"proof_timeout_ms":20000,"manual_hold_away_ms":10000}}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != CurrentVersion || got.Thresholds.HighSensitivityRSSI != -72 || got.Thresholds.HighSensitivityRSSI-8 != -80 {
		t.Fatalf("v2 high-sensitivity threshold was not migrated: %+v", got)
	}
}

func TestMigrateV0PreservesDisabledAutoLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"auto_lock":false}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.AutoLock {
		t.Fatal("explicitly disabled auto-lock was overwritten during migration")
	}
}

func TestSaveAtomicallyReplacesExistingConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	first := Default()
	if err := Save(path, first); err != nil {
		t.Fatal(err)
	}
	second := first
	second.AutoLock = false
	second.HighSensitivity = true
	second.Thresholds.HighSensitivityRSSI = -52
	second.ImmediateUnlock = true
	second.FailureCooldown = false
	if err := Save(path, second); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.AutoLock {
		t.Fatal("replacement config was not persisted")
	}
	if !got.HighSensitivity {
		t.Fatal("high-sensitivity preference was not persisted")
	}
	if got.Thresholds.HighSensitivityRSSI != -52 {
		t.Fatal("high-sensitivity threshold was not persisted")
	}
	if !got.ImmediateUnlock {
		t.Fatal("immediate-unlock preference was not persisted")
	}
	if got.FailureCooldown {
		t.Fatal("failure-cooldown preference was not persisted")
	}
}

func TestRejectsUnsafeManualThresholds(t *testing.T) {
	cfg := Default()
	cfg.Thresholds.UnlockRSSI = -65
	cfg.Thresholds.LockRSSI = -70
	if err := cfg.Validate(); err == nil {
		t.Fatal("thresholds with less than 8 dB hysteresis were accepted")
	}
}

func TestRejectsInvalidHighSensitivityThreshold(t *testing.T) {
	cfg := Default()
	cfg.Thresholds.HighSensitivityRSSI = -95
	if err := cfg.Validate(); err == nil {
		t.Fatal("out-of-range high-sensitivity threshold was accepted")
	}
}
