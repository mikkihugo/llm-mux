package from_ir

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/nghyane/llm-mux/internal/translator/ir"
	"github.com/tidwall/gjson"
)

// Claude user tracking (matches old translator behavior)
var (
	claudeUser    = ""
	claudeAccount = ""
	claudeSession = ""
)

// toClaudeToolID converts an OpenAI-style tool call ID to Claude format.
// OpenAI format: call_xxx... -> Claude format: toolu_xxx...
// Optimized for performance: avoids allocations when already Claude format.
func toClaudeToolID(id string) string {
	if len(id) > 6 && id[0] == 't' && id[1] == 'o' && id[2] == 'o' && id[3] == 'l' && id[4] == 'u' && id[5] == '_' {
		return id // Already Claude format - fast path, no allocation
	}
	if len(id) > 5 && id[0] == 'c' && id[1] == 'a' && id[2] == 'l' && id[3] == 'l' && id[4] == '_' {
		return "toolu_" + id[5:] // Replace prefix - single allocation
	}
	return "toolu_" + id // Fallback
}

// ClaudeProvider handles conversion to Claude Messages API format.
type ClaudeProvider struct{}

// ClaudeStreamState tracks state for streaming response conversion.
type ClaudeStreamState struct {
	MessageID        string
	Model            string
	MessageStartSent bool
	TextBlockStarted bool
	TextBlockStopped bool
	TextBlockIndex   int
	ToolBlockCount   int
	HasToolCalls     bool
	FinishSent       bool
}

// NewClaudeStreamState creates a new streaming state tracker.
func NewClaudeStreamState() *ClaudeStreamState {
	return &ClaudeStreamState{TextBlockIndex: 0, ToolBlockCount: 0}
}

// ConvertRequest transforms unified request into Claude Messages API JSON.
func (p *ClaudeProvider) ConvertRequest(req *ir.UnifiedChatRequest) ([]byte, error) {
	if claudeAccount == "" {
		u, _ := uuid.NewRandom()
		claudeAccount = u.String()
	}
	if claudeSession == "" {
		u, _ := uuid.NewRandom()
		claudeSession = u.String()
	}
	if claudeUser == "" {
		sum := sha256.Sum256([]byte(claudeAccount + claudeSession))
		claudeUser = hex.EncodeToString(sum[:])
	}
	userID := fmt.Sprintf("user_%s_account_%s_session_%s", claudeUser, claudeAccount, claudeSession)

	root := map[string]any{
		"model":      req.Model,
		"max_tokens": ir.ClaudeDefaultMaxTokens,
		"metadata":   map[string]any{"user_id": userID},
		"messages":   []any{},
	}

	if req.MaxTokens != nil {
		root["max_tokens"] = *req.MaxTokens
	}
	if req.Temperature != nil {
		root["temperature"] = *req.Temperature
	}
	if req.TopP != nil {
		root["top_p"] = *req.TopP
	}
	if req.TopK != nil {
		root["top_k"] = *req.TopK
	}
	if len(req.StopSequences) > 0 {
		root["stop_sequences"] = req.StopSequences
	}

	if req.Thinking != nil {
		thinking := map[string]any{}
		if req.Thinking.IncludeThoughts && req.Thinking.Budget != 0 {
			thinking["type"] = "enabled"
			if req.Thinking.Budget > 0 {
				thinking["budget_tokens"] = req.Thinking.Budget
			}
		} else if req.Thinking.Budget == 0 {
			thinking["type"] = "disabled"
		}
		if len(thinking) > 0 {
			root["thinking"] = thinking
		}
	}

	var messages []any
	for _, msg := range req.Messages {
		switch msg.Role {
		case ir.RoleSystem:
			if text := ir.CombineTextParts(msg); text != "" {
				root["system"] = text
			}
		case ir.RoleUser:
			if parts := buildClaudeContentParts(msg, false); len(parts) > 0 {
				messages = append(messages, map[string]any{"role": ir.ClaudeRoleUser, "content": parts})
			}
		case ir.RoleAssistant:
			if parts := buildClaudeContentParts(msg, true); len(parts) > 0 {
				messages = append(messages, map[string]any{"role": ir.ClaudeRoleAssistant, "content": parts})
			}
		case ir.RoleTool:
			for _, part := range msg.Content {
				if part.Type == ir.ContentTypeToolResult && part.ToolResult != nil {
					messages = append(messages, map[string]any{
						"role": ir.ClaudeRoleUser,
						"content": []any{map[string]any{
							"type": ir.ClaudeBlockToolResult, "tool_use_id": part.ToolResult.ToolCallID, "content": part.ToolResult.Result,
						}},
					})
				}
			}
		}
	}
	root["messages"] = messages

	if len(req.Tools) > 0 {
		var tools []any
		for _, t := range req.Tools {
			tool := map[string]any{"name": t.Name, "description": t.Description}
			if len(t.Parameters) > 0 {
				tool["input_schema"] = ir.CleanJsonSchemaForClaude(copyMap(t.Parameters))
			} else {
				tool["input_schema"] = map[string]any{
					"type": "object", "properties": map[string]any{}, "additionalProperties": false, "$schema": "http://json-schema.org/draft-07/schema#",
				}
			}
			tools = append(tools, tool)
		}
		root["tools"] = tools
	}

	if len(req.Metadata) > 0 {
		meta := root["metadata"].(map[string]any)
		for k, v := range req.Metadata {
			meta[k] = v
		}
	}

	return json.Marshal(root)
}

