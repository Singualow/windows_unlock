package authorize

import (
	"errors"
	"testing"
	"time"
)

func TestGrantIsSingleUse(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	signals := 0
	m := New(func() error { signals++; return nil })
	if err := m.Grant(7, now); err != nil || signals != 1 {
		t.Fatal("grant did not signal")
	}
	if grant, ok := m.Peek(now); !ok || grant.SessionID != 7 {
		t.Fatal("grant unavailable")
	}
	if _, err := m.Consume(now); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Consume(now); !errors.Is(err, ErrNoAuthorization) {
		t.Fatal("grant was reusable")
	}
}

func TestGrantExpires(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	m := New(nil)
	_ = m.Grant(1, now)
	if _, ok := m.Peek(now.Add(6 * time.Second)); ok {
		t.Fatal("expired grant remained valid")
	}
}

func TestSnapshotTracksCredentialProviderCallsWithoutConsumingGrant(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	m := New(func() error { return nil })
	if err := m.Grant(7, now); err != nil {
		t.Fatal(err)
	}
	before := m.Snapshot(now.Add(time.Second))
	if !before.Ready || !before.LastGrantedAt.Equal(now) || before.LastSignalAt.IsZero() {
		t.Fatalf("unexpected grant snapshot: %+v", before)
	}
	if _, ok := m.Peek(now.Add(2 * time.Second)); !ok {
		t.Fatal("peek failed")
	}
	if _, err := m.Consume(now.Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	after := m.Snapshot(now.Add(3 * time.Second))
	if after.Ready || after.LastPeekAt.IsZero() || after.LastConsumeAt.IsZero() {
		t.Fatalf("unexpected consumed snapshot: %+v", after)
	}
}
