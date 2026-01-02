// Package config provides configuration management for llm-mux.
package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// ParsedDSN represents a parsed database connection string.
type ParsedDSN struct {
	// Backend is the database type: "sqlite" or "postgres".
	Backend string
	// Path is the filesystem path for SQLite databases.
	Path string
	// URL is the full connection URL for Postgres databases.
	URL string
}

// ParseDSN parses a DSN string with URI scheme detection.
// Supported schemes:
//   - sqlite:///absolute/path or sqlite://relative/path or sqlite://~/home/path
//   - postgres://user:pass@host:port/db or postgresql://...
//
// Returns nil if DSN is empty (disabled).
func ParseDSN(dsn string) (*ParsedDSN, error) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return nil, nil // Disabled
	}

	// Handle sqlite:// scheme
	if strings.HasPrefix(dsn, "sqlite://") {
		path := strings.TrimPrefix(dsn, "sqlite://")
		// Handle query parameters (strip them for path)
		if idx := strings.Index(path, "?"); idx > 0 {
			path = path[:idx]
		}
		path = expandPath(path)
		if path == "" {
			return nil, fmt.Errorf("sqlite DSN requires a path: sqlite:///path/to/db.sqlite")
		}
		return &ParsedDSN{Backend: "sqlite", Path: path}, nil
	}

	// Handle postgres:// or postgresql:// scheme
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		if _, err := url.Parse(dsn); err != nil {
			return nil, fmt.Errorf("invalid postgres DSN: %w", err)
		}
		return &ParsedDSN{Backend: "postgres", URL: dsn}, nil
	}

	return nil, fmt.Errorf("unsupported DSN scheme: %q (use sqlite:// or postgres://)", dsn)
}

// expandPath expands ~ to home directory and resolves relative paths.
func expandPath(path string) string {
	if path == "" {
		return ""
	}
	// Expand ~ to home directory
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, path[2:])
		}
	}
	// Expand $XDG_CONFIG_HOME and other env vars
	path = os.ExpandEnv(path)
	return path
}

// IsSQLite returns true if the parsed DSN is for SQLite.
func (p *ParsedDSN) IsSQLite() bool {
	return p != nil && p.Backend == "sqlite"
}

// IsPostgres returns true if the parsed DSN is for Postgres.
func (p *ParsedDSN) IsPostgres() bool {
	return p != nil && p.Backend == "postgres"
}
