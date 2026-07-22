//go:build windows && ble

package ble

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/singu/proximity-unlock/internal/protocol"
	bt "tinygo.org/x/bluetooth"
)

type WinRT struct {
	adapter  *bt.Adapter
	service  bt.UUID
	write    bt.UUID
	response bt.UUID
	pairing  bt.UUID

	cancel    context.CancelFunc
	closeOnce sync.Once
	exchange  sync.Mutex
	paused    atomic.Bool
}

func New() Transport {
	service, _ := bt.ParseUUID(protocol.ServiceUUID)
	write, _ := bt.ParseUUID(protocol.ChallengeUUID)
	response, _ := bt.ParseUUID(protocol.ResponseUUID)
	pairing, _ := bt.ParseUUID(protocol.PairingUUID)
	return &WinRT{adapter: bt.DefaultAdapter, service: service, write: write, response: response, pairing: pairing}
}

func (w *WinRT) Backend() string { return "tinygo.org/x/bluetooth WinRT" }

func (w *WinRT) Start(parent context.Context, handler Handler) error {
	if err := w.adapter.Enable(); err != nil {
		return fmt.Errorf("enable WinRT BLE adapter: %w", err)
	}
	ctx, cancel := context.WithCancel(parent)
	w.cancel = cancel
	go w.scanLoop(ctx, handler)
	return nil
}

func (w *WinRT) scanLoop(ctx context.Context, handler Handler) {
	for ctx.Err() == nil {
		if w.paused.Load() {
			select {
			case <-ctx.Done():
				return
			case <-time.After(100 * time.Millisecond):
				continue
			}
		}
		err := w.adapter.Scan(func(_ *bt.Adapter, result bt.ScanResult) {
			if ctx.Err() != nil || w.paused.Load() {
				return
			}
			for _, element := range result.ManufacturerData() {
				if element.CompanyID != protocol.AdvertisementCompanyID || len(element.Data) != protocol.AdvertisementSize {
					continue
				}
				candidate := Candidate{
					Address:     result.Address.String(),
					RSSI:        int(result.RSSI),
					ServiceData: append([]byte(nil), element.Data...),
				}
				go handler(ctx, candidate)
				return
			}
		})
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			time.Sleep(time.Second)
		}
	}
}

func (w *WinRT) Exchange(ctx context.Context, candidate Candidate, messageType byte, payload []byte) ([]byte, error) {
	w.exchange.Lock()
	defer w.exchange.Unlock()
	w.paused.Store(true)
	_ = w.adapter.StopScan()
	defer w.paused.Store(false)

	var address bt.Address
	address.Set(candidate.Address)
	device, err := w.adapter.Connect(address, bt.ConnectionParams{})
	if err != nil {
		return nil, fmt.Errorf("connect %s: %w", candidate.Address, err)
	}
	defer device.Disconnect()
	services, err := device.DiscoverServices([]bt.UUID{w.service})
	if err != nil || len(services) != 1 {
		return nil, fmt.Errorf("discover Proximity Unlock service: %w", normalizeDiscoveryError(err))
	}
	characteristics, err := services[0].DiscoverCharacteristics([]bt.UUID{w.write, w.response, w.pairing})
	if err != nil {
		return nil, fmt.Errorf("discover characteristics: %w", err)
	}
	var writeCharacteristic, responseCharacteristic bt.DeviceCharacteristic
	var haveWrite, haveResponse bool
	for _, characteristic := range characteristics {
		switch messageType {
		case protocol.MessageChallenge:
			if characteristic.UUID() == w.write {
				writeCharacteristic, haveWrite = characteristic, true
			}
			if characteristic.UUID() == w.response {
				responseCharacteristic, haveResponse = characteristic, true
			}
		case protocol.MessagePairing:
			if characteristic.UUID() == w.pairing {
				writeCharacteristic, responseCharacteristic = characteristic, characteristic
				haveWrite, haveResponse = true, true
			}
		default:
			return nil, errors.New("unsupported BLE message type")
		}
	}
	if !haveWrite || !haveResponse {
		return nil, errors.New("required GATT characteristics are missing")
	}

	result := make(chan []byte, 1)
	errorsCh := make(chan error, 1)
	var reassembler protocol.Reassembler
	if err := responseCharacteristic.EnableNotifications(func(fragment []byte) {
		assembled, complete, err := reassembler.Add(append([]byte(nil), fragment...))
		if err != nil {
			select {
			case errorsCh <- err:
			default:
			}
			return
		}
		if complete {
			select {
			case result <- assembled:
			default:
			}
		}
	}); err != nil {
		return nil, fmt.Errorf("enable response notifications: %w", err)
	}
	mtu, err := writeCharacteristic.GetMTU()
	if err != nil || mtu < 23 {
		mtu = 185
	}
	mtuPayload := int(mtu) - 3
	if mtuPayload > 180 {
		mtuPayload = 180
	}
	fragments, err := protocol.Fragment(messageType, payload, mtuPayload)
	if err != nil {
		return nil, err
	}
	for _, fragment := range fragments {
		if _, err := writeCharacteristic.Write(fragment); err != nil {
			return nil, fmt.Errorf("write GATT fragment: %w", err)
		}
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case err := <-errorsCh:
		return nil, err
	case response := <-result:
		return response, nil
	}
}

func (w *WinRT) Close() error {
	w.closeOnce.Do(func() {
		if w.cancel != nil {
			w.cancel()
		}
		_ = w.adapter.StopScan()
	})
	return nil
}

func normalizeDiscoveryError(err error) error {
	if err == nil {
		return errors.New("service not found")
	}
	return err
}