// ParseResponse parses non-streaming Claude response into unified format.
func (p *ClaudeProvider) ParseResponse(responseJSON []byte) ([]ir.Message, *ir.Usage, error) {
	if !gjson.ValidBytes(responseJSON) {
		return nil, nil, &json.UnmarshalTypeError{Value: "invalid json"}
	}
	parsed := gjson.ParseBytes(responseJSON)
	usage := ir.ParseClaudeUsage(parsed.Get("usage"))

	content := parsed.Get("content")
	if !content.Exists() || !content.IsArray() {
		return nil, usage, nil
	}

	msg := ir.Message{Role: ir.RoleAssistant}
	for _, block := range content.Array() {
		ir.ParseClaudeContentBlock(block, &msg)
	}

	if len(msg.Content) == 0 && len(msg.ToolCalls) == 0 {
		return nil, usage, nil
	}
	return []ir.Message{msg}, usage, nil
}

// ParseStreamChunk parses streaming Claude SSE chunk into events.
func (p *ClaudeProvider) ParseStreamChunk(chunkJSON []byte) ([]ir.UnifiedEvent, error) {
	return p.ParseStreamChunkWithState(chunkJSON, nil)
}

// ParseStreamChunkWithState parses streaming Claude SSE chunk with state tracking.
func (p *ClaudeProvider) ParseStreamChunkWithState(chunkJSON []byte, state *ir.ClaudeStreamParserState) ([]ir.UnifiedEvent, error) {
	data := ir.ExtractSSEData(chunkJSON)
	if len(data) == 0 || !gjson.ValidBytes(data) {
		return nil, nil
	}

	parsed := gjson.ParseBytes(data)
	switch parsed.Get("type").String() {
	case ir.ClaudeSSEContentBlockStart:
		return ir.ParseClaudeContentBlockStart(parsed, state), nil
	case ir.ClaudeSSEContentBlockDelta:
		if state != nil {
			return ir.ParseClaudeStreamDeltaWithState(parsed, state), nil
		}
		return ir.ParseClaudeStreamDelta(parsed), nil
	case ir.ClaudeSSEContentBlockStop:
		return ir.ParseClaudeContentBlockStop(parsed, state), nil
	case ir.ClaudeSSEMessageDelta:
		return ir.ParseClaudeMessageDelta(parsed), nil
	case ir.ClaudeSSEMessageStop:
		return []ir.UnifiedEvent{{Type: ir.EventTypeFinish, FinishReason: ir.FinishReasonStop}}, nil
	case ir.ClaudeSSEError:
		msg := parsed.Get("error.message").String()
		if msg == "" {
			msg = "Unknown Claude API error"
		}
		return []ir.UnifiedEvent{{Type: ir.EventTypeError, Error: fmt.Errorf("%s", msg)}}, nil
	}
	return nil, nil
}

