package ipc

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
)

const (
	ControlPipe = `\\.\pipe\ProximityUnlock.Control.v1`
	AuthPipe    = `\\.\pipe\ProximityUnlock.Auth.v1`
	ReadyEvent  = `Global\ProximityUnlockCredentialReady`
	maxMessage  = 64 * 1024
)

type ControlRequest struct {
	Version int             `json:"version"`
	Op      string          `json:"op"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type ControlResponse struct {
	OK      bool   `json:"ok"`
	Error   string `json:"error,omitempty"`
	Payload any    `json:"payload,omitempty"`
}

func WriteJSON(w io.Writer, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(data) > maxMessage {
		return errors.New("IPC message is too large")
	}
	var header [4]byte
	binary.LittleEndian.PutUint32(header[:], uint32(len(data)))
	if _, err := w.Write(header[:]); err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func ReadJSON(r io.Reader, value any) error {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return err
	}
	size := binary.LittleEndian.Uint32(header[:])
	if size == 0 || size > maxMessage {
		return errors.New("invalid IPC message size")
	}
	data := make([]byte, size)
	if _, err := io.ReadFull(r, data); err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}
