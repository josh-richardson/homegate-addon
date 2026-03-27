package claim

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type ClaimResult struct {
	DeviceID  string `json:"deviceId"`
	DeviceJWT string `json:"deviceJwt"`
	BrokerURL string `json:"brokerUrl"`
}

func Exchange(apiBaseURL, token string) (*ClaimResult, error) {
	body, err := json.Marshal(map[string]string{"token": token})
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Post(
		apiBaseURL+"/device-auth/claim",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("claim request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		var errResp struct {
			Message string `json:"message"`
		}
		json.NewDecoder(resp.Body).Decode(&errResp)
		if errResp.Message != "" {
			return nil, fmt.Errorf("claim failed: %s", errResp.Message)
		}
		return nil, fmt.Errorf("claim failed with status %d", resp.StatusCode)
	}

	var result ClaimResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode claim response: %w", err)
	}
	return &result, nil
}
