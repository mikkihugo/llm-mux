// Package registry provides model family definitions for cross-provider routing.
// Model families allow clients to use a canonical model name (e.g., "claude-sonnet-4-5")
// and have it automatically routed to the appropriate provider-specific model ID.
package registry

import (
	"math/rand"
	"sort"
)

// FamilyMember represents a provider-specific model within a family.
type FamilyMember struct {
	Provider string // Provider type (e.g., "kiro", "antigravity", "claude")
	ModelID  string // Provider-specific model ID
	Priority int    // Priority level (1 = highest). Same priority = load balanced
}

// ModelFamilies maps canonical model names to their provider-specific variants.
// Priority determines selection order. Same priority = random selection among available.
var ModelFamilies = map[string][]FamilyMember{
	// Claude Sonnet 4.5 family
	"claude-sonnet-4-5": {
		{Provider: "kiro", ModelID: "claude-sonnet-4-5", Priority: 1},
		{Provider: "antigravity", ModelID: "gemini-claude-sonnet-4-5", Priority: 1},
		{Provider: "claude", ModelID: "claude-sonnet-4-5-20250929", Priority: 2},
	},
	"claude-sonnet-4-5-thinking": {
		{Provider: "claude", ModelID: "claude-sonnet-4-5-thinking", Priority: 1},
		{Provider: "antigravity", ModelID: "gemini-claude-sonnet-4-5-thinking", Priority: 2},
	},

	// Claude Opus 4.5 family
	"claude-opus-4-5": {
		{Provider: "claude", ModelID: "claude-opus-4-5-20251101", Priority: 1},
		{Provider: "kiro", ModelID: "claude-opus-4-5-20251101", Priority: 2},
	},
	"claude-opus-4-5-thinking": {
		{Provider: "antigravity", ModelID: "gemini-claude-opus-4-5-thinking", Priority: 1},
		{Provider: "claude", ModelID: "claude-opus-4-5-thinking", Priority: 2},
	},

	// Claude Sonnet 4 family
	"claude-sonnet-4": {
		{Provider: "kiro", ModelID: "claude-sonnet-4-20250514", Priority: 1},
		{Provider: "claude", ModelID: "claude-sonnet-4-20250514", Priority: 2},
	},

	// Claude 3.7 Sonnet family
	"claude-3-7-sonnet": {
		{Provider: "kiro", ModelID: "claude-3-7-sonnet-20250219", Priority: 1},
		{Provider: "claude", ModelID: "claude-3-7-sonnet-20250219", Priority: 2},
	},

	// Gemini 2.5 Pro family - all same priority (load balanced)
	"gemini-2.5-pro": {
		{Provider: "gemini-cli", ModelID: "gemini-2.5-pro", Priority: 1},
		{Provider: "antigravity", ModelID: "gemini-2.5-pro", Priority: 1},
		{Provider: "aistudio", ModelID: "gemini-2.5-pro", Priority: 2},
		{Provider: "gemini", ModelID: "gemini-2.5-pro", Priority: 3},
	},

	// Gemini 2.5 Flash family
	"gemini-2.5-flash": {
		{Provider: "gemini-cli", ModelID: "gemini-2.5-flash", Priority: 1},
		{Provider: "antigravity", ModelID: "gemini-2.5-flash", Priority: 1},
		{Provider: "aistudio", ModelID: "gemini-2.5-flash", Priority: 2},
		{Provider: "gemini", ModelID: "gemini-2.5-flash", Priority: 3},
	},

	// Gemini 2.5 Flash Lite family
	"gemini-2.5-flash-lite": {
		{Provider: "gemini-cli", ModelID: "gemini-2.5-flash-lite", Priority: 1},
		{Provider: "antigravity", ModelID: "gemini-2.5-flash-lite", Priority: 1},
		{Provider: "aistudio", ModelID: "gemini-2.5-flash-lite", Priority: 2},
		{Provider: "gemini", ModelID: "gemini-2.5-flash-lite", Priority: 3},
	},

	// Gemini 3 Pro Preview family
	"gemini-3-pro-preview": {
		{Provider: "gemini-cli", ModelID: "gemini-3-pro-preview", Priority: 1},
		{Provider: "antigravity", ModelID: "gemini-3-pro-preview", Priority: 1},
		{Provider: "aistudio", ModelID: "gemini-3-pro-preview", Priority: 2},
		{Provider: "gemini", ModelID: "gemini-3-pro-preview", Priority: 3},
	},

	// GPT-5.1 Codex Max family
	"gpt-5.1-codex-max": {
		{Provider: "github-copilot", ModelID: "gpt-5.1-codex-max", Priority: 1},
		{Provider: "openai", ModelID: "gpt-5.1-codex-max", Priority: 2},
	},
}

