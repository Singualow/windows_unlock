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

func TestFailureCooldownCanBeDisabledAndClearsActiveCooldown(t *testing.T) {
	settings := DefaultSettings()
	s := NewState(settings)
	now := time.Unix(1_800_000_000, 0)
	for i := 0; i < 3; i++ {
		s.RecordFailure(now.Add(time.Duration(i) * time.Second))
	}
	if s.CooldownUntil().IsZero() {
		t.Fatal("test did not create a cooldown")
	}
	settings.FailureCooldownOn = false
	s.UpdateSettings(settings)
	if !s.CooldownUntil().IsZero() || !s.CanAuthenticate(now.Add(3*time.Second)) {
		t.Fatal("disabling cooldown did not immediately restore authentication")
	}
	for i := 0; i < 5; i++ {
		s.RecordFailure(now.Add(time.Duration(i+4) * time.Second))
	}
	if !s.CooldownUntil().IsZero() {
		t.Fatal("disabled cooldown still accumulated failures")
	}
}

func TestSuccessfulProofClearsExpiredFailureState(t *testing.T) {
	s := NewState(DefaultSettings())
	now := time.Unix(1_800_000_000, 0)
	for i := 0; i < 3; i++ {
		s.RecordFailure(now.Add(time.Duration(i) * time.Second))
	}
	s.RecordProof(now.Add(6 * time.Minute))
	if !s.CooldownUntil().IsZero() {
		t.Fatal("successful proof did not clear cooldown state")
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

func TestHighSensitivityLocksAfterTenSecondProofTimeout(t *testing.T) {
	settings := DefaultSettings()
	settings.HighSensitivity = true
	settings.ProofTimeout = 10 * time.Second
	s := NewState(settings)
	now := time.Unix(1_800_000_000, 0)
	s.RecordProof(now)
	if s.ShouldAutoLock(now.Add(9 * time.Second)) {
		t.Fatal("high-sensitivity mode locked before its proof timeout")
	}
	if !s.ShouldAutoLock(now.Add(10*time.Second + time.Millisecond)) {
		t.Fatal("high-sensitivity mode did not lock after its proof timeout")
	}
}

func TestHighSensitivityFailureGetsFullTenSecondRecoveryWindow(t *testing.T) {
	settings := DefaultSettings()
	settings.HighSensitivity = true
	settings.ProofTimeout = 10 * time.Second
	s := NewState(settings)
	now := time.Unix(1_800_000_000, 0)
	s.RecordProof(now)
	failureAt := now.Add(30 * time.Second)
	s.RecordAttemptFailure(failureAt)
	if s.ShouldAutoLock(failureAt.Add(9 * time.Second)) {
		t.Fatal("high-sensitivity mode locked before authentication had failed continuously for ten seconds")
	}
	if !s.ShouldAutoLock(failureAt.Add(10*time.Second + time.Millisecond)) {
		t.Fatal("high-sensitivity mode did not lock after ten seconds of continuous authentication failure")
	}
}

func TestHighSensitivitySuccessfulProofClearsFailureWindow(t *testing.T) {
	settings := DefaultSettings()
	settings.HighSensitivity = true
	settings.ProofTimeout = 10 * time.Second
	s := NewState(settings)
	now := time.Unix(1_800_000_000, 0)
	s.RecordAttemptFailure(now)
	s.RecordProof(now.Add(7 * time.Second))
	if s.ShouldAutoLock(now.Add(12 * time.Second)) {
		t.Fatal("successful proof did not clear the authentication failure window")
	}
}

func TestHighSensitivityLowSignalMustPersistForTenSeconds(t *testing.T) {
	settings := DefaultSettings()
	settings.HighSensitivity = true
	settings.LockWindow = 10 * time.Second
	s := NewState(settings)
	now := time.Unix(1_800_000_000, 0)
	s.RecordProof(now.Add(5 * time.Second))
	s.ObserveRSSI(settings.LockRSSI-1, now)
	if s.ShouldAutoLock(now.Add(9 * time.Second)) {
		t.Fatal("high-sensitivity mode locked before low signal persisted for ten seconds")
	}
	if !s.ShouldAutoLock(now.Add(10*time.Second + time.Millisecond)) {
		t.Fatal("high-sensitivity mode did not lock after low signal persisted for ten seconds")
	}
}

func TestHighSensitivityUsesLatestNearSample(t *testing.T) {
	settings := DefaultSettings()
	settings.HighSensitivity = true
	settings.UnlockWindow = 1500 * time.Millisecond
	settings.ProofTimeout = 10 * time.Second
	settings.RequiredNearSamples = 1
	s := NewState(settings)
	now := time.Unix(1_800_000_000, 0)
	for index := 0; index < 4; index++ {
		s.ObserveRSSI(-90, now.Add(time.Duration(index)*200*time.Millisecond))
	}
	s.OnLock(now.Add(time.Second))
	s.ObserveRSSI(-50, now.Add(1200*time.Millisecond))
	if !s.ShouldAttemptUnlock(now.Add(1300 * time.Millisecond)) {
		t.Fatal("fresh near sample did not immediately arm high-sensitivity unlock")
	}
}

func TestSensitivityTransitionStartsFreshProofGrace(t *testing.T) {
	settings := DefaultSettings()
	s := NewState(settings)
	now := time.Unix(1_800_000_000, 0)
	s.OnUnlock(now)
	settings.HighSensitivity = true
	settings.ProofTimeout = 10 * time.Second
	s.UpdateSettings(settings)
	s.BeginProofGrace(now.Add(time.Minute))
	if s.ShouldAutoLock(now.Add(time.Minute + 3*time.Second)) {
		t.Fatal("sensitivity transition ignored its fresh proof grace")
	}
}

func TestDopplerPredictionArmsEarlyChallengeForApproachingPhone(t *testing.T) {
	settings := DefaultSettings()
	settings.DopplerPrediction = true
	settings.DopplerSensitivity = 60
	settings.ImmediateUnlock = true
	settings.UnlockRSSI = -60
	s := NewState(settings)
	now := time.Unix(1_800_000_000, 0)
	s.OnLock(now)
	s.ObserveRSSI(-72, now)
	s.ObserveRSSI(-68, now.Add(time.Second))
	s.ObserveRSSI(-64, now.Add(2*time.Second))
	prediction := s.Prediction(now.Add(2 * time.Second))
	if !prediction.Ready || prediction.ProjectedRSSI < settings.UnlockRSSI {
		t.Fatalf("approach was not predicted: %+v", prediction)
	}
	if !s.ShouldAttemptUnlock(now.Add(2 * time.Second)) {
		t.Fatal("prediction did not arm an early authentication challenge")
	}
}

func TestDopplerPredictionRejectsStableOrRecedingSignal(t *testing.T) {
	settings := DefaultSettings()
	settings.DopplerPrediction = true
	settings.DopplerSensitivity = 100
	settings.ImmediateUnlock = true
	settings.UnlockRSSI = -60
	for name, values := range map[string][]int{
		"stable":   {-68, -67, -68, -67},
		"receding": {-64, -67, -70, -73},
	} {
		t.Run(name, func(t *testing.T) {
			s := NewState(settings)
			now := time.Unix(1_800_000_000, 0)
			s.OnLock(now)
			for index, value := range values {
				s.ObserveRSSI(value, now.Add(time.Duration(index)*time.Second))
			}
			if prediction := s.Prediction(now.Add(3 * time.Second)); prediction.Ready {
				t.Fatalf("%s signal triggered prediction: %+v", name, prediction)
			}
		})
	}
}

func TestDopplerSensitivityControlsPredictionTrigger(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	readyAt := func(sensitivity int) bool {
		settings := DefaultSettings()
		settings.DopplerPrediction = true
		settings.DopplerSensitivity = sensitivity
		settings.UnlockRSSI = -65
		s := NewState(settings)
		s.ObserveRSSI(-69, now)
		s.ObserveRSSI(-68, now.Add(time.Second))
		s.ObserveRSSI(-67, now.Add(2*time.Second))
		return s.Prediction(now.Add(2 * time.Second)).Ready
	}
	if readyAt(1) {
		t.Fatal("minimum sensitivity predicted a weak trend")
	}
	if !readyAt(100) {
		t.Fatal("maximum sensitivity did not predict a valid weak approach")
	}
}

func TestDopplerModeRelocksAfterTenSecondsWithoutProof(t *testing.T) {
	settings := DefaultSettings()
	settings.DopplerPrediction = true
	settings.ProofTimeout = 10 * time.Second
	s := NewState(settings)
	now := time.Unix(1_800_000_000, 0)
	s.OnUnlock(now)
	s.RecordProof(now)
	s.RecordAttemptFailure(now.Add(20 * time.Second))
	if s.ShouldAutoLock(now.Add(29 * time.Second)) {
		t.Fatal("doppler mode relocked before the ten-second revalidation window")
	}
	if !s.ShouldAutoLock(now.Add(30*time.Second + time.Millisecond)) {
		t.Fatal("doppler mode did not relock after ten seconds without a valid proof")
	}
}
