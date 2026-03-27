// apps/agent/internal/ui/handler_test.go
package ui

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestRenderUnclaimed(t *testing.T) {
	h := NewHandler("homegate.example", "1.0.0")

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Errorf("status: got %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Not yet claimed") {
		t.Error("expected unclaimed state")
	}
	if !strings.Contains(body, "Claim token") {
		t.Error("expected claim form")
	}
}

func TestRenderConnected(t *testing.T) {
	h := NewHandler("homegate.example", "1.0.0")
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
	h := NewHandler("homegate.example", "1.0.0")
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

func TestClaimPost(t *testing.T) {
	h := NewHandler("homegate.example", "1.0.0")

	var claimedToken string
	h.OnClaim = func(token string) error {
		claimedToken = token
		return nil
	}

	form := url.Values{"token": {"my-claim-token"}}
	req := httptest.NewRequest("POST", "/claim", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if claimedToken != "my-claim-token" {
		t.Errorf("token: got %q, want %q", claimedToken, "my-claim-token")
	}

	if rec.Code != http.StatusSeeOther {
		t.Errorf("status: got %d, want 303", rec.Code)
	}
}

func TestRetryPost(t *testing.T) {
	h := NewHandler("homegate.example", "1.0.0")

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
}
