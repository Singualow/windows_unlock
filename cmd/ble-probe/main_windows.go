//go:build windows && ble

package main

import (
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"github.com/singu/proximity-unlock/internal/protocol"
	bt "tinygo.org/x/bluetooth"
)

func main() {
	adapter := bt.DefaultAdapter
	if err := adapter.Enable(); err != nil {
		fmt.Fprintln(os.Stderr, "BLE adapter:", err)
		os.Exit(1)
	}
	var allFrames, targetFrames atomic.Uint64
	var minimum, maximum atomic.Int64
	minimum.Store(127)
	maximum.Store(-127)
	timer := time.AfterFunc(20*time.Second, func() { _ = adapter.StopScan() })
	defer timer.Stop()
	err := adapter.Scan(func(_ *bt.Adapter, result bt.ScanResult) {
		for _, item := range result.ManufacturerData() {
			allFrames.Add(1)
			if item.CompanyID != protocol.AdvertisementCompanyID {
				continue
			}
			targetFrames.Add(1)
			rssi := int64(result.RSSI)
			for current := minimum.Load(); rssi < current && !minimum.CompareAndSwap(current, rssi); current = minimum.Load() {
			}
			for current := maximum.Load(); rssi > current && !maximum.CompareAndSwap(current, rssi); current = maximum.Load() {
			}
			fmt.Printf("target frame: length=%d rssi=%d\n", len(item.Data), result.RSSI)
		}
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "BLE scan:", err)
		os.Exit(1)
	}
	min, max := minimum.Load(), maximum.Load()
	if targetFrames.Load() == 0 {
		min, max = 0, 0
	}
	fmt.Printf("summary: all_manufacturer_frames=%d target_frames=%d rssi_min=%d rssi_max=%d\n", allFrames.Load(), targetFrames.Load(), min, max)
}
