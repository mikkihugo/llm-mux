// Package registry provides model builder for clean, chainable model definitions.
// This builder is used across all providers for consistent model metadata.
package registry

// ModelBuilder provides a fluent API for constructing ModelInfo instances.
type ModelBuilder struct {
	info *ModelInfo
}

// =============================================================================
// Shared Defaults
// =============================================================================

var (
	defaultGeminiMethods = []string{"generateContent", "countTokens", "createCachedContent", "batchGenerateContent"}
	defaultClaudeMethods = []string{"generateContent"}
)

const (
	geminiInputLimit  = 1048576
	geminiOutputLimit = 65536
	claudeInputLimit  = 200000
	claudeOutputLimit = 64000
)

// =============================================================================
// Factory Functions - Create builders with provider defaults
// =============================================================================

// Gemini creates a builder for Gemini models (AI Studio, Gemini CLI, Vertex, Antigravity).
func Gemini(id string) *ModelBuilder {
	return &ModelBuilder{info: &ModelInfo{
		ID:                         id,
		Object:                     "model",
		OwnedBy:                    "google",
		Type:                       "gemini",
		Name:                       "models/" + id,
		InputTokenLimit:            geminiInputLimit,
		OutputTokenLimit:           geminiOutputLimit,
		SupportedGenerationMethods: defaultGeminiMethods,
	}}
}

// Claude creates a builder for native Claude API models.
func Claude(id string) *ModelBuilder {
	return &ModelBuilder{info: &ModelInfo{
		ID:               id,
		Object:           "model",
		OwnedBy:          "anthropic",
		Type:             "claude",
		InputTokenLimit:  claudeInputLimit,
		OutputTokenLimit: claudeOutputLimit,
	}}
}

// ClaudeVia creates a builder for Claude models accessed via another provider.
func ClaudeVia(id, provider string) *ModelBuilder {
	return &ModelBuilder{info: &ModelInfo{
		ID:                         id,
		Object:                     "model",
		OwnedBy:                    "anthropic",
		Type:                       provider,
		CanonicalID:                id,
		Name:                       id,
		InputTokenLimit:            claudeInputLimit,
		OutputTokenLimit:           claudeOutputLimit,
		SupportedGenerationMethods: defaultClaudeMethods,
	}}
}

// OpenAI creates a builder for OpenAI/Codex models.
func OpenAI(id string) *ModelBuilder {
	return &ModelBuilder{info: &ModelInfo{
		ID:                  id,
		Object:              "model",
		OwnedBy:             "openai",
		Type:                "codex",
		ContextLength:       400000,
		MaxCompletionTokens: 128000,
	}}
}

// Kiro creates a builder for Kiro/Amazon Q models.
func Kiro(id string) *ModelBuilder {
	return &ModelBuilder{info: &ModelInfo{
		ID:      id,
		Object:  "model",
		OwnedBy: "kiro",
		Type:    "kiro",
	}}
}

// Copilot creates a builder for GitHub Copilot models.
func Copilot(id string) *ModelBuilder {
	return &ModelBuilder{info: &ModelInfo{
		ID:       id,
		Object:   "model",
		OwnedBy:  "github-copilot",
		Type:     "github-copilot",
		Priority: 2, // Fallback
	}}
}

// IFlow creates a builder for iFlow models.
func IFlow(id string) *ModelBuilder {
	return &ModelBuilder{info: &ModelInfo{
		ID:      id,
		Object:  "model",
		OwnedBy: "iflow",
		Type:    "iflow",
	}}
}

// Cline creates a builder for Cline models.
func Cline(id string) *ModelBuilder {
	return &ModelBuilder{info: &ModelInfo{
		ID:      id,
		Object:  "model",
		OwnedBy: "cline",
		Type:    "cline",
	}}
}

// Qwen creates a builder for Qwen models.
func Qwen(id string) *ModelBuilder {
	return &ModelBuilder{info: &ModelInfo{
		ID:      id,
		Object:  "model",
		OwnedBy: "qwen",
		Type:    "qwen",
	}}
}

// =============================================================================
// Chainable Methods
// =============================================================================

// Upstream sets the upstream model name for alias mapping.
func (b *ModelBuilder) Upstream(name string) *ModelBuilder {
	b.info.UpstreamName = name
	return b
}

// Hidden marks the model as hidden from listings.
func (b *ModelBuilder) Hidden() *ModelBuilder {
	b.info.Hidden = true
	return b
}

// Thinking sets thinking support with min/max budget (dynamic allowed).
func (b *ModelBuilder) Thinking(min, max int) *ModelBuilder {
	b.info.Thinking = &ThinkingSupport{Min: min, Max: max, DynamicAllowed: true}
	return b
}

// ThinkingFull sets thinking support with all options.
func (b *ModelBuilder) ThinkingFull(min, max int, zeroAllowed, dynamicAllowed bool) *ModelBuilder {
	b.info.Thinking = &ThinkingSupport{
		Min: min, Max: max, ZeroAllowed: zeroAllowed, DynamicAllowed: dynamicAllowed,
	}
	return b
}

// Limits sets input and output token limits.
func (b *ModelBuilder) Limits(input, output int) *ModelBuilder {
	b.info.InputTokenLimit = input
	b.info.OutputTokenLimit = output
	return b
}

// Context sets context length and max completion tokens.
func (b *ModelBuilder) Context(ctx, maxComp int) *ModelBuilder {
	b.info.ContextLength = ctx
	b.info.MaxCompletionTokens = maxComp
	return b
}

// Display sets the display name.
func (b *ModelBuilder) Display(name string) *ModelBuilder {
	b.info.DisplayName = name
	return b
}

// Desc sets the description.
func (b *ModelBuilder) Desc(desc string) *ModelBuilder {
	b.info.Description = desc
	return b
}

// Version sets the version string.
func (b *ModelBuilder) Version(v string) *ModelBuilder {
	b.info.Version = v
	return b
}

// Created sets the created timestamp.
func (b *ModelBuilder) Created(ts int64) *ModelBuilder {
	b.info.Created = ts
	return b
}

// Canonical sets the canonical ID for cross-provider routing.
func (b *ModelBuilder) Canonical(id string) *ModelBuilder {
	b.info.CanonicalID = id
	return b
}

// Owner overrides the owned_by field.
func (b *ModelBuilder) Owner(owner string) *ModelBuilder {
	b.info.OwnedBy = owner
	return b
}

// ProviderType overrides the type field.
func (b *ModelBuilder) ProviderType(t string) *ModelBuilder {
	b.info.Type = t
	return b
}

// Priority sets routing priority (lower = higher priority).
func (b *ModelBuilder) Priority(p int) *ModelBuilder {
	b.info.Priority = p
	return b
}

// B returns the constructed ModelInfo (short for Build).
func (b *ModelBuilder) B() *ModelInfo {
	return b.info
}

// Build returns the constructed ModelInfo.
func (b *ModelBuilder) Build() *ModelInfo {
	return b.info
}
