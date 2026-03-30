package ui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRenderInitializing(t *testing.T) {
	h := NewHandler("homegate.example", ".", "1.0.0", "https://homegate.example")

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Errorf("status: got %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Initializing") {
		t.Error("expected initializing state")
	}
}

func TestRenderWaiting(t *testing.T) {
	h := NewHandler("homegate.example", ".", "1.0.0", "https://homegate.example")
	h.SetVerificationURL("https://homegate.example/link/abc123")
	h.SetState("waiting", "", "")

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "Waiting to be linked") {
		t.Error("expected waiting state")
	}
	if !strings.Contains(body, "https://homegate.example/link/abc123") {
		t.Error("expected verification URL")
	}
}

func TestRenderConnected(t *testing.T) {
	h := NewHandler("homegate.example", ".", "1.0.0", "https://homegate.example")
	h.SetState("connected", "coral-thunder-maple", "")

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "Connected") {
		t.Error("expected connected state")
	}
	if !strings.Contains(body, "coral-thunder-maple.homegate.example") {
		t.Error("expected public URL")
	}
}

func TestRenderFailed(t *testing.T) {
	h := NewHandler("homegate.example", ".", "1.0.0", "https://homegate.example")
	h.SetState("failed", "", "Device may have been revoked")

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "Connection Failed") {
		t.Error("expected failed state")
	}
	if !strings.Contains(body, "revoked") {
		t.Error("expected error message")
	}
}

func TestRetryPost(t *testing.T) {
	h := NewHandler("homegate.example", ".", "1.0.0", "https://homegate.example")

	retryCalled := false
	h.OnRetry = func() {
		retryCalled = true
	}

	req := httptest.NewRequest("POST", "/retry", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if !retryCalled {
		t.Error("expected OnRetry to be called")
	}

	if rec.Code != http.StatusSeeOther {
		t.Errorf("status: got %d, want 303", rec.Code)
	}
}
