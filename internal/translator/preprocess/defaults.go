package preprocess

import (
	"github.com/nghyane/llm-mux/internal/registry"
	"github.com/nghyane/llm-mux/internal/translator/ir"
)

func applyProviderDefaults(req *ir.UnifiedChatRequest, info *registry.ModelInfo) {
	applyClaudeDefaults(req)
}

func applyClaudeDefaults(req *ir.UnifiedChatRequest) {
	if !ir.IsClaudeModel(req.Model) {
		return
	}

	if req.MaxTokens == nil || *req.MaxTokens == 0 {
		defaultMax := ir.ClaudeDefaultMaxTokens
		req.MaxTokens = &defaultMax
	}

	ir.CleanToolsForAntigravityClaude(req)
}
