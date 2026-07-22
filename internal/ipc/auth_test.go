package ipc

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"
	"unicode/utf16"
)

type countingWriter struct {
	bytes.Buffer
	writes int
	short  bool
}

func (w *countingWriter) Write(value []byte) (int, error) {
	w.writes++
	if w.short && len(value) > 0 {
		return len(value) - 1, nil
	}
	return w.Buffer.Write(value)
}

func TestAuthResponseWireIncludesTargetSID(t *testing.T) {
	want := AuthResponse{
		Status:    AuthAvailable,
		Username:  `MicrosoftAccount\test@example.com`,
		Password:  "secret-password",
		TargetSID: "S-1-5-21-1001",
	}
	var buffer bytes.Buffer
	if err := WriteAuthResponse(&buffer, want); err != nil {
		t.Fatal(err)
	}
	data := buffer.Bytes()
	if len(data) < 20 || string(data[:4]) != "PUA1" {
		t.Fatal("invalid auth response header")
	}
	lengths := []uint32{
		binary.LittleEndian.Uint32(data[8:12]),
		binary.LittleEndian.Uint32(data[12:16]),
		binary.LittleEndian.Uint32(data[16:20]),
	}
	offset := 20
	readString := func(length uint32) string {
		units := make([]uint16, length)
		for i := range units {
			units[i] = binary.LittleEndian.Uint16(data[offset+i*2:])
		}
		offset += int(length) * 2
		return string(utf16.Decode(units))
	}
	if got := readString(lengths[0]); got != want.Username {
		t.Fatalf("username = %q", got)
	}
	if got := readString(lengths[1]); got != want.Password {
		t.Fatalf("password = %q", got)
	}
	if got := readString(lengths[2]); got != want.TargetSID {
		t.Fatalf("target SID = %q", got)
	}
	if offset != len(data) {
		t.Fatal("unexpected trailing auth response bytes")
	}
}

func TestAuthResponseUsesOnePipeMessage(t *testing.T) {
	writer := &countingWriter{}
	if err := WriteAuthResponse(writer, AuthResponse{
		Status:    AuthAvailable,
		Username:  `MicrosoftAccount\test@example.com`,
		Password:  "secret-password",
		TargetSID: "S-1-5-21-1001",
	}); err != nil {
		t.Fatal(err)
	}
	if writer.writes != 1 {
		t.Fatalf("writes = %d, want exactly one message-mode pipe write", writer.writes)
	}
}

func TestAuthResponseRejectsShortWrite(t *testing.T) {
	writer := &countingWriter{short: true}
	err := WriteAuthResponse(writer, AuthResponse{Status: AuthUnavailable})
	if !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("error = %v, want %v", err, io.ErrShortWrite)
	}
}
