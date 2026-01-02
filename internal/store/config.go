package store

import "strings"

// StoreType identifies the backend storage mechanism.
type StoreType string

const (
	// TypeNone indicates file-based storage (no remote store).
	TypeNone StoreType = ""
	// TypePostgres indicates PostgreSQL-backed storage.
	TypePostgres StoreType = "postgres"
	// TypeObject indicates S3-compatible object storage.
	TypeObject StoreType = "object"
	// TypeGit indicates git repository-backed storage.
	TypeGit StoreType = "git"
)

// GitStoreConfig captures configuration for git-backed storage.
type GitStoreConfig struct {
	RemoteURL string
	Username  string
	Password  string
	LocalPath string
}

// StoreConfig unifies configuration for all store backends.
type StoreConfig struct {
	Type     StoreType
	Postgres PostgresStoreConfig
	Git      GitStoreConfig
	Object   ObjectStoreConfig
}

// LookupEnvFunc is a function that looks up environment variables.
// It accepts multiple keys and returns the first non-empty value found.
type LookupEnvFunc func(keys ...string) (string, bool)

// ParseFromEnv builds a StoreConfig from environment variables.
// The lookupEnv function is injected to avoid circular dependencies with the env package.
// writableBase provides the default local path when no explicit path is configured.
func ParseFromEnv(lookupEnv LookupEnvFunc, writableBase string) StoreConfig {
	cfg := StoreConfig{}

	storeType, _ := lookupEnv("LLM_MUX_STORE_TYPE")
	storeType = strings.ToLower(strings.TrimSpace(storeType))

	// Parse Postgres store configuration
	if storeType == "postgres" || storeType == "pg" {
		cfg.Type = TypePostgres
	}
	if value, ok := lookupEnv("LLM_MUX_PGSTORE_DSN", "PGSTORE_DSN"); ok {
		cfg.Type = TypePostgres
		cfg.Postgres.DSN = value
	}
	if cfg.Type == TypePostgres {
		if value, ok := lookupEnv("LLM_MUX_PGSTORE_SCHEMA", "PGSTORE_SCHEMA"); ok {
			cfg.Postgres.Schema = value
		}
		if value, ok := lookupEnv("LLM_MUX_PGSTORE_LOCAL_PATH", "PGSTORE_LOCAL_PATH"); ok {
			cfg.Postgres.SpoolDir = value
		}
		if cfg.Postgres.SpoolDir == "" && writableBase != "" {
			cfg.Postgres.SpoolDir = writableBase
		}
		return cfg
	}

	// Parse Git store configuration
	if storeType == "git" {
		cfg.Type = TypeGit
	}
	if value, ok := lookupEnv("LLM_MUX_GITSTORE_URL", "GITSTORE_GIT_URL"); ok {
		cfg.Type = TypeGit
		cfg.Git.RemoteURL = value
	}
	if cfg.Type == TypeGit {
		if value, ok := lookupEnv("LLM_MUX_GITSTORE_USERNAME", "GITSTORE_GIT_USERNAME"); ok {
			cfg.Git.Username = value
		}
		if value, ok := lookupEnv("LLM_MUX_GITSTORE_TOKEN", "GITSTORE_GIT_TOKEN"); ok {
			cfg.Git.Password = value
		}
		if value, ok := lookupEnv("LLM_MUX_GITSTORE_LOCAL_PATH", "GITSTORE_LOCAL_PATH"); ok {
			cfg.Git.LocalPath = value
		}
		if cfg.Git.LocalPath == "" && writableBase != "" {
			cfg.Git.LocalPath = writableBase
		}
		return cfg
	}

	// Parse Object store configuration
	if storeType == "s3" || storeType == "object" || storeType == "minio" {
		cfg.Type = TypeObject
	}
	if value, ok := lookupEnv("LLM_MUX_OBJECTSTORE_ENDPOINT", "OBJECTSTORE_ENDPOINT"); ok {
		cfg.Type = TypeObject
		cfg.Object.Endpoint = value
	}
	if cfg.Type == TypeObject {
		if value, ok := lookupEnv("LLM_MUX_OBJECTSTORE_ACCESS_KEY", "OBJECTSTORE_ACCESS_KEY"); ok {
			cfg.Object.AccessKey = value
		}
		if value, ok := lookupEnv("LLM_MUX_OBJECTSTORE_SECRET_KEY", "OBJECTSTORE_SECRET_KEY"); ok {
			cfg.Object.SecretKey = value
		}
		if value, ok := lookupEnv("LLM_MUX_OBJECTSTORE_BUCKET", "OBJECTSTORE_BUCKET"); ok {
			cfg.Object.Bucket = value
		}
		if value, ok := lookupEnv("LLM_MUX_OBJECTSTORE_LOCAL_PATH", "OBJECTSTORE_LOCAL_PATH"); ok {
			cfg.Object.LocalRoot = value
		}
		if cfg.Object.LocalRoot == "" && writableBase != "" {
			cfg.Object.LocalRoot = writableBase
		}
		return cfg
	}

	return cfg
}

// IsConfigured returns true if any store backend is configured.
func (c StoreConfig) IsConfigured() bool {
	return c.Type != TypeNone
}

// IsPostgres returns true if PostgreSQL backend is configured.
func (c StoreConfig) IsPostgres() bool {
	return c.Type == TypePostgres
}

// IsGit returns true if Git backend is configured.
func (c StoreConfig) IsGit() bool {
	return c.Type == TypeGit
}

// IsObject returns true if object storage backend is configured.
func (c StoreConfig) IsObject() bool {
	return c.Type == TypeObject
}
