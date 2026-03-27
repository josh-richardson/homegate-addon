// apps/agent/internal/tunnel/client_test.go
package tunnel

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/homegate/agent/internal/protocol"
)

var testUpgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

func TestClient_AuthAndReceiveACK(t *testing.T) {
	var receivedJWT string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, _ := testUpgrader.Upgrade(w, r, nil)
		defer ws.Close()

		_, msg, _ := ws.ReadMessage()
		receivedJWT = string(msg)

		ack, _ := json.Marshal(map[string]string{"status": "authenticated", "label": "coral-thunder-maple"})
		ws.WriteMessage(websocket.TextMessage, ack)

		// Keep connection open briefly
		time.Sleep(200 * time.Millisecond)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// Mock HA server (won't be used in this test)
	ha := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer ha.Close()

	client := NewClient(wsURL, "test-jwt-token", ha.URL)
	errCh := make(chan error, 1)
	go func() {
		errCh <- client.Connect()
	}()

	time.Sleep(100 * time.Millisecond)

	if receivedJWT != "test-jwt-token" {
		t.Errorf("jwt: got %q, want %q", receivedJWT, "test-jwt-token")
	}

	if client.Label() != "coral-thunder-maple" {
		t.Errorf("label: got %q, want %q", client.Label(), "coral-thunder-maple")
	}

	if client.State() != StateConnected {
		t.Errorf("state: got %d, want Connected", client.State())
	}

	client.Close()
}

func TestClient_ProxiesHTTPRequest(t *testing.T) {
	// Mock HA
	ha := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(200)
		w.Write([]byte("hello from HA"))
	}))
	defer ha.Close()

	var mu sync.Mutex
	var receivedFrames []*protocol.Frame

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, _ := testUpgrader.Upgrade(w, r, nil)
		defer ws.Close()

		// Read JWT
		ws.ReadMessage()

		// Send ACK
		ack, _ := json.Marshal(map[string]string{"status": "authenticated", "label": "test"})
		ws.WriteMessage(websocket.TextMessage, ack)

		// Send a request to the agent
		reqHeaders := RequestHeaders{Method: "GET", Path: "/api/states", Headers: map[string]string{}}
		headersJSON, _ := json.Marshal(reqHeaders)
		reqFrame := &protocol.Frame{StreamID: 1, Type: protocol.FrameRequestHeaders, Payload: headersJSON}
		ws.WriteMessage(websocket.BinaryMessage, reqFrame.Encode())

		// Read response frames
		for {
			_, raw, err := ws.ReadMessage()
			if err != nil {
				return
			}
			frame, err := protocol.DecodeFrame(bytes.NewReader(raw))
			if err != nil {
				return
			}
			mu.Lock()
			receivedFrames = append(receivedFrames, frame)
			mu.Unlock()

			if frame.Type == protocol.FrameStreamClose {
				return
			}
		}
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	client := NewClient(wsURL, "jwt", ha.URL)

	go client.Connect()
	time.Sleep(500 * time.Millisecond)
	client.Close()

	mu.Lock()
	defer mu.Unlock()

	if len(receivedFrames) < 3 {
		t.Fatalf("expected at least 3 frames, got %d", len(receivedFrames))
	}

	// First frame: response headers
	if receivedFrames[0].Type != protocol.FrameResponseHeaders {
		t.Errorf("first frame: got type %d, want ResponseHeaders", receivedFrames[0].Type)
	}

	var respHeaders ResponseHeaders
	json.Unmarshal(receivedFrames[0].Payload, &respHeaders)
	if respHeaders.StatusCode != 200 {
		t.Errorf("status: got %d, want 200", respHeaders.StatusCode)
	}

	// Second frame: response body
	if string(receivedFrames[1].Payload) != "hello from HA" {
		t.Errorf("body: got %q, want %q", string(receivedFrames[1].Payload), "hello from HA")
	}

	// Last frame: stream close
	if receivedFrames[len(receivedFrames)-1].Type != protocol.FrameStreamClose {
		t.Error("expected last frame to be StreamClose")
	}
}

func TestClient_ReconnectsOnDisconnect(t *testing.T) {
	connectCount := 0
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, _ := testUpgrader.Upgrade(w, r, nil)

		mu.Lock()
		connectCount++
		count := connectCount
		mu.Unlock()

		ws.ReadMessage() // JWT

		ack, _ := json.Marshal(map[string]string{"status": "authenticated", "label": "test"})
		ws.WriteMessage(websocket.TextMessage, ack)

		if count == 1 {
			// Close immediately on first connection to trigger reconnect
			ws.Close()
			return
		}

		// Keep second connection alive
		time.Sleep(2 * time.Second)
		ws.Close()
	}))
	defer server.Close()

	ha := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer ha.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	client := NewClient(wsURL, "jwt", ha.URL)
	client.SetBaseDelay(50 * time.Millisecond) // speed up test

	go client.Connect()
	time.Sleep(500 * time.Millisecond)
	client.Close()

	mu.Lock()
	defer mu.Unlock()
	if connectCount < 2 {
		t.Errorf("expected at least 2 connections, got %d", connectCount)
	}
}
