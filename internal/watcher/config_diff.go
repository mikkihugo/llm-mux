package watcher

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/nghyane/llm-mux/internal/config"
)

// buildConfigChangeDetails computes a redacted, human-readable list of config changes.
// It avoids printing secrets (like API keys) and focuses on structural or non-sensitive fields.
func buildConfigChangeDetails(oldCfg, newCfg *config.Config) []string {
	changes := make([]string, 0, 16)
	if oldCfg == nil || newCfg == nil {
		return changes
	}

	// Simple scalars
	if oldCfg.Port != newCfg.Port {
		changes = append(changes, fmt.Sprintf("port: %d -> %d", oldCfg.Port, newCfg.Port))
	}
	if oldCfg.AuthDir != newCfg.AuthDir {
		changes = append(changes, fmt.Sprintf("auth-dir: %s -> %s", oldCfg.AuthDir, newCfg.AuthDir))
	}
	if oldCfg.Debug != newCfg.Debug {
		changes = append(changes, fmt.Sprintf("debug: %t -> %t", oldCfg.Debug, newCfg.Debug))
	}
	if oldCfg.LoggingToFile != newCfg.LoggingToFile {
		changes = append(changes, fmt.Sprintf("logging-to-file: %t -> %t", oldCfg.LoggingToFile, newCfg.LoggingToFile))
	}
	if oldCfg.UsageStatisticsEnabled != newCfg.UsageStatisticsEnabled {
		changes = append(changes, fmt.Sprintf("usage-statistics-enabled: %t -> %t", oldCfg.UsageStatisticsEnabled, newCfg.UsageStatisticsEnabled))
	}
	if oldCfg.DisableCooling != newCfg.DisableCooling {
		changes = append(changes, fmt.Sprintf("disable-cooling: %t -> %t", oldCfg.DisableCooling, newCfg.DisableCooling))
	}
	if oldCfg.RequestLog != newCfg.RequestLog {
		changes = append(changes, fmt.Sprintf("request-log: %t -> %t", oldCfg.RequestLog, newCfg.RequestLog))
	}
	if oldCfg.RequestRetry != newCfg.RequestRetry {
		changes = append(changes, fmt.Sprintf("request-retry: %d -> %d", oldCfg.RequestRetry, newCfg.RequestRetry))
	}
	if oldCfg.MaxRetryInterval != newCfg.MaxRetryInterval {
		changes = append(changes, fmt.Sprintf("max-retry-interval: %d -> %d", oldCfg.MaxRetryInterval, newCfg.MaxRetryInterval))
	}
	if oldCfg.ProxyURL != newCfg.ProxyURL {
		changes = append(changes, fmt.Sprintf("proxy-url: %s -> %s", oldCfg.ProxyURL, newCfg.ProxyURL))
	}
	if oldCfg.WebsocketAuth != newCfg.WebsocketAuth {
		changes = append(changes, fmt.Sprintf("ws-auth: %t -> %t", oldCfg.WebsocketAuth, newCfg.WebsocketAuth))
	}

	// Quota-exceeded behavior
	if oldCfg.QuotaExceeded.SwitchProject != newCfg.QuotaExceeded.SwitchProject {
		changes = append(changes, fmt.Sprintf("quota-exceeded.switch-project: %t -> %t", oldCfg.QuotaExceeded.SwitchProject, newCfg.QuotaExceeded.SwitchProject))
	}
	if oldCfg.QuotaExceeded.SwitchPreviewModel != newCfg.QuotaExceeded.SwitchPreviewModel {
		changes = append(changes, fmt.Sprintf("quota-exceeded.switch-preview-model: %t -> %t", oldCfg.QuotaExceeded.SwitchPreviewModel, newCfg.QuotaExceeded.SwitchPreviewModel))
	}

	// API keys (redacted) and counts
	if len(oldCfg.APIKeys) != len(newCfg.APIKeys) {
		changes = append(changes, fmt.Sprintf("api-keys count: %d -> %d", len(oldCfg.APIKeys), len(newCfg.APIKeys)))
	} else if !reflect.DeepEqual(trimStrings(oldCfg.APIKeys), trimStrings(newCfg.APIKeys)) {
		changes = append(changes, "api-keys: values updated (count unchanged, redacted)")
	}

	// Providers
	if len(oldCfg.Providers) != len(newCfg.Providers) {
		changes = append(changes, fmt.Sprintf("providers count: %d -> %d", len(oldCfg.Providers), len(newCfg.Providers)))
	} else if !reflect.DeepEqual(oldCfg.Providers, newCfg.Providers) {
		changes = append(changes, "providers: updated")
	}

	// AmpCode settings (redacted where needed)
	oldAmpURL := strings.TrimSpace(oldCfg.AmpCode.UpstreamURL)
	newAmpURL := strings.TrimSpace(newCfg.AmpCode.UpstreamURL)
	if oldAmpURL != newAmpURL {
		changes = append(changes, fmt.Sprintf("ampcode.upstream-url: %s -> %s", oldAmpURL, newAmpURL))
	}
	oldAmpKey := strings.TrimSpace(oldCfg.AmpCode.UpstreamAPIKey)
	newAmpKey := strings.TrimSpace(newCfg.AmpCode.UpstreamAPIKey)
	switch {
	case oldAmpKey == "" && newAmpKey != "":
		changes = append(changes, "ampcode.upstream-api-key: added")
	case oldAmpKey != "" && newAmpKey == "":
		changes = append(changes, "ampcode.upstream-api-key: removed")
	case oldAmpKey != newAmpKey:
		changes = append(changes, "ampcode.upstream-api-key: updated")
	}
	if oldCfg.AmpCode.RestrictManagementToLocalhost != newCfg.AmpCode.RestrictManagementToLocalhost {
		changes = append(changes, fmt.Sprintf("ampcode.restrict-management-to-localhost: %t -> %t", oldCfg.AmpCode.RestrictManagementToLocalhost, newCfg.AmpCode.RestrictManagementToLocalhost))
	}
	oldMappings := summarizeAmpModelMappings(oldCfg.AmpCode.ModelMappings)
	newMappings := summarizeAmpModelMappings(newCfg.AmpCode.ModelMappings)
	if oldMappings.hash != newMappings.hash {
		changes = append(changes, fmt.Sprintf("ampcode.model-mappings: updated (%d -> %d entries)", oldMappings.count, newMappings.count))
	}

	if entries, _ := diffOAuthExcludedModelChanges(oldCfg.OAuthExcludedModels, newCfg.OAuthExcludedModels); len(entries) > 0 {
		changes = append(changes, entries...)
	}

	// Remote management (never print the key)
	if oldCfg.RemoteManagement.AllowRemote != newCfg.RemoteManagement.AllowRemote {
		changes = append(changes, fmt.Sprintf("remote-management.allow-remote: %t -> %t", oldCfg.RemoteManagement.AllowRemote, newCfg.RemoteManagement.AllowRemote))
	}

	return changes
}
