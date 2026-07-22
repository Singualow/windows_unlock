//go:build !ble

package ble

import "context"

type Stub struct{}

func New() Transport { return &Stub{} }

func (s *Stub) Start(ctx context.Context, handler Handler) error { return nil }
func (s *Stub) Exchange(context.Context, Candidate, byte, []byte) ([]byte, error) {
	return nil, ErrUnavailable
}
func (s *Stub) Backend() string { return "disabled (rebuild with -tags ble)" }
func (s *Stub) Close() error    { return nil }
