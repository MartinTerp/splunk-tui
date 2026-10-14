package secrets

import (
	"path/filepath"
	"testing"
)

func TestFileBackendStore(t *testing.T) {
	tempDir := filepath.Join(t.TempDir(), "keyring")

	opts := Options{
		Backend:        "file",
		Dir:            tempDir,
		MasterPassword: "test-master-password-123",
	}

	store, err := Open(opts)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	// 1. Get nonexistent
	_, err = store.Get("nonexistent")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	// 2. Set and Get
	if err := store.Set("prod", "secretPass99"); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	pass, err := store.Get("prod")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if pass != "secretPass99" {
		t.Errorf("got %q, want %q", pass, "secretPass99")
	}

	// 3. Rename
	if err := store.Rename("prod", "prod-renamed"); err != nil {
		t.Fatalf("Rename failed: %v", err)
	}
	_, err = store.Get("prod")
	if err != ErrNotFound {
		t.Errorf("old key should not exist, got err: %v", err)
	}
	pass, err = store.Get("prod-renamed")
	if err != nil || pass != "secretPass99" {
		t.Errorf("expected renamed pass, got %q, err: %v", pass, err)
	}

	// 4. Delete
	if err := store.Delete("prod-renamed"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	_, err = store.Get("prod-renamed")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound after Delete, got %v", err)
	}
}

func TestFileBackendRequiresPassword(t *testing.T) {
	tempDir := filepath.Join(t.TempDir(), "keyring")

	opts := Options{
		Backend:        "file",
		Dir:            tempDir,
		MasterPassword: "", // empty
	}

	store, err := Open(opts)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	// Attempting an operation should return ErrMasterPasswordRequired
	err = store.Set("prod", "pw")
	if err == nil {
		t.Fatalf("expected error without master password, got nil")
	}
}
