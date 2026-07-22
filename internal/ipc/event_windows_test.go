//go:build windows

package ipc

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"golang.org/x/sys/windows"
)

func TestSignalCredentialEventWhenEventAlreadyExists(t *testing.T) {
	eventName := fmt.Sprintf(`Local\ProximityUnlock.EventTest.%d`, os.Getpid())
	name, err := windows.UTF16PtrFromString(eventName)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateEvent(nil, 0, 0, name)
	if err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		t.Fatal(err)
	}
	if handle == 0 {
		t.Fatal("CreateEvent returned an invalid handle")
	}
	defer windows.CloseHandle(handle)

	if err := signalCredentialEvent(eventName); err != nil {
		t.Fatalf("signal existing event: %v", err)
	}
	if result, err := windows.WaitForSingleObject(handle, 1000); err != nil || result != windows.WAIT_OBJECT_0 {
		t.Fatalf("event was not signaled: result=%d err=%v", result, err)
	}
}
