//go:build windows

package ipc

import (
	"errors"

	"golang.org/x/sys/windows"
)

func SignalCredentialReady() error {
	return signalCredentialEvent(ReadyEvent)
}

func signalCredentialEvent(eventName string) error {
	name, err := windows.UTF16PtrFromString(eventName)
	if err != nil {
		return err
	}
	handle, err := windows.CreateEvent(nil, 0, 0, name)
	// CreateEvent returns a valid handle together with ERROR_ALREADY_EXISTS
	// when LogonUI created the named event first. That is the normal signaling
	// path: keep the handle and set the existing event.
	if err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return err
	}
	if handle == 0 {
		return windows.ERROR_INVALID_HANDLE
	}
	defer windows.CloseHandle(handle)
	return windows.SetEvent(handle)
}
