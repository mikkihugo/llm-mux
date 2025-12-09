package ir

import "encoding/json"

// toClaudeToolID converts tool call ID to Claude format (toolu_...).
// Optimized: avoids allocation if already in correct format.
func toClaudeToolID(id string) string {
	if len(id) > 6 && id[0] == 't' && id[1] == 'o' && id[2] == 'o' && id[3] == 'l' && id[4] == 'u' && id[5] == '_' {
		return id // Already Claude format
	}
	if len(id) > 5 && id[0] == 'c' && id[1] == 'a' && id[2] == 'l' && id[3] == 'l' && id[4] == '_' {
		return "toolu_" + id[5:] // Replace call_ with toolu_
	}
	return "toolu_" + id
}

// ResponseBuilder helps construct provider-specific responses from IR messages
type ResponseBuilder struct {
	messages []Message
	usage    *Usage
	model    string
}

// NewResponseBuilder creates a new response builder
func NewResponseBuilder(messages []Message, usage *Usage, model string) *ResponseBuilder {
	return &ResponseBuilder{messages: messages, usage: usage, model: model}
}

// GetLastMessage returns the last message or nil if no messages exist
func (b *ResponseBuilder) GetLastMessage() *Message {
	if len(b.messages) == 0 {
		return nil
	}
	return &b.messages[len(b.messages)-1]
}

// HasContent returns true if the last message has any content or tool calls
func (b *ResponseBuilder) HasContent() bool {
	msg := b.GetLastMessage()
	return msg != nil && (len(msg.Content) > 0 || len(msg.ToolCalls) > 0)
}

// GetTextContent returns combined text content from the last message
func (b *ResponseBuilder) GetTextContent() string {
	if msg := b.GetLastMessage(); msg != nil {
		return CombineTextParts(*msg)
	}
	return ""
}

// GetReasoningContent returns combined reasoning content from the last message
func (b *ResponseBuilder) GetReasoningContent() string {
	if msg := b.GetLastMessage(); msg != nil {
		return CombineReasoningParts(*msg)
	}
	return ""
}

// GetToolCalls returns tool calls from the last message
func (b *ResponseBuilder) GetToolCalls() []ToolCall {
	if msg := b.GetLastMessage(); msg != nil {
		return msg.ToolCalls
	}
	return nil
}

// HasToolCalls returns true if the last message has any tool calls
func (b *ResponseBuilder) HasToolCalls() bool {
	return len(b.GetToolCalls()) > 0
}

// DetermineFinishReason determines the finish reason based on message content
func (b *ResponseBuilder) DetermineFinishReason() string {
	if len(b.GetToolCalls()) > 0 {
		return "tool_calls"
	}
	return "stop"
}

// BuildOpenAIToolCalls builds OpenAI-format tool calls array
func (b *ResponseBuilder) BuildOpenAIToolCalls() []any {
	toolCalls := b.GetToolCalls()
	if len(toolCalls) == 0 {
		return nil
	}
	result := make([]any, len(toolCalls))
	for i, tc := range toolCalls {
		result[i] = map[string]any{
			"id":   tc.ID,
			"type": "function",
			"function": map[string]any{
				"name":      tc.Name,
				"arguments": tc.Args,
			},
		}
	}
	return result
}

// BuildClaudeContentParts builds Claude-format content parts array
func (b *ResponseBuilder) BuildClaudeContentParts() []any {
	msg := b.GetLastMessage()
	if msg == nil {
		return []any{}
	}

	var parts []any

	// Add reasoning/thinking content first
	for _, part := range msg.Content {
		if part.Type == ContentTypeReasoning && part.Reasoning != "" {
			parts = append(parts, map[string]any{"type": "thinking", "thinking": part.Reasoning})
		}
	}

	// Add text content
	for _, part := range msg.Content {
		if part.Type == ContentTypeText && part.Text != "" {
			parts = append(parts, map[string]any{"type": "text", "text": part.Text})
		}
	}

	// Add tool calls
	for _, tc := range msg.ToolCalls {
		toolUse := map[string]any{
			"type":  "tool_use",
			"id":    toClaudeToolID(tc.ID),
			"name":  tc.Name,
			"input": map[string]any{},
		}
		if tc.Args != "" && tc.Args != "{}" {
			var argsObj any
			if json.Unmarshal([]byte(tc.Args), &argsObj) == nil {
				toolUse["input"] = argsObj
			}
		}
		parts = append(parts, toolUse)
	}

	return parts
}

// BuildGeminiContentParts builds Gemini-format content parts array
func (b *ResponseBuilder) BuildGeminiContentParts() []any {
	msg := b.GetLastMessage()
	if msg == nil {
		return []any{}
	}

	var parts []any

	// Process all content parts in order to preserve original sequence
	for _, part := range msg.Content {
		switch part.Type {
		case ContentTypeReasoning:
			if part.Reasoning != "" {
				p := map[string]any{"text": part.Reasoning, "thought": true}
				if part.ThoughtSignature != "" {
					p["thoughtSignature"] = part.ThoughtSignature
				}
				parts = append(parts, p)
			}
		case ContentTypeText:
			if part.Text != "" {
				p := map[string]any{"text": part.Text}
				if part.ThoughtSignature != "" {
					p["thoughtSignature"] = part.ThoughtSignature
				}
				parts = append(parts, p)
			}
		case ContentTypeImage:
			if part.Image != nil && part.Image.Data != "" {
				parts = append(parts, map[string]any{
					"inlineData": map[string]any{
						"mimeType": part.Image.MimeType,
						"data":     part.Image.Data,
					},
				})
			}
		case ContentTypeExecutableCode:
			if part.CodeExecution != nil && part.CodeExecution.Code != "" {
				parts = append(parts, map[string]any{
					"executableCode": map[string]any{
						"language": part.CodeExecution.Language,
						"code":     part.CodeExecution.Code,
					},
				})
			}
		case ContentTypeCodeResult:
			if part.CodeExecution != nil {
				parts = append(parts, map[string]any{
					"codeExecutionResult": map[string]any{
						"outcome": part.CodeExecution.Outcome,
						"output":  part.CodeExecution.Output,
					},
				})
			}
		}
	}

	// Add tool calls as functionCall parts
	for _, tc := range msg.ToolCalls {
		parts = append(parts, map[string]any{
			"functionCall": map[string]any{
				"name": tc.Name,
				"args": ParseToolCallArgs(tc.Args),
			},
		})
	}

	return parts
}

// BuildUsageMap builds a usage statistics map
func (b *ResponseBuilder) BuildUsageMap() map[string]any {
	if b.usage == nil {
		return nil
	}
	return map[string]any{
		"prompt_tokens":     b.usage.PromptTokens,
		"completion_tokens": b.usage.CompletionTokens,
		"total_tokens":      b.usage.TotalTokens,
	}
}
