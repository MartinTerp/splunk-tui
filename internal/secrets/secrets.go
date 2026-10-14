// Package secrets manages credentials using 99designs/keyring with an
// encrypted file fallback when no OS keyring daemon is available (e.g. WSL).
package secrets

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/99designs/keyring"
)

const (
	// ServiceName is the application namespace in the OS keyring.
	ServiceName = "splunk-tui"

	// EnvMasterPassword allows skipping the unlock prompt (e.g. for CI or scripting).
	EnvMasterPassword = "SPLUNK_TUI_KEYRING_PASSWORD"
)

var (
	// ErrNotFound is returned when no secret exists for the given key.
	ErrNotFound = errors.New("secret not found")

	// ErrMasterPasswordRequired indicates that the file backend is selected
	// but no master password has been supplied yet.
	ErrMasterPasswordRequired = errors.New("master password required to unlock keyring")
)

// Store provides secure credential persistence for bookmarks.
type Store interface {
	Get(name string) (string, error)
	Set(name, secret string) error
	Delete(name string) error
	Rename(oldName, newName string) error
	Backend() string
}

// Options configures the secrets store.
type Options struct {
	// Backend can be "auto", "file", "secret-service", "keychain", "wincred", "pass".
	Backend string
	// Dir is where the encrypted file store is kept when using the file backend.
	Dir string
	// MasterPassword unlocks the file backend. If empty, EnvMasterPassword is read.
	MasterPassword string
}

type keyringStore struct {
	ring     keyring.Keyring
	backend  string
	memCache map[string]string
	mu       sync.RWMutex
}

// ParseBackend maps string names to keyring.BackendType.
func ParseBackend(s string) (keyring.BackendType, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "secret-service", "secretservice":
		return keyring.SecretServiceBackend, true
	case "keychain":
		return keyring.KeychainBackend, true
	case "wincred", "windows":
		return keyring.WinCredBackend, true
	case "pass":
		return keyring.PassBackend, true
	case "file":
		return keyring.FileBackend, true
	default:
		return "", false
	}
}

func defaultMachineKey(dir string) string {
	h := sha256.Sum256([]byte("splunk-tui-key:" + dir))
	return hex.EncodeToString(h[:])
}

// Open initializes the secrets store.
func Open(opts Options) (Store, error) {
	service := ServiceName
	dir := opts.Dir
	if dir == "" {
		cfgDir, err := os.UserConfigDir()
		if err != nil {
			dir = filepath.Join(".", "keyring")
		} else {
			dir = filepath.Join(cfgDir, "splunk-tui", "keyring")
		}
	}

	masterPass := opts.MasterPassword
	if masterPass == "" {
		masterPass = os.Getenv(EnvMasterPassword)
	}

	backendChoice := strings.ToLower(strings.TrimSpace(opts.Backend))

	cfg := keyring.Config{
		ServiceName: service,
		FileDir:     dir,
		FilePasswordFunc: func(prompt string) (string, error) {
			if masterPass != "" {
				return masterPass, nil
			}
			// If user explicitly chose the file backend without a password, prompt is required
			if backendChoice == "file" {
				return "", ErrMasterPasswordRequired
			}
			// When using auto-fallback, use deterministic machine key so it doesn't block
			return defaultMachineKey(dir), nil
		},
	}

	if backendChoice != "" && backendChoice != "auto" {
		bt, ok := ParseBackend(backendChoice)
		if !ok {
			return nil, fmt.Errorf("unsupported keyring backend %q", opts.Backend)
		}
		cfg.AllowedBackends = []keyring.BackendType{bt}
	}

	ring, err := keyring.Open(cfg)
	if err != nil {
		return nil, fmt.Errorf("opening keyring: %w", err)
	}

	// If "auto", probe the backend with a 500ms timeout so we don't hang if DBus/SecretService is unresponsive
	if backendChoice == "" || backendChoice == "auto" {
		probeDone := make(chan error, 1)
		go func() {
			_, pErr := ring.Get("__splunk_probe__")
			probeDone <- pErr
		}()

		var probeErr error
		select {
		case probeErr = <-probeDone:
		case <-time.After(500 * time.Millisecond):
			probeErr = errors.New("keyring probe timed out")
		}

		// If backend failed (e.g. headless Linux without DBus session), fall back to file backend
		if probeErr != nil && !errors.Is(probeErr, keyring.ErrKeyNotFound) {
			fileCfg := keyring.Config{
				ServiceName: service,
				FileDir:     dir,
				FilePasswordFunc: func(prompt string) (string, error) {
					if masterPass != "" {
						return masterPass, nil
					}
					return defaultMachineKey(dir), nil
				},
				AllowedBackends: []keyring.BackendType{keyring.FileBackend},
			}
			if fileRing, fErr := keyring.Open(fileCfg); fErr == nil {
				ring = fileRing
				backendChoice = "file"
			}
		}
	}

	return &keyringStore{
		ring:     ring,
		backend:  backendChoice,
		memCache: make(map[string]string),
	}, nil
}

func (s *keyringStore) Backend() string {
	if s.backend == "" || s.backend == "auto" {
		return "keyring"
	}
	return s.backend
}

// Get retrieves the stored password for the given bookmark name.
func (s *keyringStore) Get(name string) (string, error) {
	s.mu.RLock()
	if val, ok := s.memCache[name]; ok {
		s.mu.RUnlock()
		return val, nil
	}
	s.mu.RUnlock()

	s.mu.RLock()
	defer s.mu.RUnlock()

	item, err := s.ring.Get(name)
	if err != nil {
		if errors.Is(err, keyring.ErrKeyNotFound) {
			return "", ErrNotFound
		}
		return "", err
	}

	val := string(item.Data)
	s.memCache[name] = val
	return val, nil
}

// Set saves a password for the given bookmark name.
func (s *keyringStore) Set(name, secret string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	err := s.ring.Set(keyring.Item{
		Key:         name,
		Data:        []byte(secret),
		Label:       fmt.Sprintf("splunk-tui credentials for %s", name),
		Description: "Splunk instance password stored by splunk-tui",
	})
	if err != nil {
		return err
	}
	s.memCache[name] = secret
	return nil
}

// Delete removes the password associated with the given bookmark name.
func (s *keyringStore) Delete(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.memCache, name)
	err := s.ring.Remove(name)
	if err != nil && !errors.Is(err, keyring.ErrKeyNotFound) {
		return err
	}
	return nil
}

// Rename updates the bookmark key in the keyring.
func (s *keyringStore) Rename(oldName, newName string) error {
	if oldName == newName {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if val, ok := s.memCache[oldName]; ok {
		s.memCache[newName] = val
		delete(s.memCache, oldName)
	}

	val, err := s.ring.Get(oldName)
	if err != nil {
		if errors.Is(err, keyring.ErrKeyNotFound) {
			return nil // no secret to move
		}
		return err
	}

	if err := s.ring.Set(keyring.Item{
		Key:         newName,
		Data:        val.Data,
		Label:       fmt.Sprintf("splunk-tui credentials for %s", newName),
		Description: "Splunk instance password stored by splunk-tui",
	}); err != nil {
		return err
	}

	_ = s.ring.Remove(oldName)
	return nil
}
