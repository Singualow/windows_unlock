//go:build windows

package winshell

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	messageBoxOK            = 0x00000000
	messageBoxOKCancel      = 0x00000001
	messageBoxIconError     = 0x00000010
	messageBoxIconWarning   = 0x00000030
	messageBoxIconInfo      = 0x00000040
	messageBoxTopmost       = 0x00040000
	messageBoxSetForeground = 0x00010000
	messageBoxResultOK      = 1
)

var procMessageBox = windows.NewLazySystemDLL("user32.dll").NewProc("MessageBoxW")

func Info(title, message string) {
	messageBox(title, message, messageBoxOK|messageBoxIconInfo)
}

func Error(title, message string) {
	messageBox(title, message, messageBoxOK|messageBoxIconError)
}

func Confirm(title, message string) bool {
	return messageBox(title, message, messageBoxOKCancel|messageBoxIconWarning) == messageBoxResultOK
}

func messageBox(title, message string, flags uintptr) uintptr {
	titlePtr, _ := windows.UTF16PtrFromString(title)
	messagePtr, _ := windows.UTF16PtrFromString(message)
	result, _, _ := procMessageBox.Call(
		0,
		uintptr(unsafe.Pointer(messagePtr)),
		uintptr(unsafe.Pointer(titlePtr)),
		flags|messageBoxTopmost|messageBoxSetForeground,
	)
	return result
}
