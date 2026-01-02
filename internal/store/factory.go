package store

import (
	"context"
	"fmt"
	"time"

	"github.com/nghyane/llm-mux/internal/provider"
)

const bootstrapTimeout = 30 * time.Second

// StoreResult holds the initialized store and its resolved paths.
type StoreResult struct {
	Store      provider.Store
	ConfigPath string
	AuthDir    string
	StoreType  StoreType
}

// NewStore creates and initializes a store based on the provided configuration.
// For TypeNone, it returns a nil Store indicating file-based fallback.
func NewStore(ctx context.Context, cfg StoreConfig) (*StoreResult, error) {
	switch cfg.Type {
	case TypePostgres:
		return newPostgresStore(ctx, cfg.Postgres)
	case TypeObject:
		return newObjectStore(ctx, cfg.Object)
	case TypeGit:
		return newGitStore(cfg.Git)
	case TypeNone:
		return &StoreResult{
			Store:     nil,
			StoreType: TypeNone,
		}, nil
	default:
		return nil, fmt.Errorf("store: unknown store type: %s", cfg.Type)
	}
}

func newPostgresStore(ctx context.Context, cfg PostgresStoreConfig) (*StoreResult, error) {
	store, err := NewPostgresStore(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("store: create postgres store: %w", err)
	}

	bootstrapCtx, cancel := context.WithTimeout(ctx, bootstrapTimeout)
	defer cancel()

	if err := store.Bootstrap(bootstrapCtx); err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("store: bootstrap postgres store: %w", err)
	}

	return &StoreResult{
		Store:      store,
		ConfigPath: store.ConfigPath(),
		AuthDir:    store.AuthDir(),
		StoreType:  TypePostgres,
	}, nil
}

func newObjectStore(ctx context.Context, cfg ObjectStoreConfig) (*StoreResult, error) {
	store, err := NewObjectTokenStore(cfg)
	if err != nil {
		return nil, fmt.Errorf("store: create object store: %w", err)
	}

	bootstrapCtx, cancel := context.WithTimeout(ctx, bootstrapTimeout)
	defer cancel()

	if err := store.Bootstrap(bootstrapCtx); err != nil {
		return nil, fmt.Errorf("store: bootstrap object store: %w", err)
	}

	return &StoreResult{
		Store:      store,
		ConfigPath: store.ConfigPath(),
		AuthDir:    store.AuthDir(),
		StoreType:  TypeObject,
	}, nil
}

func newGitStore(cfg GitStoreConfig) (*StoreResult, error) {
	store := NewGitTokenStore(cfg.RemoteURL, cfg.Username, cfg.Password)
	if cfg.LocalPath != "" {
		store.SetBaseDir(cfg.LocalPath)
	}

	if err := store.EnsureRepository(); err != nil {
		return nil, fmt.Errorf("store: ensure git repository: %w", err)
	}

	return &StoreResult{
		Store:      store,
		ConfigPath: store.ConfigPath(),
		AuthDir:    store.AuthDir(),
		StoreType:  TypeGit,
	}, nil
}
