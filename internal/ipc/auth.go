package ipc

import (
	"encoding/binary"
	"errors"
	"io"
	"unicode/utf16"
)

var authMagic = [4]byte{'P', 'U', 'A', '1'}

const (
	AuthUnavailable uint32 = 0
	AuthAvailable   uint32 = 1
	AuthError       uint32 = 2
	AuthInvalid     uint32 = 3
)

type AuthResponse struct {
	Status    uint32
	Username  string
	Password  string
	TargetSID string
}

func WriteAuthResponse(w io.Writer, response AuthResponse) error {
	user := utf16.Encode([]rune(response.Username))
	password := utf16.Encode([]rune(response.Password))
	targetSID := utf16.Encode([]rune(response.TargetSID))
	defer zero16(password)
	if len(user) > 4096 || len(password) > 4096 || len(targetSID) > 4096 {
		return errors.New("credential field is too long")
	}
	// The auth pipe is a message-mode named pipe. Every Write call becomes a
	// separate pipe message, so the complete response must be serialized and
	// written in one call. Otherwise CallNamedPipeW receives only the 20-byte
	// header and the credential provider rejects the response as truncated.
	buffer := make([]byte, 20+2*(len(user)+len(password)+len(targetSID)))
	defer func() {
		for i := range buffer {
			buffer[i] = 0
		}
	}()
	copy(buffer[:4], authMagic[:])
	binary.LittleEndian.PutUint32(buffer[4:8], response.Status)
	binary.LittleEndian.PutUint32(buffer[8:12], uint32(len(user)))
	binary.LittleEndian.PutUint32(buffer[12:16], uint32(len(password)))
	binary.LittleEndian.PutUint32(buffer[16:20], uint32(len(targetSID)))
	offset := 20
	offset = appendUTF16(buffer, offset, user)
	offset = appendUTF16(buffer, offset, password)
	appendUTF16(buffer, offset, targetSID)
	written, err := w.Write(buffer)
	if err == nil && written != len(buffer) {
		return io.ErrShortWrite
	}
	return err
}

func appendUTF16(buffer []byte, offset int, values []uint16) int {
	for i, value := range values {
		binary.LittleEndian.PutUint16(buffer[offset+i*2:], value)
	}
	return offset + len(values)*2
}

func zero16(values []uint16) {
	for i := range values {
		values[i] = 0
	}
}
