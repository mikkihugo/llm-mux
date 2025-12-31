package preprocess

import (
	"strings"

	"github.com/nghyane/llm-mux/internal/registry"
	"github.com/nghyane/llm-mux/internal/translator/ir"
)

func applyThinkingNormalization(req *ir.UnifiedChatRequest, info *registry.ModelInfo) {
	promoteToThinkingModel(req, info)
	normalizeThinkingBudget(req, info)
}

func promoteToThinkingModel(req *ir.UnifiedChatRequest, info *registry.ModelInfo) bool {
	if req.Thinking == nil {
		return false
	}

	if !ir.IsClaudeModel(req.Model) {
		return false
	}

	if strings.HasSuffix(req.Model, "-thinking") {
		if req.Thinking.ThinkingBudget == nil {
			budget := int32(1024)
			req.Thinking.ThinkingBudget = &budget
		}
		req.Thinking.IncludeThoughts = true
		return false
	}

	thinkingModel := req.Model + "-thinking"
	if registry.GetGlobalRegistry().GetModelInfo(thinkingModel) != nil {
		req.Model = thinkingModel
		return true
	}

	return false
}

func normalizeThinkingBudget(req *ir.UnifiedChatRequest, info *registry.ModelInfo) {
	if req.Thinking == nil || req.Thinking.ThinkingBudget == nil {
		return
	}

	if info == nil || info.Thinking == nil {
		return
	}

	budget := int(*req.Thinking.ThinkingBudget)

	if budget == -1 && !info.Thinking.DynamicAllowed {
		budget = (info.Thinking.Min + info.Thinking.Max) / 2
	}

	if budget == 0 && !info.Thinking.ZeroAllowed {
		budget = info.Thinking.Min
	}

	if budget > 0 {
		if budget < info.Thinking.Min {
			budget = info.Thinking.Min
		}
		if budget > info.Thinking.Max {
			budget = info.Thinking.Max
		}
	}

	b := int32(budget)
	req.Thinking.ThinkingBudget = &b
}
