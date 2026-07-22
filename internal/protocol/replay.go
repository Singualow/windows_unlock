package protocol

import (
	"errors"
	"sync"
	"time"
)

var (
	ErrReplay          = errors.New("response was already used")
	ErrCounterRollback = errors.New("phone counter did not advance")
)

type ReplayGuard struct {
	mu          sync.Mutex
	lastCounter uint64
	used        map[[NonceSize]byte]time.Time
	retention   time.Duration
}

func NewReplayGuard(retention time.Duration) *ReplayGuard {
	return &ReplayGuard{used: make(map[[NonceSize]byte]time.Time), retention: retention}
}

func (g *ReplayGuard) Accept(nonce [NonceSize]byte, counter uint64, now time.Time) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	for n, expiry := range g.used {
		if !expiry.After(now) {
			delete(g.used, n)
		}
	}
	if _, exists := g.used[nonce]; exists {
		return ErrReplay
	}
	if counter <= g.lastCounter {
		return ErrCounterRollback
	}
	g.used[nonce] = now.Add(g.retention)
	g.lastCounter = counter
	return nil
}

func (g *ReplayGuard) ResetCounter(counter uint64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.lastCounter = counter
	g.used = make(map[[NonceSize]byte]time.Time)
}
