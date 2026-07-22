//go:build windows

package winshell

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	shell32            = windows.NewLazySystemDLL("shell32.dll")
	procShellExecute   = shell32.NewProc("ShellExecuteW")
	procShellExecuteEx = shell32.NewProc("ShellExecuteExW")
)

const seeMaskNoCloseProcess = 0x00000040

type shellExecuteInfo struct {
	Size       uint32
	Mask       uint32
	Window     uintptr
	Verb       *uint16
	File       *uint16
	Parameters *uint16
	Directory  *uint16
	Show       int32
	Instance   uintptr
	IDList     uintptr
	Class      *uint16
	ClassKey   uintptr
	HotKey     uint32
	Icon       uintptr
	Process    windows.Handle
}

func Open(path string) error {
	return Execute("open", path, "")
}

func Execute(verb, path, parameters string) error {
	operation, _ := windows.UTF16PtrFromString(verb)
	file, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	var parameterPtr *uint16
	if parameters != "" {
		parameterPtr, err = windows.UTF16PtrFromString(parameters)
		if err != nil {
			return err
		}
	}
	result, _, _ := procShellExecute.Call(
		0,
		uintptr(unsafe.Pointer(operation)),
		uintptr(unsafe.Pointer(file)),
		uintptr(unsafe.Pointer(parameterPtr)),
		0,
		1,
	)
	if result <= 32 {
		return fmt.Errorf("ShellExecute failed with code %d", result)
	}
	return nil
}

func ExecuteWait(verb, path, parameters string) (uint32, error) {
	operation, _ := windows.UTF16PtrFromString(verb)
	file, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	var parameterPtr *uint16
	if parameters != "" {
		parameterPtr, err = windows.UTF16PtrFromString(parameters)
		if err != nil {
			return 0, err
		}
	}
	info := shellExecuteInfo{
		Size:       uint32(unsafe.Sizeof(shellExecuteInfo{})),
		Mask:       seeMaskNoCloseProcess,
		Verb:       operation,
		File:       file,
		Parameters: parameterPtr,
		Show:       1,
	}
	ok, _, callErr := procShellExecuteEx.Call(uintptr(unsafe.Pointer(&info)))
	if ok == 0 {
		return 0, callErr
	}
	if info.Process == 0 {
		return 0, fmt.Errorf("ShellExecuteEx returned no process handle")
	}
	defer windows.CloseHandle(info.Process)
	if _, err := windows.WaitForSingleObject(info.Process, windows.INFINITE); err != nil {
		return 0, err
	}
	var exitCode uint32
	if err := windows.GetExitCodeProcess(info.Process, &exitCode); err != nil {
		return 0, err
	}
	return exitCode, nil
}
