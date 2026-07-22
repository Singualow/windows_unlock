package ble

import (
	"context"
	"errors"
)

var ErrUnavailable = errors.New("BLE WinRT backend is unavailable")

type Candidate struct {
	Address     string
	RSSI        int
	ServiceData []byte
}

type Handler func(context.Context, Candidate)

type Transport interface {
	Start(context.Context, Handler) error
	Exchange(context.Context, Candidate, byte, []byte) ([]byte, error)
	Backend() string
	Close() error
}