// ToClaudeSSE converts event to Claude SSE format.
func ToClaudeSSE(event ir.UnifiedEvent, model, messageID string, state *ClaudeStreamState) ([]byte, error) {
	var result strings.Builder

	if state != nil && !state.MessageStartSent {
		state.MessageStartSent = true
		state.Model, state.MessageID = model, messageID
		result.WriteString(formatSSE(ir.ClaudeSSEMessageStart, map[string]any{
			"type": ir.ClaudeSSEMessageStart,
			"message": map[string]any{
				"id": messageID, "type": "message", "role": ir.ClaudeRoleAssistant,
				"content": []any{}, "model": model, "stop_reason": nil, "stop_sequence": nil,
				"usage": map[string]any{"input_tokens": 0, "output_tokens": 0},
			},
		}))
	}

	switch event.Type {
	case ir.EventTypeToken:
		result.WriteString(emitTextDelta(event.Content, state))
	case ir.EventTypeReasoning:
		result.WriteString(emitThinkingDelta(event.Reasoning, state))
	case ir.EventTypeToolCall:
		if event.ToolCall != nil {
			result.WriteString(emitToolCall(event.ToolCall, state))
		}
	case ir.EventTypeFinish:
		if state != nil && state.FinishSent {
			return nil, nil
		}
		if state != nil {
			state.FinishSent = true
		}
		result.WriteString(emitFinish(event.Usage, state))
	case ir.EventTypeError:
		result.WriteString(formatSSE(ir.ClaudeSSEError, map[string]any{
			"type": ir.ClaudeSSEError, "error": map[string]any{"type": "api_error", "message": errMsg(event.Error)},
		}))
	}

	if result.Len() == 0 {
		return nil, nil
	}
	return []byte(result.String()), nil
}

// ToClaudeResponse converts messages to complete Claude response.
func ToClaudeResponse(messages []ir.Message, usage *ir.Usage, model, messageID string) ([]byte, error) {
	builder := ir.NewResponseBuilder(messages, usage, model)
	response := map[string]any{
		"id": messageID, "type": "message", "role": ir.ClaudeRoleAssistant,
		"content": builder.BuildClaudeContentParts(), "model": model, "stop_reason": ir.ClaudeStopEndTurn,
	}
	if builder.HasToolCalls() {
		response["stop_reason"] = ir.ClaudeStopToolUse
	}
	if usage != nil {
		response["usage"] = map[string]any{"input_tokens": usage.PromptTokens, "output_tokens": usage.CompletionTokens}
	}
	return json.Marshal(response)
}

func buildClaudeContentParts(msg ir.Message, includeToolCalls bool) []any {
	var parts []any
	for _, p := range msg.Content {
		switch p.Type {
		case ir.ContentTypeReasoning:
			if p.Reasoning != "" {
				parts = append(parts, map[string]any{"type": ir.ClaudeBlockThinking, "thinking": p.Reasoning})
			}
		case ir.ContentTypeText:
			if p.Text != "" {
				parts = append(parts, map[string]any{"type": ir.ClaudeBlockText, "text": p.Text})
			}
		case ir.ContentTypeImage:
			if p.Image != nil {
				parts = append(parts, map[string]any{
					"type":   ir.ClaudeBlockImage,
					"source": map[string]any{"type": "base64", "media_type": p.Image.MimeType, "data": p.Image.Data},
				})
			}
		case ir.ContentTypeToolResult:
			if p.ToolResult != nil {
				parts = append(parts, map[string]any{
					"type": ir.ClaudeBlockToolResult, "tool_use_id": p.ToolResult.ToolCallID, "content": p.ToolResult.Result,
				})
			}
		}
	}
	if includeToolCalls {
		for _, tc := range msg.ToolCalls {
			toolUse := map[string]any{"type": ir.ClaudeBlockToolUse, "id": toClaudeToolID(tc.ID), "name": tc.Name}
			toolUse["input"] = ir.ParseToolCallArgs(tc.Args)
			parts = append(parts, toolUse)
		}
	}
	return parts
}

func formatSSE(eventType string, data any) string {
	jsonData, _ := json.Marshal(data)
	return fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, string(jsonData))
}

