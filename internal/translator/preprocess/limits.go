package preprocess

import (
	"github.com/nghyane/llm-mux/internal/registry"
	"github.com/nghyane/llm-mux/internal/translator/ir"
)

func applyLimits(req *ir.UnifiedChatRequest, info *registry.ModelInfo) {
	clampMaxTokens(req, info)
	clampCandidateCount(req, info)
}

func clampMaxTokens(req *ir.UnifiedChatRequest, info *registry.ModelInfo) {
	if req.MaxTokens == nil {
		return
	}

	if info == nil {
		return
	}

	limit := info.OutputTokenLimit
	if limit == 0 {
		limit = info.MaxCompletionTokens
	}

	if limit > 0 && *req.MaxTokens > limit {
		*req.MaxTokens = limit
	}
}

func clampCandidateCount(req *ir.UnifiedChatRequest, info *registry.ModelInfo) {
	if req.CandidateCount == nil {
		return
	}

	if *req.CandidateCount < 1 {
		*req.CandidateCount = 1
	}

	maxCandidates := 8
	if *req.CandidateCount > maxCandidates {
		*req.CandidateCount = maxCandidates
	}
}
