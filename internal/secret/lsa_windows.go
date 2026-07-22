//go:build windows

package secret

import (
	"encoding/base64"
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	policyGetPrivateInformation = 0x00000004
	policyCreateSecret          = 0x00000020
)

var (
	advapi32                   = windows.NewLazySystemDLL("advapi32.dll")
	procLsaOpenPolicy          = advapi32.NewProc("LsaOpenPolicy")
	procLsaStorePrivateData    = advapi32.NewProc("LsaStorePrivateData")
	procLsaRetrievePrivateData = advapi32.NewProc("LsaRetrievePrivateData")
	procLsaFreeMemory          = advapi32.NewProc("LsaFreeMemory")
	procLsaClose               = advapi32.NewProc("LsaClose")
	procLsaNtStatusToWinError  = advapi32.NewProc("LsaNtStatusToWinError")
)

type lsaUnicodeString struct {
	Length        uint16
	MaximumLength uint16
	Buffer        *uint16
}

type lsaObjectAttributes struct {
	Length                   uint32
	RootDirectory            uintptr
	ObjectName               uintptr
	Attributes               uint32
	SecurityDescriptor       uintptr
	SecurityQualityOfService uintptr
}

type LSAStore struct{}

func NewLSAStore() *LSAStore { return &LSAStore{} }

func (s *LSAStore) Put(name string, value []byte) error {
	encoded := base64.RawStdEncoding.EncodeToString(value)
	defer func() { encoded = "" }()
	return withPolicy(policyCreateSecret, func(handle uintptr) error {
		key, keyBacking, err := makeLSAString(name)
		if err != nil {
			return err
		}
		defer zeroUTF16(keyBacking)
		data, dataBacking, err := makeLSAString(encoded)
		if err != nil {
			return err
		}
		defer zeroUTF16(dataBacking)
		status, _, _ := procLsaStorePrivateData.Call(handle, uintptr(unsafe.Pointer(&key)), uintptr(unsafe.Pointer(&data)))
		return lsaStatusError(status)
	})
}

func (s *LSAStore) Get(name string) ([]byte, error) {
	var encoded string
	err := withPolicy(policyGetPrivateInformation, func(handle uintptr) error {
		key, backing, err := makeLSAString(name)
		if err != nil {
			return err
		}
		defer zeroUTF16(backing)
		var output *lsaUnicodeString
		status, _, _ := procLsaRetrievePrivateData.Call(handle, uintptr(unsafe.Pointer(&key)), uintptr(unsafe.Pointer(&output)))
		if status != 0 {
			winErr := lsaStatusError(status)
			if errors.Is(winErr, windows.ERROR_FILE_NOT_FOUND) || errors.Is(winErr, windows.ERROR_NOT_FOUND) {
				return ErrNotFound
			}
			return winErr
		}
		if output == nil {
			return ErrNotFound
		}
		defer procLsaFreeMemory.Call(uintptr(unsafe.Pointer(output)))
		units := unsafe.Slice(output.Buffer, int(output.Length)/2)
		encoded = windows.UTF16ToString(units)
		return nil
	})
	if err != nil {
		return nil, err
	}
	decoded, err := base64.RawStdEncoding.DecodeString(encoded)
	encoded = ""
	if err != nil {
		return nil, fmt.Errorf("decode LSA secret: %w", err)
	}
	return decoded, nil
}

func (s *LSAStore) Delete(name string) error {
	return withPolicy(policyCreateSecret, func(handle uintptr) error {
		key, backing, err := makeLSAString(name)
		if err != nil {
			return err
		}
		defer zeroUTF16(backing)
		status, _, _ := procLsaStorePrivateData.Call(handle, uintptr(unsafe.Pointer(&key)), 0)
		err = lsaStatusError(status)
		if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_NOT_FOUND) {
			return nil
		}
		return err
	})
}

func withPolicy(access uint32, action func(uintptr) error) error {
	attributes := lsaObjectAttributes{Length: uint32(unsafe.Sizeof(lsaObjectAttributes{}))}
	var handle uintptr
	status, _, _ := procLsaOpenPolicy.Call(0, uintptr(unsafe.Pointer(&attributes)), uintptr(access), uintptr(unsafe.Pointer(&handle)))
	if err := lsaStatusError(status); err != nil {
		return err
	}
	defer procLsaClose.Call(handle)
	return action(handle)
}

func makeLSAString(value string) (lsaUnicodeString, []uint16, error) {
	backing, err := windows.UTF16FromString(value)
	if err != nil {
		return lsaUnicodeString{}, nil, err
	}
	byteLength := (len(backing) - 1) * 2
	if byteLength > 0xffff-2 {
		return lsaUnicodeString{}, nil, errors.New("LSA string is too long")
	}
	return lsaUnicodeString{Length: uint16(byteLength), MaximumLength: uint16(byteLength + 2), Buffer: &backing[0]}, backing, nil
}

func lsaStatusError(status uintptr) error {
	if status == 0 {
		return nil
	}
	code, _, _ := procLsaNtStatusToWinError.Call(status)
	return windows.Errno(code)
}

func zeroUTF16(value []uint16) {
	for i := range value {
		value[i] = 0
	}
}
