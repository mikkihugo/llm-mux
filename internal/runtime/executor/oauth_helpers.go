package executor

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/nghyane/llm-mux/internal/json"
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

func TokenExpiry(metadata map[string]any) time.Time {
	if metadata == nil {
		return time.Time{}
	}

	if expStr, ok := metadata["expired"].(string); ok {
		expStr = strings.TrimSpace(expStr)
		if expStr != "" {
			if parsed, errParse := time.Parse(time.RFC3339, expStr); errParse == nil {
				return parsed
			}
		}
	}

	expiresIn, hasExpires := Int64Value(metadata["expires_in"])
	tsMs, hasTimestamp := Int64Value(metadata["timestamp"])
	if hasExpires && hasTimestamp {
		return time.Unix(0, tsMs*int64(time.Millisecond)).Add(time.Duration(expiresIn) * time.Second)
	}

	return time.Time{}
}

func MetaStringValue(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	if v, ok := metadata[key]; ok {
		switch typed := v.(type) {
		case string:
			return strings.TrimSpace(typed)
		case []byte:
			return strings.TrimSpace(string(typed))
		}
	}
	return ""
}

func Int64Value(value any) (int64, bool) {
	switch typed := value.(type) {
	case int:
		return int64(typed), true
	case int64:
		return typed, true
	case float64:
		return int64(typed), true
	case json.Number:
		if i, errParse := typed.Int64(); errParse == nil {
			return i, true
		}
	case string:
		if strings.TrimSpace(typed) == "" {
			return 0, false
		}
		if i, errParse := strconv.ParseInt(strings.TrimSpace(typed), 10, 64); errParse == nil {
			return i, true
		}
	}
	return 0, false
}

func ResolveHost(base string) string {
	parsed, errParse := url.Parse(base)
	if errParse != nil {
		return ""
	}
	if parsed.Host != "" {
		return parsed.Host
	}
	return strings.TrimPrefix(strings.TrimPrefix(base, "https://"), "http://")
}

func CloneMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func AttrStringValue(attrs map[string]string, key string) string {
	if attrs == nil {
		return ""
	}
	return strings.TrimSpace(attrs[key])
}
