package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/nghyane/llm-mux/internal/json"
	"github.com/nghyane/llm-mux/internal/logging"
)

const (
	CredentialsFileName = "credentials.json"
	ManagementKeyLength = 16 // 32-char hex string
	CredentialsVersion  = 1
)

type Credentials struct {
	ManagementKey string    `json:"management-key"`
	CreatedAt     time.Time `json:"created-at"`
	Version       int       `json:"version"`
}

var (
	cache   *Credentials
	cacheMu sync.RWMutex
)

// CredentialsDir returns the credentials directory following XDG Base Directory spec.
// Uses $XDG_CONFIG_HOME/llm-mux if set, otherwise ~/.config/llm-mux
func CredentialsDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "llm-mux")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".config", "llm-mux")
	}
	return ""
}

// CredentialsFilePath returns the credentials file path following XDG spec.
// Uses $XDG_CONFIG_HOME/llm-mux/credentials.json if set, otherwise ~/.config/llm-mux/credentials.json
func CredentialsFilePath() string {
	dir := CredentialsDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, CredentialsFileName)
}

func GenerateManagementKey() (string, error) {
	b := make([]byte, ManagementKeyLength)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// LoadCredentials loads credentials with priority: ENV > file
func LoadCredentials() (*Credentials, error) {
	// Priority 1: Environment variable (LLM_MUX_MANAGEMENT_KEY with legacy fallback)
	for _, envKey := range []string{"LLM_MUX_MANAGEMENT_KEY", "MANAGEMENT_PASSWORD"} {
		if key := strings.TrimSpace(os.Getenv(envKey)); key != "" {
			return &Credentials{ManagementKey: key, CreatedAt: time.Now(), Version: CredentialsVersion}, nil
		}
	}

	// Priority 2: Cache
	cacheMu.RLock()
	if cache != nil {
		c := *cache
		cacheMu.RUnlock()
		return &c, nil
	}
	cacheMu.RUnlock()

	// Priority 3: File
	path := CredentialsFilePath()
	if path == "" {
		return nil, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}

	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, err
	}

	// Migration: handle old snake_case format
	if creds.ManagementKey == "" {
		var oldCreds struct {
			ManagementKey string    `json:"management_key"`
			CreatedAt     time.Time `json:"created_at"`
			Version       int       `json:"version"`
		}
		if json.Unmarshal(data, &oldCreds) == nil && oldCreds.ManagementKey != "" {
			logging.Warn("credentials.json uses deprecated snake_case format, migrating to kebab-case")
			creds.ManagementKey = oldCreds.ManagementKey
			creds.CreatedAt = oldCreds.CreatedAt
			creds.Version = oldCreds.Version
			// Auto-migrate: save in new format
			_ = SaveCredentials(&creds)
		}
	}

	if creds.ManagementKey == "" {
		return nil, nil
	}

	cacheMu.Lock()
	cache = &creds
	cacheMu.Unlock()

	return &creds, nil
}

func SaveCredentials(creds *Credentials) error {
	path := CredentialsFilePath()
	if path == "" {
		return fmt.Errorf("cannot determine credentials path")
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}

	if creds.Version == 0 {
		creds.Version = CredentialsVersion
	}

	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}

	cacheMu.Lock()
	cache = creds
	cacheMu.Unlock()

	return nil
}

func CreateCredentials() (string, error) {
	key, err := GenerateManagementKey()
	if err != nil {
		return "", err
	}
	creds := &Credentials{ManagementKey: key, CreatedAt: time.Now(), Version: CredentialsVersion}
	if err := SaveCredentials(creds); err != nil {
		return "", err
	}
	return key, nil
}

func GetManagementKey() string {
	creds, _ := LoadCredentials()
	if creds == nil {
		return ""
	}
	return creds.ManagementKey
}

func HasManagementKey() bool {
	return GetManagementKey() != ""
}

func InvalidateCache() {
	cacheMu.Lock()
	cache = nil
	cacheMu.Unlock()
}
