// apps/agent/internal/protocol/frame.go
package protocol

import (
	"encoding/binary"
	"fmt"
	"io"
)

const (
	HeaderSize     = 9
	MaxPayloadSize = 10 * 1024 * 1024
)

type FrameType byte

const (
	FrameRequestHeaders  FrameType = 0x01
	FrameRequestBody     FrameType = 0x02
	FrameResponseHeaders FrameType = 0x03
	FrameResponseBody    FrameType = 0x04
	FrameWebSocketData   FrameType = 0x05
	FrameStreamClose     FrameType = 0x06
)

type Frame struct {
	StreamID uint32
	Type     FrameType
	Payload  []byte
}

func (f *Frame) Encode() []byte {
	payloadLen := len(f.Payload)
	buf := make([]byte, HeaderSize+payloadLen)
	binary.BigEndian.PutUint32(buf[0:4], f.StreamID)
	buf[4] = byte(f.Type)
	binary.BigEndian.PutUint32(buf[5:9], uint32(payloadLen))
	if payloadLen > 0 {
		copy(buf[9:], f.Payload)
	}
	return buf
}

func DecodeFrame(r io.Reader) (*Frame, error) {
	header := make([]byte, HeaderSize)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}

	streamID := binary.BigEndian.Uint32(header[0:4])
	frameType := FrameType(header[4])
	payloadLen := binary.BigEndian.Uint32(header[5:9])

	if payloadLen > MaxPayloadSize {
		return nil, fmt.Errorf("payload too large: %d > %d", payloadLen, MaxPayloadSize)
	}

	var payload []byte
	if payloadLen > 0 {
		payload = make([]byte, payloadLen)
		if _, err := io.ReadFull(r, payload); err != nil {
			return nil, fmt.Errorf("read payload: %w", err)
		}
	}

	return &Frame{
		StreamID: streamID,
		Type:     frameType,
		Payload:  payload,
	}, nil
}
