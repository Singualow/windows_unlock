package protocol

import (
	"errors"
	"fmt"
)

const FragmentHeaderSize = 3

func Fragment(messageType byte, payload []byte, mtuPayload int) ([][]byte, error) {
	if mtuPayload <= FragmentHeaderSize {
		return nil, errors.New("MTU payload is too small")
	}
	partSize := mtuPayload - FragmentHeaderSize
	count := (len(payload) + partSize - 1) / partSize
	if count == 0 {
		count = 1
	}
	if count > 255 {
		return nil, errors.New("payload requires too many BLE fragments")
	}
	result := make([][]byte, 0, count)
	for i := 0; i < count; i++ {
		start := i * partSize
		end := start + partSize
		if end > len(payload) {
			end = len(payload)
		}
		part := make([]byte, FragmentHeaderSize+end-start)
		part[0] = messageType
		part[1] = byte(i)
		part[2] = byte(count)
		copy(part[FragmentHeaderSize:], payload[start:end])
		result = append(result, part)
	}
	return result, nil
}

type Reassembler struct {
	messageType byte
	count       int
	parts       [][]byte
	received    int
}

func (r *Reassembler) Add(fragment []byte) ([]byte, bool, error) {
	if len(fragment) < FragmentHeaderSize || fragment[2] == 0 || fragment[1] >= fragment[2] {
		return nil, false, errors.New("invalid BLE fragment")
	}
	messageType, index, count := fragment[0], int(fragment[1]), int(fragment[2])
	if r.parts == nil {
		r.messageType = messageType
		r.count = count
		r.parts = make([][]byte, count)
	} else if r.messageType != messageType || r.count != count {
		return nil, false, errors.New("fragment belongs to a different message")
	}
	if r.parts[index] == nil {
		r.parts[index] = append([]byte(nil), fragment[FragmentHeaderSize:]...)
		r.received++
	}
	if r.received != r.count {
		return nil, false, nil
	}
	var total int
	for _, part := range r.parts {
		total += len(part)
	}
	payload := make([]byte, 0, total)
	for _, part := range r.parts {
		payload = append(payload, part...)
	}
	r.Reset()
	return payload, true, nil
}

func (r *Reassembler) Reset() {
	r.messageType = 0
	r.count = 0
	r.parts = nil
	r.received = 0
}

func (r *Reassembler) String() string {
	return fmt.Sprintf("type=%d parts=%d/%d", r.messageType, r.received, r.count)
}
