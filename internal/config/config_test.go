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
	if got.Version != CurrentVersion || got.Mode != want.Mode || got.AutoLock != want.AutoLock || got.FailureCooldown != want.FailureCooldown || got.Thresholds != want.Thresholds {
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
	if got.Version != CurrentVersion || !got.FailureCooldown {
		t.Fatalf("v1 cooldown default was not migrated: %+v", got)
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
	if !got.ImmediateUnlock {
		t.Fatal("immediate-unlock preference was not persisted")
	}
	if got.FailureCooldown {
		t.Fatal("failure-cooldown preference was not persisted")
	}
}
