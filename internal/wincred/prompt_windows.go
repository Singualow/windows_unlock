//go:build windows

package wincred

import (
	"errors"
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	creduiwinGeneric          = 0x00000001
	creduiwinEnumerateCurrent = 0x00000200
	logon32LogonInteractive   = 2
	logon32ProviderDefault    = 0
	errorCancelled            = 1223
)

var (
	credui                = windows.NewLazySystemDLL("credui.dll")
	ole32                 = windows.NewLazySystemDLL("ole32.dll")
	advapi32              = windows.NewLazySystemDLL("advapi32.dll")
	procPromptCredentials = credui.NewProc("CredUIPromptForWindowsCredentialsW")
	procUnpackCredentials = credui.NewProc("CredUnPackAuthenticationBufferW")
	procCoTaskMemFree     = ole32.NewProc("CoTaskMemFree")
	procLogonUser         = advapi32.NewProc("LogonUserW")
)

type credUIInfo struct {
	Size        uint32
	Parent      uintptr
	MessageText *uint16
	CaptionText *uint16
	Banner      windows.Handle
}

type Credential struct {
	CanonicalUser string
	SID           string
	Password      []byte
}

func (c *Credential) Clear() {
	for i := range c.Password {
		c.Password[i] = 0
	}
	c.Password = nil
}

func PromptAndValidate() (Credential, error) {
	caption, _ := windows.UTF16PtrFromString("Proximity Unlock")
	message, _ := windows.UTF16PtrFromString("请输入当前 Windows/Microsoft 账户密码。不能使用 PIN。")
	info := credUIInfo{Size: uint32(unsafe.Sizeof(credUIInfo{})), CaptionText: caption, MessageText: message}
	var authPackage uint32
	var output uintptr
	var outputSize uint32
	var save int32
	result, _, _ := procPromptCredentials.Call(
		uintptr(unsafe.Pointer(&info)), 0, uintptr(unsafe.Pointer(&authPackage)), 0, 0,
		uintptr(unsafe.Pointer(&output)), uintptr(unsafe.Pointer(&outputSize)), uintptr(unsafe.Pointer(&save)),
		creduiwinGeneric|creduiwinEnumerateCurrent,
	)
	if result != 0 {
		if result == errorCancelled {
			return Credential{}, errors.New("credential enrollment was cancelled")
		}
		return Credential{}, windows.Errno(result)
	}
	if output == 0 || outputSize == 0 {
		return Credential{}, errors.New("Windows returned an empty credential buffer")
	}
	packed := unsafe.Slice((*byte)(unsafe.Pointer(output)), int(outputSize))
	defer func() {
		for i := range packed {
			packed[i] = 0
		}
		procCoTaskMemFree.Call(output)
	}()

	username := make([]uint16, 513)
	domain := make([]uint16, 513)
	password := make([]uint16, 513)
	defer zero16(password)
	usernameLen, domainLen, passwordLen := uint32(len(username)), uint32(len(domain)), uint32(len(password))
	ok, _, callErr := procUnpackCredentials.Call(
		0, output, uintptr(outputSize),
		uintptr(unsafe.Pointer(&username[0])), uintptr(unsafe.Pointer(&usernameLen)),
		uintptr(unsafe.Pointer(&domain[0])), uintptr(unsafe.Pointer(&domainLen)),
		uintptr(unsafe.Pointer(&password[0])), uintptr(unsafe.Pointer(&passwordLen)),
	)
	if ok == 0 {
		return Credential{}, fmt.Errorf("unpack credential: %w", callErr)
	}
	userString := windows.UTF16ToString(username)
	domainString := windows.UTF16ToString(domain)
	passwordString := windows.UTF16ToString(password)
	if userString == "" || passwordString == "" {
		return Credential{}, errors.New("username or password is empty")
	}
	sid, err := validate(userString, domainString, password)
	if err != nil {
		return Credential{}, err
	}
	canonical := userString
	if domainString != "" && !strings.Contains(userString, `\`) {
		canonical = domainString + `\` + userString
	}
	credential := Credential{CanonicalUser: canonical, SID: sid, Password: []byte(passwordString)}
	passwordString = ""
	return credential, nil
}

func validate(username, domain string, password []uint16) (string, error) {
	usernamePtr, _ := windows.UTF16PtrFromString(username)
	var domainPtr *uint16
	if domain != "" {
		domainPtr, _ = windows.UTF16PtrFromString(domain)
	}
	var token windows.Token
	ok, _, callErr := procLogonUser.Call(
		uintptr(unsafe.Pointer(usernamePtr)), uintptr(unsafe.Pointer(domainPtr)), uintptr(unsafe.Pointer(&password[0])),
		logon32LogonInteractive, logon32ProviderDefault, uintptr(unsafe.Pointer(&token)),
	)
	if ok == 0 {
		return "", fmt.Errorf("Windows rejected the password: %w", callErr)
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		return "", err
	}
	return user.User.Sid.String(), nil
}

func CurrentSID() (string, error) {
	token := windows.Token(0)
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token); err != nil {
		return "", err
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		return "", err
	}
	return user.User.Sid.String(), nil
}

func IsElevated() bool {
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token); err != nil {
		return false
	}
	defer token.Close()
	return token.IsElevated()
}

func zero16(values []uint16) {
	for i := range values {
		values[i] = 0
	}
}
