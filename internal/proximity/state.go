package proximity

import (
	"sort"
	"sync"
	"time"
)

type Settings struct {
	UnlockRSSI          int
	LockRSSI            int
	HighSensitivity     bool
	UnlockWindow        time.Duration
	LockWindow          time.Duration
	ProofTimeout        time.Duration
	ManualHoldAway      time.Duration
	ImmediateUnlock     bool
	FailureCooldownOn   bool
	FailureWindow       time.Duration
	FailureLimit        int
	FailureCooldown     time.Duration
	ResumeUnlockWindow  time.Duration
	RequiredNearSamples int
}

func DefaultSettings() Settings {
	return Settings{
		UnlockRSSI:          -65,
		LockRSSI:            -80,
		UnlockWindow:        3 * time.Second,
		LockWindow:          20 * time.Second,
		ProofTimeout:        20 * time.Second,
		ManualHoldAway:      10 * time.Second,
		FailureCooldownOn:   true,
		FailureWindow:       time.Minute,
		FailureLimit:        3,
		FailureCooldown:     5 * time.Minute,
		ResumeUnlockWindow:  15 * time.Second,
		RequiredNearSamples: 3,
	}
}

type sample struct {
	rssi int
	at   time.Time
}

type State struct {
	mu sync.Mutex

	settings Settings
	samples  []sample

	locked           bool
	manualHold       bool
	awaySince        time.Time
	lastSeen         time.Time
	lowSince         time.Time
	lastProof        time.Time
	unlockedAt       time.Time
	resumeWindowEnds time.Time
	failures         []time.Time
	cooldownUntil    time.Time
}

func NewState(settings Settings) *State { return &State{settings: settings} }

func (s *State) UpdateSettings(settings Settings) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settings = settings
	if !settings.FailureCooldownOn {
		s.failures = nil
		s.cooldownUntil = time.Time{}
	}
	// Measurements collected under old thresholds must not immediately drive a
	// lock or unlock decision after calibration.
	s.samples = nil
	s.awaySince = time.Time{}
	s.lowSince = time.Time{}
}

// BeginProofGrace gives a newly selected timing profile one complete proof
// window before it can lock an already-unlocked session.
func (s *State) BeginProofGrace(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.locked {
		s.unlockedAt = now
	}
}

func (s *State) SetImmediateUnlock(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settings.ImmediateUnlock = enabled
	if enabled {
		s.manualHold = false
	}
}

// ResetDeviceEvidence removes all observations and authentication state tied
// to the previous phone. Session lock state is intentionally preserved.
func (s *State) ResetDeviceEvidence() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.samples = nil
	s.manualHold = false
	s.awaySince = time.Time{}
	s.lastSeen = time.Time{}
	s.lowSince = time.Time{}
	s.lastProof = time.Time{}
	s.resumeWindowEnds = time.Time{}
	s.failures = nil
	s.cooldownUntil = time.Time{}
}

func (s *State) ObserveRSSI(rssi int, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	previousSeen := s.lastSeen
	s.lastSeen = now
	// A valid advertisement after a sufficiently long silence proves that the
	// phone left before returning. This is needed because no callback is made
	// while a BLE peripheral is completely out of range.
	if s.manualHold && !previousSeen.IsZero() && now.Sub(previousSeen) >= s.settings.ManualHoldAway {
		s.manualHold = false
	}
	s.samples = append(s.samples, sample{rssi: rssi, at: now})
	if len(s.samples) > 5 {
		s.samples = s.samples[len(s.samples)-5:]
	}
	if rssi <= s.settings.LockRSSI {
		if s.lowSince.IsZero() {
			s.lowSince = now
		}
	} else {
		s.lowSince = time.Time{}
	}
	if rssi < s.settings.UnlockRSSI {
		if s.awaySince.IsZero() {
			s.awaySince = now
		}
	} else {
		s.awaySince = time.Time{}
	}
	if s.manualHold && !s.awaySince.IsZero() && now.Sub(s.awaySince) >= s.settings.ManualHoldAway {
		s.manualHold = false
	}
}

