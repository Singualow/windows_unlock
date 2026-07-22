package proximity

import (
	"math"
	"sort"
	"sync"
	"time"
)

type Settings struct {
	UnlockRSSI          int
	LockRSSI            int
	HighSensitivity     bool
	DopplerPrediction   bool
	DopplerSensitivity  int
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

	settings      Settings
	samples       []sample
	motionSamples []sample

	locked           bool
	manualHold       bool
	awaySince        time.Time
	lastSeen         time.Time
	lowSince         time.Time
	lastProof        time.Time
	authFailureSince time.Time
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
	s.motionSamples = nil
	s.awaySince = time.Time{}
	s.lowSince = time.Time{}
	s.authFailureSince = time.Time{}
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
	s.motionSamples = nil
	s.manualHold = false
	s.awaySince = time.Time{}
	s.lastSeen = time.Time{}
	s.lowSince = time.Time{}
	s.lastProof = time.Time{}
	s.authFailureSince = time.Time{}
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
	s.motionSamples = append(s.motionSamples, sample{rssi: rssi, at: now})
	if len(s.motionSamples) > 12 {
		s.motionSamples = s.motionSamples[len(s.motionSamples)-12:]
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
	s.authFailureSince = time.Time{}
	s.failures = nil
	s.cooldownUntil = time.Time{}
}

// RecordAttemptFailure starts a continuous authentication-failure window.
// Repeated failures keep the original start time; a valid proof clears it.
func (s *State) RecordAttemptFailure(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.authFailureSince.IsZero() {
		s.authFailureSince = now
	}
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
	s.authFailureSince = time.Time{}
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
	if !s.locked || s.manualHold || (s.settings.FailureCooldownOn && now.Before(s.cooldownUntil)) {
		return false
	}
	nearBySignal := s.nearLocked(now)
	near := nearBySignal && s.hasNearSamplesLocked(now)
	predicted := s.predictionLocked(now).Ready
	return near || predicted || (nearBySignal && now.Before(s.resumeWindowEnds))
}

func (s *State) ShouldAutoLock(now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.locked {
		return false
	}
	graceElapsed := s.unlockedAt.IsZero() || now.Sub(s.unlockedAt) >= s.settings.ProofTimeout
	proofMissing := graceElapsed && (s.lastProof.IsZero() || now.Sub(s.lastProof) >= s.settings.ProofTimeout)
	if (s.settings.HighSensitivity || s.settings.DopplerPrediction) && !s.authFailureSince.IsZero() {
		// An explicit failure gets a complete continuous failure window of its
		// own instead of inheriting time already elapsed since the last proof.
		proofMissing = now.Sub(s.authFailureSince) >= s.settings.ProofTimeout
	}
	lowLongEnough := !s.lowSince.IsZero() && now.Sub(s.lowSince) >= s.settings.LockWindow
	return proofMissing || lowLongEnough
}

type MotionPrediction struct {
	ProjectedRSSI int
	SlopeDBPerSec float64
	Ready         bool
}

func (s *State) Prediction(now time.Time) MotionPrediction {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.predictionLocked(now)
}

func (s *State) predictionLocked(now time.Time) MotionPrediction {
	if !s.settings.DopplerPrediction {
		return MotionPrediction{}
	}
	cutoff := now.Add(-3500 * time.Millisecond)
	values := make([]sample, 0, len(s.motionSamples))
	for _, value := range s.motionSamples {
		if !value.at.Before(cutoff) && !value.at.After(now) {
			values = append(values, value)
		}
	}
	if len(values) < 3 {
		return MotionPrediction{}
	}
	span := values[len(values)-1].at.Sub(values[0].at).Seconds()
	if span < 0.6 {
		return MotionPrediction{}
	}
	origin := values[0].at
	var sumTime, sumRSSI float64
	for _, value := range values {
		sumTime += value.at.Sub(origin).Seconds()
		sumRSSI += float64(value.rssi)
	}
	meanTime := sumTime / float64(len(values))
	meanRSSI := sumRSSI / float64(len(values))
	var numerator, denominator float64
	for _, value := range values {
		t := value.at.Sub(origin).Seconds() - meanTime
		numerator += t * (float64(value.rssi) - meanRSSI)
		denominator += t * t
	}
	if denominator == 0 {
		return MotionPrediction{}
	}
	slope := numerator / denominator
	sensitivity := math.Max(1, math.Min(100, float64(s.settings.DopplerSensitivity)))
	ratio := (sensitivity - 1) / 99
	horizon := 0.75 + 2.25*ratio
	minimumSlope := 3.0 - 2.4*ratio
	pretriggerMargin := 4.0 + 10.0*ratio
	latest := values[len(values)-1].rssi
	projected := int(math.Round(float64(latest) + slope*horizon))
	ready := latest < s.settings.UnlockRSSI &&
		float64(latest) >= float64(s.settings.UnlockRSSI)-pretriggerMargin &&
		slope >= minimumSlope && projected >= s.settings.UnlockRSSI
	return MotionPrediction{ProjectedRSSI: projected, SlopeDBPerSec: slope, Ready: ready}
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
