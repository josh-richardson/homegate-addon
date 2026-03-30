package link

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type LinkRequestResult struct {
	RequestID       string `json:"requestId"`
	VerificationURL string `json:"verificationUrl"`
	ExpiresAt       string `json:"expiresAt"`
}

type LinkStatusResult struct {
	Status    string `json:"status"`
	DeviceID  string `json:"deviceId,omitempty"`
	DeviceJWT string `json:"deviceJwt,omitempty"`
	BrokerURL string `json:"brokerUrl,omitempty"`
}

func CreateRequest(apiBaseURL, deviceUUID string) (*LinkRequestResult, error) {
	body, err := json.Marshal(map[string]string{"deviceUuid": deviceUUID})
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Post(
		apiBaseURL+"/device-auth/link-request",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("link request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		var errResp struct {
			Message string `json:"message"`
		}
		json.NewDecoder(resp.Body).Decode(&errResp)
		if errResp.Message != "" {
			return nil, fmt.Errorf("link request failed: %s", errResp.Message)
		}
		return nil, fmt.Errorf("link request failed with status %d", resp.StatusCode)
	}

	var result LinkRequestResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode link request response: %w", err)
	}
	return &result, nil
}

func PollStatus(apiBaseURL, requestID string) (*LinkStatusResult, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(apiBaseURL + "/device-auth/link-status/" + requestID)
	if err != nil {
		return nil, fmt.Errorf("poll status failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("poll status failed with status %d", resp.StatusCode)
	}

	var result LinkStatusResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode status response: %w", err)
	}
	return &result, nil
}
