// apps/agent/internal/credentials/store.go
package credentials

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

const filename = "credentials.json"

type Credentials struct {
	DeviceID  string `json:"device_id"`
	JWT       string `json:"jwt"`
	BrokerURL string `json:"broker_url"`
}

type Store struct {
	dir string
}

func NewStore(dir string) *Store {
	return &Store{dir: dir}
}

func (s *Store) path() string {
	return filepath.Join(s.dir, filename)
}

func (s *Store) Save(creds *Credentials) error {
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path(), data, 0600)
}

func (s *Store) Load() (*Credentials, error) {
	data, err := os.ReadFile(s.path())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, err
	}
	return &creds, nil
}

func (s *Store) Clear() {
	os.Remove(s.path())
}