func (s *State) RecordProof(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastProof = now
	s.failures = nil
	s.cooldownUntil = time.Time{}
}

func (s *State) RecordFailure(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.settings.FailureCooldownOn {
		s.failures = nil
		s.cooldownUntil = time.Time{}
		return
	}
	cutoff := now.Add(-s.settings.FailureWindow)
	filtered := s.failures[:0]
	for _, failure := range s.failures {
		if failure.After(cutoff) {
			filtered = append(filtered, failure)
		}
	}
	s.failures = append(filtered, now)
	if len(s.failures) >= s.settings.FailureLimit {
		s.cooldownUntil = now.Add(s.settings.FailureCooldown)
	}
}

func (s *State) OnLock(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.locked = true
	// If the phone was already authenticated nearby, treat the lock as an
	// intentional/idle lock and do not instantly reverse it.
	s.manualHold = !s.settings.ImmediateUnlock && s.nearLocked(now)
}

func (s *State) OnUnlock(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.locked = false
	s.manualHold = false
	s.lowSince = time.Time{}
	s.unlockedAt = now
	s.resumeWindowEnds = time.Time{}
}

func (s *State) OnResume(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resumeWindowEnds = now.Add(s.settings.ResumeUnlockWindow)
	s.manualHold = false
}

func (s *State) ShouldAttemptUnlock(now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.locked || s.manualHold || (s.settings.FailureCooldownOn && now.Before(s.cooldownUntil)) || !s.nearLocked(now) {
		return false
	}
	return s.hasNearSamplesLocked(now) || now.Before(s.resumeWindowEnds)
}

func (s *State) ShouldAutoLock(now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.locked {
		return false
	}
	graceElapsed := s.unlockedAt.IsZero() || now.Sub(s.unlockedAt) >= s.settings.ProofTimeout
	proofMissing := graceElapsed && (s.lastProof.IsZero() || now.Sub(s.lastProof) >= s.settings.ProofTimeout)
	lowLongEnough := !s.lowSince.IsZero() && now.Sub(s.lowSince) >= s.settings.LockWindow
	return proofMissing || lowLongEnough
}

func (s *State) CanAuthenticate(now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.settings.FailureCooldownOn || !now.Before(s.cooldownUntil)
}

func (s *State) MedianRSSI(now time.Time) (int, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.medianLocked(now)
}

func (s *State) CooldownUntil() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cooldownUntil
}

func (s *State) LastProof() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastProof
}

func (s *State) nearLocked(now time.Time) bool {
	if s.settings.HighSensitivity {
		cutoff := now.Add(-s.settings.UnlockWindow)
		for index := len(s.samples) - 1; index >= 0; index-- {
			sample := s.samples[index]
			if sample.at.Before(cutoff) {
				break
			}
			return sample.rssi >= s.settings.UnlockRSSI
		}
		return false
	}
	median, ok := s.medianLocked(now)
	return ok && median >= s.settings.UnlockRSSI
}

func (s *State) hasNearSamplesLocked(now time.Time) bool {
	cutoff := now.Add(-s.settings.UnlockWindow)
	count := 0
	for _, sample := range s.samples {
		if !sample.at.Before(cutoff) && sample.rssi >= s.settings.UnlockRSSI {
			count++
		}
	}
	return count >= s.settings.RequiredNearSamples
}

func (s *State) medianLocked(now time.Time) (int, bool) {
	cutoff := now.Add(-s.settings.ProofTimeout)
	values := make([]int, 0, len(s.samples))
	for _, sample := range s.samples {
		if !sample.at.Before(cutoff) {
			values = append(values, sample.rssi)
		}
	}
	if len(values) == 0 {
		return 0, false
	}
	sort.Ints(values)
	return values[len(values)/2], true
}