func emitTextDelta(text string, state *ClaudeStreamState) string {
	var result strings.Builder
	idx := 0
	if state != nil {
		idx = state.TextBlockIndex
		if !state.TextBlockStarted {
			state.TextBlockStarted = true
			result.WriteString(formatSSE(ir.ClaudeSSEContentBlockStart, map[string]any{
				"type": ir.ClaudeSSEContentBlockStart, "index": idx,
				"content_block": map[string]any{"type": ir.ClaudeBlockText, "text": ""},
			}))
		}
	}
	result.WriteString(formatSSE(ir.ClaudeSSEContentBlockDelta, map[string]any{
		"type": ir.ClaudeSSEContentBlockDelta, "index": idx,
		"delta": map[string]any{"type": "text_delta", "text": text},
	}))
	return result.String()
}

func emitThinkingDelta(thinking string, state *ClaudeStreamState) string {
	var result strings.Builder
	idx := 0
	if state != nil {
		idx = state.TextBlockIndex
		if !state.TextBlockStarted {
			state.TextBlockStarted = true
			result.WriteString(formatSSE(ir.ClaudeSSEContentBlockStart, map[string]any{
				"type": ir.ClaudeSSEContentBlockStart, "index": idx,
				"content_block": map[string]any{"type": ir.ClaudeBlockThinking, "thinking": ""},
			}))
		}
	}
	result.WriteString(formatSSE(ir.ClaudeSSEContentBlockDelta, map[string]any{
		"type": ir.ClaudeSSEContentBlockDelta, "index": idx,
		"delta": map[string]any{"type": "thinking_delta", "thinking": thinking},
	}))
	return result.String()
}

func emitToolCall(tc *ir.ToolCall, state *ClaudeStreamState) string {
	var result strings.Builder
	if state != nil && state.TextBlockStarted && !state.TextBlockStopped {
		state.TextBlockStopped = true
		result.WriteString(formatSSE(ir.ClaudeSSEContentBlockStop, map[string]any{"type": ir.ClaudeSSEContentBlockStop, "index": state.TextBlockIndex}))
	}

	idx := 1
	if state != nil {
		state.HasToolCalls = true
		idx = 1 + state.ToolBlockCount
		state.ToolBlockCount++
	}

	result.WriteString(formatSSE(ir.ClaudeSSEContentBlockStart, map[string]any{
		"type": ir.ClaudeSSEContentBlockStart, "index": idx,
		"content_block": map[string]any{"type": ir.ClaudeBlockToolUse, "id": toClaudeToolID(tc.ID), "name": tc.Name, "input": map[string]any{}},
	}))

	args := tc.Args
	if args == "" {
		args = "{}"
	}
	result.WriteString(formatSSE(ir.ClaudeSSEContentBlockDelta, map[string]any{
		"type": ir.ClaudeSSEContentBlockDelta, "index": idx,
		"delta": map[string]any{"type": "input_json_delta", "partial_json": args},
	}))
	result.WriteString(formatSSE(ir.ClaudeSSEContentBlockStop, map[string]any{"type": ir.ClaudeSSEContentBlockStop, "index": idx}))
	return result.String()
}

func emitFinish(usage *ir.Usage, state *ClaudeStreamState) string {
	var result strings.Builder
	stopReason := ir.ClaudeStopEndTurn
	if state != nil && state.HasToolCalls {
		stopReason = ir.ClaudeStopToolUse
	}
	delta := map[string]any{"type": ir.ClaudeSSEMessageDelta, "delta": map[string]any{"stop_reason": stopReason}}
	if usage != nil {
		delta["usage"] = map[string]any{"input_tokens": usage.PromptTokens, "output_tokens": usage.CompletionTokens}
	}
	result.WriteString(formatSSE(ir.ClaudeSSEMessageDelta, delta))
	result.WriteString(formatSSE(ir.ClaudeSSEMessageStop, map[string]any{"type": ir.ClaudeSSEMessageStop}))
	return result.String()
}

func errMsg(err error) string {
	if err != nil {
		return err.Error()
	}
	return "Unknown error"
}

func copyMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	result := make(map[string]any, len(m))
	for k, v := range m {
		if nested, ok := v.(map[string]any); ok {
			result[k] = copyMap(nested)
		} else if arr, ok := v.([]any); ok {
			newArr := make([]any, len(arr))
			for i, item := range arr {
				if nestedMap, ok := item.(map[string]any); ok {
					newArr[i] = copyMap(nestedMap)
				} else {
					newArr[i] = item
				}
			}
			result[k] = newArr
		} else {
			result[k] = v
		}
	}
	return result
}
