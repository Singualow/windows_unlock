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
	if got.Version != CurrentVersion || got.Mode != want.Mode || got.AutoLock != want.AutoLock || got.Thresholds != want.Thresholds {
		t.Fatalf("v0 defaults were not migrated: %+v", got)
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
}
