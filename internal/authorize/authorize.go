package authorize

import (
	"errors"
	"sync"
	"time"
)

var ErrNoAuthorization = errors.New("no current unlock authorization")

type Grant struct {
	SessionID uint32
	CreatedAt time.Time
	ExpiresAt time.Time
}

type Manager struct {
	mu              sync.Mutex
	grant           *Grant
	disabled        bool
	signal          func() error
	lastGrantedAt   time.Time
	lastSignalAt    time.Time
	lastSignalError string
	lastPeekAt      time.Time
	lastConsumeAt   time.Time
}

type Snapshot struct {
	Ready           bool      `json:"ready"`
	ExpiresAt       time.Time `json:"expires_at,omitempty"`
	LastGrantedAt   time.Time `json:"last_granted_at,omitempty"`
	LastSignalAt    time.Time `json:"last_signal_at,omitempty"`
	LastSignalError string    `json:"last_signal_error,omitempty"`
	LastPeekAt      time.Time `json:"last_peek_at,omitempty"`
	LastConsumeAt   time.Time `json:"last_consume_at,omitempty"`
}

func New(signal func() error) *Manager { return &Manager{signal: signal} }

func (m *Manager) Grant(sessionID uint32, now time.Time) error {
	m.mu.Lock()
	if m.disabled {
		m.mu.Unlock()
		return errors.New("credential authorization is disabled")
	}
	m.grant = &Grant{SessionID: sessionID, CreatedAt: now, ExpiresAt: now.Add(5 * time.Second)}
	m.lastGrantedAt = now
	m.mu.Unlock()
	if m.signal != nil {
		err := m.signal()
		m.mu.Lock()
		m.lastSignalAt = time.Now()
		m.lastSignalError = ""
		if err != nil {
			m.lastSignalError = err.Error()
		}
		m.mu.Unlock()
		return err
	}
	return nil
}

func (m *Manager) Peek(now time.Time) (Grant, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastPeekAt = now
	if m.grant == nil || m.disabled || now.After(m.grant.ExpiresAt) {
		m.grant = nil
		return Grant{}, false
	}
	return *m.grant, true
}

func (m *Manager) Consume(now time.Time) (Grant, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastConsumeAt = now
	if m.grant == nil || m.disabled || now.After(m.grant.ExpiresAt) {
		m.grant = nil
		return Grant{}, ErrNoAuthorization
	}
	grant := *m.grant
	m.grant = nil
	return grant, nil
}

func (m *Manager) Snapshot(now time.Time) Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := Snapshot{
		LastGrantedAt:   m.lastGrantedAt,
		LastSignalAt:    m.lastSignalAt,
		LastSignalError: m.lastSignalError,
		LastPeekAt:      m.lastPeekAt,
		LastConsumeAt:   m.lastConsumeAt,
	}
	if m.grant != nil && !m.disabled && !now.After(m.grant.ExpiresAt) {
		result.Ready = true
		result.ExpiresAt = m.grant.ExpiresAt
	}
	return result
}

func (m *Manager) Cancel() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.grant = nil
}

func (m *Manager) Disable() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.disabled = true
	m.grant = nil
}

func (m *Manager) Enable() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.disabled = false
}
