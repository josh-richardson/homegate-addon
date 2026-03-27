// apps/agent/internal/credentials/store_test.go
package credentials

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	creds := &Credentials{
		DeviceID:  "device-abc",
		JWT:       "eyJhbGciOiJFZERTQSJ9.test.signature",
		BrokerURL: "wss://broker.homegate.example:8081",
	}

	if err := store.Save(creds); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if loaded.DeviceID != creds.DeviceID {
		t.Errorf("deviceID: got %q, want %q", loaded.DeviceID, creds.DeviceID)
	}
	if loaded.JWT != creds.JWT {
		t.Errorf("jwt: got %q, want %q", loaded.JWT, creds.JWT)
	}
	if loaded.BrokerURL != creds.BrokerURL {
		t.Errorf("brokerURL: got %q, want %q", loaded.BrokerURL, creds.BrokerURL)
	}
}

func TestLoadMissing(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	creds, err := store.Load()
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	if creds != nil {
		t.Fatalf("expected nil for missing file, got: %+v", creds)
	}
}

func TestClear(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	store.Save(&Credentials{DeviceID: "x", JWT: "y", BrokerURL: "z"})
	store.Clear()

	creds, _ := store.Load()
	if creds != nil {
		t.Fatal("expected nil after clear")
	}
}

func TestSaveFilePermissions(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	store.Save(&Credentials{DeviceID: "x", JWT: "y", BrokerURL: "z"})

	info, err := os.Stat(filepath.Join(dir, "credentials.json"))
	if err != nil {
		t.Fatal(err)
	}
	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("permissions: got %o, want 0600", perm)
	}
}
