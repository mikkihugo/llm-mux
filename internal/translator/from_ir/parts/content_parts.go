// Package parts provides shared content part builders for IR to provider translation.
package parts

import (
	"strings"

	"github.com/nghyane/llm-mux/internal/translator/ir"
)

// BuildTextPart creates a text content part.
func BuildTextPart(text string) map[string]any {
	return map[string]any{"text": text}
}

// BuildThoughtPart creates a thought/reasoning content part.
func BuildThoughtPart(text string, signature []byte) map[string]any {
	part := map[string]any{"text": text, "thought": true}
	if ir.IsValidThoughtSignature(signature) {
		part["thoughtSignature"] = string(signature)
	}
	return part
}

// BuildImagePart creates an image content part from IR.
// Supports inline data (base64) and file references (files/, gs://).
func BuildImagePart(img *ir.ImagePart) map[string]any {
	if img == nil {
		return nil
	}
	if img.Data != "" {
		return map[string]any{
			"inlineData": map[string]any{
				"mimeType": img.MimeType,
				"data":     img.Data,
			},
		}
	}
	if u := img.URL; strings.HasPrefix(u, "files/") || strings.HasPrefix(u, "gs://") {
		return map[string]any{
			"fileData": map[string]any{
				"mimeType": img.MimeType,
				"fileUri":  u,
			},
		}
	}
	return nil
}

// BuildAudioPart creates an audio content part from IR.
func BuildAudioPart(audio *ir.AudioPart) map[string]any {
	if audio == nil {
		return nil
	}
	if audio.FileURI != "" {
		return map[string]any{
			"fileData": map[string]any{
				"mimeType": audio.MimeType,
				"fileUri":  audio.FileURI,
			},
		}
	}
	if audio.Data != "" {
		return map[string]any{
			"inlineData": map[string]any{
				"mimeType": audio.MimeType,
				"data":     audio.Data,
			},
		}
	}
	return nil
}

// BuildVideoPart creates a video content part from IR.
func BuildVideoPart(video *ir.VideoPart) map[string]any {
	if video == nil {
		return nil
	}
	if video.Data != "" {
		return map[string]any{
			"inlineData": map[string]any{
				"mimeType": video.MimeType,
				"data":     video.Data,
			},
		}
	}
	if video.FileURI != "" {
		return map[string]any{
			"fileData": map[string]any{
				"mimeType": video.MimeType,
				"fileUri":  video.FileURI,
			},
		}
	}
	return nil
}

// BuildFunctionCall creates a function call part for tool use.
func BuildFunctionCall(name, id string, args any, signature []byte, isG3 bool) map[string]any {
	part := map[string]any{
		"functionCall": map[string]any{
			"name": name,
			"args": args,
			"id":   id,
		},
	}
	if ir.IsValidThoughtSignature(signature) {
		part["thoughtSignature"] = string(signature)
	} else if isG3 {
		part["thoughtSignature"] = ir.DummyThoughtSignature
	}
	return part
}

// BuildFunctionResponse creates a function response part for tool results.
func BuildFunctionResponse(name, id string, response any) map[string]any {
	return map[string]any{
		"functionResponse": map[string]any{
			"name":     name,
			"id":       id,
			"response": response,
		},
	}
}

// BuildUserParts converts IR message content to provider parts format.
// This is the shared implementation for both Gemini and Vertex.
func BuildUserParts(content []ir.ContentPart) []any {
	parts := make([]any, 0, len(content))
	for i := range content {
		part := &content[i]
		switch part.Type {
		case ir.ContentTypeText:
			if part.Text != "" {
				parts = append(parts, BuildTextPart(part.Text))
			}
		case ir.ContentTypeImage:
			if p := BuildImagePart(part.Image); p != nil {
				parts = append(parts, p)
			}
		case ir.ContentTypeAudio:
			if p := BuildAudioPart(part.Audio); p != nil {
				parts = append(parts, p)
			}
		case ir.ContentTypeVideo:
			if p := BuildVideoPart(part.Video); p != nil {
				parts = append(parts, p)
			}
		}
	}
	return parts
}