// ResolveModelFamily attempts to resolve a canonical model name to a provider-specific model.
// It groups members by priority, then selects randomly among available providers at the highest priority level.
//
// Parameters:
//   - canonicalID: The canonical model name (e.g., "claude-sonnet-4-5")
//   - availableProviders: List of currently available provider types
//
// Returns:
//   - provider: The matched provider type
//   - modelID: The provider-specific model ID to use
//   - found: Whether a family match was found
func ResolveModelFamily(canonicalID string, availableProviders []string) (provider string, modelID string, found bool) {
	family, ok := ModelFamilies[canonicalID]
	if !ok {
		return "", canonicalID, false
	}

	// Create a set for O(1) lookup
	availableSet := make(map[string]bool, len(availableProviders))
	for _, p := range availableProviders {
		availableSet[p] = true
	}

	// Group available members by priority
	priorityGroups := make(map[int][]FamilyMember)
	for _, member := range family {
		if availableSet[member.Provider] {
			priorityGroups[member.Priority] = append(priorityGroups[member.Priority], member)
		}
	}

	if len(priorityGroups) == 0 {
		return "", canonicalID, false
	}

	// Get sorted priority levels (ascending = lowest number first = highest priority)
	priorities := make([]int, 0, len(priorityGroups))
	for p := range priorityGroups {
		priorities = append(priorities, p)
	}
	sort.Ints(priorities)

	// Select from highest priority group (first in sorted list)
	highestPriorityMembers := priorityGroups[priorities[0]]

	// If multiple members at same priority, select randomly
	var selected FamilyMember
	if len(highestPriorityMembers) == 1 {
		selected = highestPriorityMembers[0]
	} else {
		selected = highestPriorityMembers[rand.Intn(len(highestPriorityMembers))]
	}

	return selected.Provider, selected.ModelID, true
}

// GetCanonicalModelID returns the canonical ID for a provider-specific model ID.
// This is useful for reverse lookup (e.g., finding the family from a specific model).
//
// Returns empty string if no family contains this model ID.
func GetCanonicalModelID(providerModelID string) string {
	for canonical, members := range ModelFamilies {
		for _, member := range members {
			if member.ModelID == providerModelID {
				return canonical
			}
		}
	}
	return ""
}

// IsCanonicalID checks if the given ID is a canonical family name.
func IsCanonicalID(modelID string) bool {
	_, ok := ModelFamilies[modelID]
	return ok
}

// GetFamilyMembers returns all members of a model family, sorted by priority.
// Returns nil if the family doesn't exist.
func GetFamilyMembers(canonicalID string) []FamilyMember {
	family, ok := ModelFamilies[canonicalID]
	if !ok {
		return nil
	}

	// Return a copy sorted by priority
	sorted := make([]FamilyMember, len(family))
	copy(sorted, family)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Priority < sorted[j].Priority
	})
	return sorted
}

// ResolveAllProviders returns all available providers for a canonical model,
// sorted by priority with load balancing within same priority level.
// Returns the list of providers and whether any were found.
func ResolveAllProviders(canonicalID string, availableProviders []string) (providers []string, found bool) {
	family, ok := ModelFamilies[canonicalID]
	if !ok {
		return nil, false
	}

	// Create a set for O(1) lookup
	availableSet := make(map[string]bool, len(availableProviders))
	for _, p := range availableProviders {
		availableSet[p] = true
	}

	// Group available members by priority
	priorityGroups := make(map[int][]FamilyMember)
	for _, member := range family {
		if availableSet[member.Provider] {
			priorityGroups[member.Priority] = append(priorityGroups[member.Priority], member)
		}
	}

	if len(priorityGroups) == 0 {
		return nil, false
	}

	// Get sorted priority levels
	priorities := make([]int, 0, len(priorityGroups))
	for p := range priorityGroups {
		priorities = append(priorities, p)
	}
	sort.Ints(priorities)

	// Build result: for each priority level, shuffle members for load balancing
	result := make([]string, 0)
	for _, priority := range priorities {
		members := priorityGroups[priority]
		// Shuffle within same priority for load balancing
		if len(members) > 1 {
			rand.Shuffle(len(members), func(i, j int) {
				members[i], members[j] = members[j], members[i]
			})
		}
		for _, m := range members {
			result = append(result, m.Provider)
		}
	}

	return result, true
}

// TranslateModelForProvider translates a canonical model ID to the provider-specific ID.
// If the model is not a canonical ID or provider not in family, returns the original model.
func TranslateModelForProvider(canonicalID, provider string) string {
	family, ok := ModelFamilies[canonicalID]
	if !ok {
		return canonicalID
	}

	for _, member := range family {
		if member.Provider == provider {
			return member.ModelID
		}
	}

	return canonicalID
}
