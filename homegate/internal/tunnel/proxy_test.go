// apps/agent/internal/tunnel/proxy_test.go
package tunnel

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/homegate/agent/internal/protocol"
)

func TestProxyHTTPRequest(t *testing.T) {
	// Mock HA server
	ha := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("method: got %s, want GET", r.Method)
		}
		if r.URL.Path != "/api/states" {
			t.Errorf("path: got %s, want /api/states", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`[{"entity_id":"light.living_room"}]`))
	}))
	defer ha.Close()

	proxy := NewRequestProxy(ha.URL)

	// Build request headers frame
	reqHeaders := RequestHeaders{
		Method:  "GET",
		Path:    "/api/states",
		Headers: map[string]string{"Accept": "application/json"},
	}
	headersJSON, _ := json.Marshal(reqHeaders)

	// Simulate receiving frames from broker
	frames := []*protocol.Frame{
		{StreamID: 1, Type: protocol.FrameRequestHeaders, Payload: headersJSON},
	}

	// Process and collect response frames
	var responseFrames []*protocol.Frame
	proxy.HandleStream(1, frames, func(f *protocol.Frame) {
		responseFrames = append(responseFrames, f)
	})

	// Should have: ResponseHeaders + ResponseBody + StreamClose
	if len(responseFrames) < 3 {
		t.Fatalf("expected at least 3 response frames, got %d", len(responseFrames))
	}

	// Check response headers
	if responseFrames[0].Type != protocol.FrameResponseHeaders {
		t.Errorf("first frame type: got %d, want %d", responseFrames[0].Type, protocol.FrameResponseHeaders)
	}
	var respHeaders ResponseHeaders
	json.Unmarshal(responseFrames[0].Payload, &respHeaders)
	if respHeaders.StatusCode != 200 {
		t.Errorf("status: got %d, want 200", respHeaders.StatusCode)
	}

	// Check response body
	if responseFrames[1].Type != protocol.FrameResponseBody {
		t.Errorf("second frame type: got %d, want %d", responseFrames[1].Type, protocol.FrameResponseBody)
	}
	if string(responseFrames[1].Payload) != `[{"entity_id":"light.living_room"}]` {
		t.Errorf("body: got %q", string(responseFrames[1].Payload))
	}

	// Check stream close
	last := responseFrames[len(responseFrames)-1]
	if last.Type != protocol.FrameStreamClose {
		t.Errorf("last frame type: got %d, want %d", last.Type, protocol.FrameStreamClose)
	}
}

func TestProxyPOSTWithBody(t *testing.T) {
	ha := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method: got %s, want POST", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"state":"on"}` {
			t.Errorf("body: got %q", string(body))
		}
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}))
	defer ha.Close()

	proxy := NewRequestProxy(ha.URL)

	reqHeaders := RequestHeaders{
		Method:  "POST",
		Path:    "/api/services/light/turn_on",
		Headers: map[string]string{"Content-Type": "application/json"},
	}
	headersJSON, _ := json.Marshal(reqHeaders)

	frames := []*protocol.Frame{
		{StreamID: 2, Type: protocol.FrameRequestHeaders, Payload: headersJSON},
		{StreamID: 2, Type: protocol.FrameRequestBody, Payload: []byte(`{"state":"on"}`)},
	}

	var responseFrames []*protocol.Frame
	proxy.HandleStream(2, frames, func(f *protocol.Frame) {
		responseFrames = append(responseFrames, f)
	})

	if len(responseFrames) < 2 {
		t.Fatalf("expected at least 2 response frames, got %d", len(responseFrames))
	}
}

func TestProxyHAUnavailable(t *testing.T) {
	proxy := NewRequestProxy("http://localhost:1") // nothing listening

	reqHeaders := RequestHeaders{
		Method:  "GET",
		Path:    "/api/states",
		Headers: map[string]string{},
	}
	headersJSON, _ := json.Marshal(reqHeaders)

	frames := []*protocol.Frame{
		{StreamID: 1, Type: protocol.FrameRequestHeaders, Payload: headersJSON},
	}

	var responseFrames []*protocol.Frame
	proxy.HandleStream(1, frames, func(f *protocol.Frame) {
		responseFrames = append(responseFrames, f)
	})

	// Should send an error response
	if len(responseFrames) < 2 {
		t.Fatalf("expected at least 2 frames, got %d", len(responseFrames))
	}

	var respHeaders ResponseHeaders
	json.Unmarshal(responseFrames[0].Payload, &respHeaders)
	if respHeaders.StatusCode != 502 {
		t.Errorf("expected 502, got %d", respHeaders.StatusCode)
	}
}
