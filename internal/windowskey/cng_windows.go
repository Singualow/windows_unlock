//go:build windows

package windowskey

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	ncryptMachineKeyFlag = 0x00000020
	ncryptSilentFlag     = 0x00000040
	ecdsaPublicP256Magic = 0x31534345
	algorithmECDSAP256   = "ECDSA_P256"
	publicBlobType       = "ECCPUBLICBLOB"
	platformProvider     = "Microsoft Platform Crypto Provider"
	softwareProvider     = "Microsoft Software Key Storage Provider"
)

var (
	ncrypt                 = windows.NewLazySystemDLL("ncrypt.dll")
	procOpenProvider       = ncrypt.NewProc("NCryptOpenStorageProvider")
	procOpenKey            = ncrypt.NewProc("NCryptOpenKey")
	procCreatePersistedKey = ncrypt.NewProc("NCryptCreatePersistedKey")
	procFinalizeKey        = ncrypt.NewProc("NCryptFinalizeKey")
	procSignHash           = ncrypt.NewProc("NCryptSignHash")
	procExportKey          = ncrypt.NewProc("NCryptExportKey")
	procDeleteKey          = ncrypt.NewProc("NCryptDeleteKey")
	procFreeObject         = ncrypt.NewProc("NCryptFreeObject")
)

type Key struct {
	handle   uintptr
	provider uintptr
	name     string
	backend  string
}

func OpenOrCreate(name string, machine bool) (*Key, error) {
	flags := uintptr(0)
	if machine {
		flags = ncryptMachineKeyFlag
	}
	// Search every provider before creating anything. Otherwise a software KSP
	// key could be silently replaced by a new TPM key when TPM availability
	// changes, breaking the already paired PC identity.
	if key, err := openExisting(name, flags); err == nil {
		return key, nil
	}
	var failures []error
	for _, providerName := range []string{platformProvider, softwareProvider} {
		key, err := openOrCreateWithProvider(providerName, name, flags, true)
		if err == nil {
			return key, nil
		}
		failures = append(failures, fmt.Errorf("%s: %w", providerName, err))
	}
	return nil, errors.Join(failures...)
}

func Open(name string, machine bool) (*Key, error) {
	flags := uintptr(0)
	if machine {
		flags = ncryptMachineKeyFlag
	}
	return openExisting(name, flags)
}

func openExisting(name string, flags uintptr) (*Key, error) {
	var failures []error
	for _, providerName := range []string{platformProvider, softwareProvider} {
		key, err := openOrCreateWithProvider(providerName, name, flags, false)
		if err == nil {
			return key, nil
		}
		failures = append(failures, fmt.Errorf("%s: %w", providerName, err))
	}
	return nil, errors.Join(failures...)
}

func openOrCreateWithProvider(providerName, name string, flags uintptr, createIfMissing bool) (*Key, error) {
	providerPtr, _ := windows.UTF16PtrFromString(providerName)
	namePtr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}
	var provider uintptr
	status, _, _ := procOpenProvider.Call(uintptr(unsafe.Pointer(&provider)), uintptr(unsafe.Pointer(providerPtr)), 0)
	if status != 0 {
		return nil, windows.Errno(status)
	}
	result := &Key{provider: provider, name: name, backend: providerName}
	status, _, _ = procOpenKey.Call(provider, uintptr(unsafe.Pointer(&result.handle)), uintptr(unsafe.Pointer(namePtr)), 0, flags|ncryptSilentFlag)
	if status == 0 {
		return result, nil
	}
	if !createIfMissing {
		result.Close()
		return nil, windows.Errno(status)
	}
	algorithmPtr, _ := windows.UTF16PtrFromString(algorithmECDSAP256)
	status, _, _ = procCreatePersistedKey.Call(provider, uintptr(unsafe.Pointer(&result.handle)), uintptr(unsafe.Pointer(algorithmPtr)), uintptr(unsafe.Pointer(namePtr)), 0, flags)
	if status != 0 {
		result.Close()
		return nil, windows.Errno(status)
	}
	status, _, _ = procFinalizeKey.Call(result.handle, ncryptSilentFlag)
	if status != 0 {
		procDeleteKey.Call(result.handle, ncryptSilentFlag)
		result.handle = 0
		result.Close()
		return nil, windows.Errno(status)
	}
	return result, nil
}

func (k *Key) Backend() string { return k.backend }

func (k *Key) PublicKey() (*ecdsa.PublicKey, error) {
	if k == nil || k.handle == 0 {
		return nil, errors.New("CNG key is closed")
	}
	blobType, _ := windows.UTF16PtrFromString(publicBlobType)
	var size uint32
	status, _, _ := procExportKey.Call(k.handle, 0, uintptr(unsafe.Pointer(blobType)), 0, 0, 0, uintptr(unsafe.Pointer(&size)), 0)
	if status != 0 {
		return nil, windows.Errno(status)
	}
	blob := make([]byte, size)
	status, _, _ = procExportKey.Call(k.handle, 0, uintptr(unsafe.Pointer(blobType)), 0, uintptr(unsafe.Pointer(&blob[0])), uintptr(size), uintptr(unsafe.Pointer(&size)), 0)
	if status != 0 {
		return nil, windows.Errno(status)
	}
	if len(blob) != 72 || binary.LittleEndian.Uint32(blob[:4]) != ecdsaPublicP256Magic || binary.LittleEndian.Uint32(blob[4:8]) != 32 {
		return nil, errors.New("unexpected CNG P-256 public key blob")
	}
	return &ecdsa.PublicKey{Curve: elliptic.P256(), X: new(big.Int).SetBytes(blob[8:40]), Y: new(big.Int).SetBytes(blob[40:72])}, nil
}

func (k *Key) Sign(message []byte) ([64]byte, error) {
	var signature [64]byte
	if k == nil || k.handle == 0 {
		return signature, errors.New("CNG key is closed")
	}
	digest := sha256.Sum256(message)
	var size uint32
	status, _, _ := procSignHash.Call(k.handle, 0, uintptr(unsafe.Pointer(&digest[0])), uintptr(len(digest)), 0, 0, uintptr(unsafe.Pointer(&size)), ncryptSilentFlag)
	if status != 0 {
		return signature, windows.Errno(status)
	}
	if size != uint32(len(signature)) {
		return signature, fmt.Errorf("unexpected CNG ECDSA signature size %d", size)
	}
	status, _, _ = procSignHash.Call(k.handle, 0, uintptr(unsafe.Pointer(&digest[0])), uintptr(len(digest)), uintptr(unsafe.Pointer(&signature[0])), uintptr(len(signature)), uintptr(unsafe.Pointer(&size)), ncryptSilentFlag)
	if status != 0 {
		return [64]byte{}, windows.Errno(status)
	}
	return signature, nil
}

func (k *Key) Delete() error {
	if k == nil || k.handle == 0 {
		return nil
	}
	status, _, _ := procDeleteKey.Call(k.handle, ncryptSilentFlag)
	k.handle = 0
	if status != 0 {
		return windows.Errno(status)
	}
	return nil
}

func (k *Key) Close() error {
	if k == nil {
		return nil
	}
	if k.handle != 0 {
		procFreeObject.Call(k.handle)
		k.handle = 0
	}
	if k.provider != 0 {
		procFreeObject.Call(k.provider)
		k.provider = 0
	}
	return nil
}
