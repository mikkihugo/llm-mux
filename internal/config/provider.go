package config

import "strings"

// ProviderType defines the type of API provider.
type ProviderType string

const (
	// ProviderTypeGemini uses Google's Gemini API with dynamic model discovery.
	ProviderTypeGemini ProviderType = "gemini"

	// ProviderTypeAnthropic uses Anthropic's Claude API.
	ProviderTypeAnthropic ProviderType = "anthropic"

	// ProviderTypeOpenAI uses OpenAI-compatible APIs (OpenAI, DeepSeek, Groq, etc.).
	ProviderTypeOpenAI ProviderType = "openai"

	// ProviderTypeVertexCompat uses Vertex AI-compatible endpoints (zenmux, etc.).
	ProviderTypeVertexCompat ProviderType = "vertex-compat"
)

// Provider represents a unified API provider configuration.
// This replaces the legacy gemini-api-key, claude-api-key, codex-api-key,
// openai-compatibility, and vertex-api-key configurations.
type Provider struct {
	// Type specifies the provider type (gemini, anthropic, openai, vertex-compat).
	Type ProviderType `yaml:"type" json:"type"`

	// Name is a display name for this provider instance.
	// Required for openai type when multiple providers use the same type.
	// Examples: "openai", "deepseek", "groq", "together"
	Name string `yaml:"name,omitempty" json:"name,omitempty"`

	// Enabled allows disabling a provider without removing it. Default: true.
	Enabled *bool `yaml:"enabled,omitempty" json:"enabled,omitempty"`

	// APIKey is the primary API key for this provider.
	// For providers supporting multiple keys, use APIKeys instead.
	APIKey string `yaml:"api-key,omitempty" json:"api-key,omitempty"`

	// APIKeys allows multiple API keys with per-key proxy settings.
	// Used for load balancing across multiple keys.
	APIKeys []ProviderAPIKey `yaml:"api-keys,omitempty" json:"api-keys,omitempty"`

	// BaseURL is the API endpoint URL.
	// Required for: openai, vertex-compat
	// Optional for: gemini, anthropic (uses default if not set)
	BaseURL string `yaml:"base-url,omitempty" json:"base-url,omitempty"`

	// ProxyURL sets a proxy for this provider's requests.
	ProxyURL string `yaml:"proxy-url,omitempty" json:"proxy-url,omitempty"`

	// Headers adds custom HTTP headers to requests.
	Headers map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`

	// Models defines available models for this provider.
	// Required for: openai, vertex-compat
	// Optional for: gemini, anthropic (uses built-in registry if not set)
	Models []ProviderModel `yaml:"models,omitempty" json:"models,omitempty"`

	// ExcludedModels lists model names to exclude from this provider.
	ExcludedModels []string `yaml:"excluded-models,omitempty" json:"excluded-models,omitempty"`
}

// ProviderAPIKey represents an API key with optional per-key settings.
type ProviderAPIKey struct {
	// Key is the API key value.
	Key string `yaml:"key" json:"key"`

	// ProxyURL overrides the provider's proxy for this key.
	ProxyURL string `yaml:"proxy-url,omitempty" json:"proxy-url,omitempty"`
}

// ProviderModel defines a model available from this provider.
type ProviderModel struct {
	// Name is the actual model name used in API requests.
	Name string `yaml:"name" json:"name"`

	// Alias is an optional alternative name for this model.
	// If set, both Name and Alias can be used to reference this model.
	Alias string `yaml:"alias,omitempty" json:"alias,omitempty"`
}

// IsEnabled returns true if the provider is enabled (default: true).
func (p *Provider) IsEnabled() bool {
	if p.Enabled == nil {
		return true
	}
	return *p.Enabled
}

// GetAPIKeys returns all API keys for this provider.
// If APIKey is set and APIKeys is empty, returns APIKey as a single entry.
func (p *Provider) GetAPIKeys() []ProviderAPIKey {
	if len(p.APIKeys) > 0 {
		return p.APIKeys
	}
	if p.APIKey != "" {
		return []ProviderAPIKey{{Key: p.APIKey, ProxyURL: p.ProxyURL}}
	}
	return nil
}

// GetDisplayName returns the display name for this provider.
// Falls back to type if name is not set.
func (p *Provider) GetDisplayName() string {
	if p.Name != "" {
		return p.Name
	}
	return string(p.Type)
}

// Validate checks if the provider configuration is valid.
func (p *Provider) Validate() error {
	if p.Type == "" {
		return &ProviderValidationError{Field: "type", Message: "type is required"}
	}

	// Check API key
	if p.APIKey == "" && len(p.APIKeys) == 0 {
		return &ProviderValidationError{Field: "api-key", Message: "api-key or api-keys is required"}
	}

	// Type-specific validation
	switch p.Type {
	case ProviderTypeOpenAI, ProviderTypeVertexCompat:
		if p.BaseURL == "" {
			return &ProviderValidationError{Field: "base-url", Message: "base-url is required for " + string(p.Type)}
		}
		if len(p.Models) == 0 {
			return &ProviderValidationError{Field: "models", Message: "models is required for " + string(p.Type)}
		}
	}

	return nil
}

// ProviderValidationError represents a validation error for provider config.
type ProviderValidationError struct {
	Field   string
	Message string
}

func (e *ProviderValidationError) Error() string {
	return "provider config error: " + e.Field + ": " + e.Message
}

// SanitizeProviders normalizes and validates the providers list.
func SanitizeProviders(providers []Provider) []Provider {
	if len(providers) == 0 {
		return nil
	}

	result := make([]Provider, 0, len(providers))
	seen := make(map[string]struct{})

	for i := range providers {
		p := &providers[i]

		// Skip disabled providers
		if !p.IsEnabled() {
			continue
		}

		// Normalize fields
		p.Type = ProviderType(strings.TrimSpace(strings.ToLower(string(p.Type))))
		p.Name = strings.TrimSpace(p.Name)
		p.APIKey = strings.TrimSpace(p.APIKey)
		p.BaseURL = strings.TrimRight(strings.TrimSpace(p.BaseURL), "/")
		p.ProxyURL = strings.TrimSpace(p.ProxyURL)
		p.Headers = NormalizeHeaders(p.Headers)

		// Normalize API keys
		validKeys := make([]ProviderAPIKey, 0, len(p.APIKeys))
		for _, k := range p.APIKeys {
			k.Key = strings.TrimSpace(k.Key)
			k.ProxyURL = strings.TrimSpace(k.ProxyURL)
			if k.Key != "" {
				validKeys = append(validKeys, k)
			}
		}
		p.APIKeys = validKeys

		// Normalize models
		validModels := make([]ProviderModel, 0, len(p.Models))
		for _, m := range p.Models {
			m.Name = strings.TrimSpace(m.Name)
			m.Alias = strings.TrimSpace(m.Alias)
			if m.Name != "" {
				validModels = append(validModels, m)
			}
		}
		p.Models = validModels

		// Validate
		if err := p.Validate(); err != nil {
			continue
		}

		// Deduplicate by type+name+baseurl
		uniqueKey := string(p.Type) + "|" + p.Name + "|" + p.BaseURL
		if _, exists := seen[uniqueKey]; exists {
			continue
		}
		seen[uniqueKey] = struct{}{}

		result = append(result, *p)
	}

	return result
}

// GetProvidersByType returns all providers of the specified type.
func (cfg *Config) GetProvidersByType(t ProviderType) []Provider {
	if cfg == nil {
		return nil
	}
	var result []Provider
	for _, p := range cfg.Providers {
		if p.Type == t {
			result = append(result, p)
		}
	}
	return result
}

// GetProviderByName returns a provider by its display name.
func (cfg *Config) GetProviderByName(name string) *Provider {
	if cfg == nil {
		return nil
	}
	for i := range cfg.Providers {
		if cfg.Providers[i].GetDisplayName() == name {
			return &cfg.Providers[i]
		}
	}
	return nil
}
