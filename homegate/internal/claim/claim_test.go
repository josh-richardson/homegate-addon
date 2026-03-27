package claim

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClaimSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method: got %s, want POST", r.Method)
		}
		if r.URL.Path != "/device-auth/claim" {
			t.Errorf("path: got %s, want /device-auth/claim", r.URL.Path)
		}

		var body struct {
			Token string `json:"token"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.Token != "test-claim-token" {
			t.Errorf("token: got %q, want %q", body.Token, "test-claim-token")
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{
			"deviceId":  "device-123",
			"deviceJwt": "eyJ.test.jwt",
			"brokerUrl": "wss://broker.homegate.example:8081",
		})
	}))
	defer server.Close()

	result, err := Exchange(server.URL, "test-claim-token")
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}
	if result.DeviceID != "device-123" {
		t.Errorf("deviceID: got %q, want %q", result.DeviceID, "device-123")
	}
	if result.DeviceJWT != "eyJ.test.jwt" {
		t.Errorf("jwt: got %q, want %q", result.DeviceJWT, "eyJ.test.jwt")
	}
	if result.BrokerURL != "wss://broker.homegate.example:8081" {
		t.Errorf("brokerURL: got %q, want %q", result.BrokerURL, "wss://broker.homegate.example:8081")
	}
}

func TestClaimInvalidToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"message": "Invalid or expired token",
		})
	}))
	defer server.Close()

	_, err := Exchange(server.URL, "bad-token")
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
}

func TestClaimServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	_, err := Exchange(server.URL, "any-token")
	if err == nil {
		t.Fatal("expected error for server error")
	}
}
