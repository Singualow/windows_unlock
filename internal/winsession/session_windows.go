//go:build windows

package winsession

import (
	"os"

	"golang.org/x/sys/windows"
)

var (
	user32              = windows.NewLazySystemDLL("user32.dll")
	kernel32            = windows.NewLazySystemDLL("kernel32.dll")
	procLockWorkStation = user32.NewProc("LockWorkStation")
	procActiveConsoleID = kernel32.NewProc("WTSGetActiveConsoleSessionId")
)

func CurrentID() (uint32, error) {
	var sessionID uint32
	if err := windows.ProcessIdToSessionId(uint32(os.Getpid()), &sessionID); err != nil {
		return 0, err
	}
	return sessionID, nil
}

func ActiveConsoleID() (uint32, bool) {
	result, _, _ := procActiveConsoleID.Call()
	sessionID := uint32(result)
	return sessionID, sessionID != 0xffffffff
}

func IsActiveConsole(sessionID uint32) bool {
	active, ok := ActiveConsoleID()
	return ok && active == sessionID
}

func Lock() error {
	result, _, err := procLockWorkStation.Call()
	if result == 0 {
		return err
	}
	return nil
}
