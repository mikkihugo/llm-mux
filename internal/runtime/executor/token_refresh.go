package executor

import (
	"context"
	"time"

	"golang.org/x/sync/singleflight"
)

const defaultTokenRefreshTimeout = 30 * time.Second

type TokenRefreshGroup struct {
	sf      singleflight.Group
	timeout time.Duration
}

func NewTokenRefreshGroup() *TokenRefreshGroup {
	return &TokenRefreshGroup{timeout: defaultTokenRefreshTimeout}
}

func (g *TokenRefreshGroup) Do(key string, fn func(ctx context.Context) (any, error)) (any, error) {
	result, err, _ := g.sf.Do(key, func() (any, error) {
		ctx, cancel := context.WithTimeout(context.Background(), g.timeout)
		defer cancel()
		return fn(ctx)
	})
	return result, err
}
