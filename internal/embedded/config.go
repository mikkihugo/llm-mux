// Package embedded provides access to generated default configuration.
package embedded

import "github.com/nghyane/llm-mux/internal/config"

// DefaultConfigTemplate returns the default config YAML generated from NewDefaultConfig().
// This replaces the previously embedded config.example.yaml file.
func DefaultConfigTemplate() []byte {
	return config.GenerateDefaultConfigYAML()
}
