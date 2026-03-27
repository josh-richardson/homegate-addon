// apps/agent/internal/protocol/frame_test.go
package protocol

import (
	"bytes"
	"testing"
)

func TestEncodeDecodeRoundTrip(t *testing.T) {
	original := Frame{
		StreamID: 42,
		Type:     FrameRequestHeaders,
		Payload:  []byte(`{"method":"GET","path":"/"}`),
	}

	buf := original.Encode()
	decoded, err := DecodeFrame(bytes.NewReader(buf))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	if decoded.StreamID != original.StreamID {
		t.Errorf("stream_id: got %d, want %d", decoded.StreamID, original.StreamID)
	}
	if decoded.Type != original.Type {
		t.Errorf("type: got %d, want %d", decoded.Type, original.Type)
	}
	if !bytes.Equal(decoded.Payload, original.Payload) {
		t.Error("payload mismatch")
	}
}

func TestFrameHeaderSizeConstant(t *testing.T) {
	f := Frame{StreamID: 1, Type: FrameResponseHeaders, Payload: []byte("x")}
	buf := f.Encode()
	if len(buf) != HeaderSize+1 {
		t.Errorf("expected %d bytes, got %d", HeaderSize+1, len(buf))
	}
}

func TestEmptyPayload(t *testing.T) {
	f := Frame{StreamID: 1, Type: FrameStreamClose}
	buf := f.Encode()
	decoded, err := DecodeFrame(bytes.NewReader(buf))
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded.Payload) != 0 {
		t.Errorf("expected empty payload, got %d bytes", len(decoded.Payload))
	}
}
