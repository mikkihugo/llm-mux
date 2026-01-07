package watcher

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/nghyane/llm-mux/internal/config"
	"github.com/nghyane/llm-mux/internal/json"
	"github.com/nghyane/llm-mux/internal/provider"
)

func computeExcludedModelsHash(excluded []string) string {
	if len(excluded) == 0 {
		return ""
	}
	normalized := make([]string, 0, len(excluded))
	for _, entry := range excluded {
		if trimmed := strings.TrimSpace(entry); trimmed != "" {
			normalized = append(normalized, strings.ToLower(trimmed))
		}
	}
	if len(normalized) == 0 {
		return ""
	}
	sort.Strings(normalized)
	data, err := json.Marshal(normalized)
	if err != nil || len(data) == 0 {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

type excludedModelsSummary struct {
	hash  string
	count int
}

func summarizeExcludedModels(list []string) excludedModelsSummary {
	if len(list) == 0 {
		return excludedModelsSummary{}
	}
	seen := make(map[string]struct{}, len(list))
	normalized := make([]string, 0, len(list))
	for _, entry := range list {
		if trimmed := strings.ToLower(strings.TrimSpace(entry)); trimmed != "" {
			if _, exists := seen[trimmed]; exists {
				continue
			}
			seen[trimmed] = struct{}{}
			normalized = append(normalized, trimmed)
		}
	}
	sort.Strings(normalized)
	return excludedModelsSummary{
		hash:  computeExcludedModelsHash(normalized),
		count: len(normalized),
	}
}

type ampModelMappingsSummary struct {
	hash  string
	count int
}

func summarizeAmpModelMappings(mappings []config.AmpModelMapping) ampModelMappingsSummary {
	if len(mappings) == 0 {
		return ampModelMappingsSummary{}
	}
	entries := make([]string, 0, len(mappings))
	for _, mapping := range mappings {
		from := strings.TrimSpace(mapping.From)
		to := strings.TrimSpace(mapping.To)
		if from == "" && to == "" {
			continue
		}
		entries = append(entries, from+"->"+to)
	}
	if len(entries) == 0 {
		return ampModelMappingsSummary{}
	}
	sort.Strings(entries)
	sum := sha256.Sum256([]byte(strings.Join(entries, "|")))
	return ampModelMappingsSummary{
		hash:  hex.EncodeToString(sum[:]),
		count: len(entries),
	}
}

func summarizeOAuthExcludedModels(entries map[string][]string) map[string]excludedModelsSummary {
	if len(entries) == 0 {
		return nil
	}
	out := make(map[string]excludedModelsSummary, len(entries))
	for k, v := range entries {
		key := strings.ToLower(strings.TrimSpace(k))
		if key == "" {
			continue
		}
		out[key] = summarizeExcludedModels(v)
	}
	return out
}

func diffOAuthExcludedModelChanges(oldMap, newMap map[string][]string) ([]string, []string) {
	oldSummary := summarizeOAuthExcludedModels(oldMap)
	newSummary := summarizeOAuthExcludedModels(newMap)
	keys := make(map[string]struct{}, len(oldSummary)+len(newSummary))
	for k := range oldSummary {
		keys[k] = struct{}{}
	}
	for k := range newSummary {
		keys[k] = struct{}{}
	}
	changes := make([]string, 0, len(keys))
	affected := make([]string, 0, len(keys))
	for key := range keys {
		oldInfo, okOld := oldSummary[key]
		newInfo, okNew := newSummary[key]
		switch {
		case okOld && !okNew:
			changes = append(changes, fmt.Sprintf("oauth-excluded-models[%s]: removed", key))
			affected = append(affected, key)
		case !okOld && okNew:
			changes = append(changes, fmt.Sprintf("oauth-excluded-models[%s]: added (%d entries)", key, newInfo.count))
			affected = append(affected, key)
		case okOld && okNew && oldInfo.hash != newInfo.hash:
			changes = append(changes, fmt.Sprintf("oauth-excluded-models[%s]: updated (%d -> %d entries)", key, oldInfo.count, newInfo.count))
			affected = append(affected, key)
		}
	}
	sort.Strings(changes)
	sort.Strings(affected)
	return changes, affected
}

func applyAuthExcludedModelsMeta(auth *provider.Auth, cfg *config.Config, perKey []string, authKind string) {
	if auth == nil || cfg == nil {
		return
	}
	authKindKey := strings.ToLower(strings.TrimSpace(authKind))
	seen := make(map[string]struct{})
	add := func(list []string) {
		for _, entry := range list {
			if trimmed := strings.TrimSpace(entry); trimmed != "" {
				key := strings.ToLower(trimmed)
				if _, exists := seen[key]; exists {
					continue
				}
				seen[key] = struct{}{}
			}
		}
	}
	if authKindKey == "apikey" {
		add(perKey)
	} else if cfg.OAuthExcludedModels != nil {
		providerKey := strings.ToLower(strings.TrimSpace(auth.Provider))
		add(cfg.OAuthExcludedModels[providerKey])
	}
	combined := make([]string, 0, len(seen))
	for k := range seen {
		combined = append(combined, k)
	}
	sort.Strings(combined)
	hash := computeExcludedModelsHash(combined)
	if auth.Attributes == nil {
		auth.Attributes = make(map[string]string)
	}
	if hash != "" {
		auth.Attributes["excluded_models_hash"] = hash
	}
	if authKind != "" {
		auth.Attributes["auth_kind"] = authKind
	}
}

func trimStrings(in []string) []string {
	out := make([]string, len(in))
	for i := range in {
		out[i] = strings.TrimSpace(in[i])
	}
	return out
}

func addConfigHeadersToAttrs(headers map[string]string, attrs map[string]string) {
	if len(headers) == 0 || attrs == nil {
		return
	}
	for hk, hv := range headers {
		key := strings.TrimSpace(hk)
		val := strings.TrimSpace(hv)
		if key == "" || val == "" {
			continue
		}
		attrs["header:"+key] = val
	}
}

// materialMetadataKeys defines credential fields that require model re-registration when changed.
var materialMetadataKeys = []string{
	"refresh_token",
	"client_id",
	"client_secret",
}

// volatileAttributeKeys defines runtime/transient attribute keys ignored for material change detection.
var volatileAttributeKeys = map[string]struct{}{
	"last_error":        {},
	"status_message":    {},
	"last_refreshed_at": {},
}

// isMaterialChange returns true if the auth change requires model re-registration.
// Material changes: Provider, credentials (refresh_token, client_id, client_secret), Attributes.
// Volatile changes (access_token, expiry, quota, timestamps) return false.
func isMaterialChange(old, new *provider.Auth) bool {
	if old == nil || new == nil {
		return old != new
	}
	if old.Provider != new.Provider {
		return true
	}
	for _, key := range materialMetadataKeys {
		if metadataString(old.Metadata, key) != metadataString(new.Metadata, key) {
			return true
		}
	}
	return !equalMaterialAttributes(old.Attributes, new.Attributes)
}

func metadataString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func equalMaterialAttributes(a, b map[string]string) bool {
	countA := countMaterialAttrs(a)
	countB := countMaterialAttrs(b)
	if countA != countB {
		return false
	}
	for k, v := range a {
		if _, volatile := volatileAttributeKeys[k]; volatile {
			continue
		}
		if b[k] != v {
			return false
		}
	}
	return true
}

func countMaterialAttrs(attrs map[string]string) int {
	count := 0
	for k := range attrs {
		if _, volatile := volatileAttributeKeys[k]; !volatile {
			count++
		}
	}
	return count
}
