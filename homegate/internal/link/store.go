package link

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type LinkState struct {
	DeviceUUID      string `json:"deviceUuid"`
	RequestID       string `json:"requestId"`
	VerificationURL string `json:"verificationUrl"`
	ExpiresAt       string `json:"expiresAt"`
}

type Store struct {
	path string
}

func NewStore(dataDir string) *Store {
	return &Store{path: filepath.Join(dataDir, "link-request.json")}
}

func (s *Store) Load() (*LinkState, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return nil, err
	}
	var state LinkState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

func (s *Store) Save(state *LinkState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0600)
}

func (s *Store) Clear() error {
	if err := os.Remove(s.path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (ls *LinkState) IsExpired() bool {
	t, err := time.Parse(time.RFC3339, ls.ExpiresAt)
	if err != nil {
		return true
	}
	return time.Now().After(t)
}
