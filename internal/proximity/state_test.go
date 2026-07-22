package proximity

import (
	"testing"
	"time"
)

func near(s *State, at time.Time) {
	for i := 0; i < 3; i++ {
		s.ObserveRSSI(-55, at.Add(time.Duration(i)*time.Second))
	}
}

func TestManualLockRequiresAwayAndReturn(t *testing.T) {
	settings := DefaultSettings()
	s := NewState(settings)
	now := time.Unix(1_800_000_000, 0)
	near(s, now)
	s.OnLock(now.Add(3 * time.Second))
	if s.ShouldAttemptUnlock(now.Add(4 * time.Second)) {
		t.Fatal("manual lock was immediately reversed")
	}
	s.ObserveRSSI(-90, now.Add(5*time.Second))
	s.ObserveRSSI(-90, now.Add(16*time.Second))
	near(s, now.Add(17*time.Second))
	if !s.ShouldAttemptUnlock(now.Add(20 * time.Second)) {
		t.Fatal("return after away period did not arm unlock")
	}
}

func TestManualLockRecognizesAdvertisingSilence(t *testing.T) {
	s := NewState(DefaultSettings())
	now := time.Unix(1_800_000_000, 0)
	near(s, now)
	s.OnLock(now.Add(3 * time.Second))

	// No callbacks arrive while the phone is fully out of range. Its first
	// valid advertisement after returning must account for that absence.
	near(s, now.Add(15*time.Second))
	if !s.ShouldAttemptUnlock(now.Add(18 * time.Second)) {
		t.Fatal("return after an advertising gap did not arm unlock")
	}
}

func TestImmediateUnlockDoesNotRequireAwayPeriod(t *testing.T) {
	settings := DefaultSettings()
	settings.ImmediateUnlock = true
	s := NewState(settings)
	now := time.Unix(1_800_000_000, 0)
	near(s, now)
	s.OnLock(now.Add(3 * time.Second))
	if !s.ShouldAttemptUnlock(now.Add(3 * time.Second)) {
		t.Fatal("immediate mode retained the manual-lock hold")
	}
}

func TestEnablingImmediateUnlockClearsExistingManualHold(t *testing.T) {
	s := NewState(DefaultSettings())
	now := time.Unix(1_800_000_000, 0)
	near(s, now)
	s.OnLock(now.Add(3 * time.Second))
	s.SetImmediateUnlock(true)
	if !s.ShouldAttemptUnlock(now.Add(3 * time.Second)) {
		t.Fatal("enabling immediate mode did not clear the manual-lock hold")
	}
}

func TestResumeAllowsFreshChallenge(t *testing.T) {
	s := NewState(DefaultSettings())
	now := time.Unix(1_800_000_000, 0)
	near(s, now)
	s.RecordProof(now.Add(2 * time.Second))
	s.OnLock(now.Add(3 * time.Second))
	s.OnResume(now.Add(4 * time.Second))
	if !s.ShouldAttemptUnlock(now.Add(5 * time.Second)) {
		t.Fatal("resume window did not override manual hold")
	}
}

func TestFailureCooldown(t *testing.T) {
	s := NewState(DefaultSettings())
	now := time.Unix(1_800_000_000, 0)
	near(s, now)
	s.OnLock(now)
	s.OnResume(now)
	for i := 0; i < 3; i++ {
		s.RecordFailure(now.Add(time.Duration(i) * time.Second))
	}
	if s.ShouldAttemptUnlock(now.Add(10 * time.Second)) {
		t.Fatal("unlock allowed during cooldown")
	}
	if !s.CooldownUntil().Equal(now.Add(2*time.Second + 5*time.Minute)) {
		t.Fatal("unexpected cooldown deadline")
	}
}

func TestResetDeviceEvidenceClearsOldPhoneState(t *testing.T) {
	s := NewState(DefaultSettings())
	now := time.Unix(1_800_000_000, 0)
	near(s, now)
	s.RecordProof(now.Add(2 * time.Second))
	for i := 0; i < 3; i++ {
		s.RecordFailure(now.Add(time.Duration(i) * time.Second))
	}

	s.ResetDeviceEvidence()
	if _, ok := s.MedianRSSI(now.Add(3 * time.Second)); ok {
		t.Fatal("old phone RSSI survived reset")
	}
	if !s.LastProof().IsZero() {
		t.Fatal("old phone proof survived reset")
	}
	if !s.CooldownUntil().IsZero() || !s.CanAuthenticate(now.Add(3*time.Second)) {
		t.Fatal("old phone authentication cooldown survived reset")
	}
}

func TestAutoLockAfterProofTimeout(t *testing.T) {
	s := NewState(DefaultSettings())
	now := time.Unix(1_800_000_000, 0)
	s.RecordProof(now)
	if s.ShouldAutoLock(now.Add(10 * time.Second)) {
		t.Fatal("locked before proof timeout")
	}
	if !s.ShouldAutoLock(now.Add(21 * time.Second)) {
		t.Fatal("did not lock after proof timeout")
	}
}

func TestManualUnlockHasProofGracePeriod(t *testing.T) {
	s := NewState(DefaultSettings())
	now := time.Unix(1_800_000_000, 0)
	s.OnUnlock(now)
	if s.ShouldAutoLock(now.Add(10 * time.Second)) {
		t.Fatal("manual unlock was relocked before the proof grace period")
	}
	if !s.ShouldAutoLock(now.Add(21 * time.Second)) {
		t.Fatal("missing proof did not lock after the grace period")
	}
}
