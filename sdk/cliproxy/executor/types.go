package executor

import (
	"net/http"
	"net/url"

	sdktranslator "github.com/nghyane/llm-mux/sdk/translator"
)

// Request encapsulates the translated payload that will be sent to a provider executor.
type Request struct {
	Model    string
	Payload  []byte
	Format   sdktranslator.Format
	Metadata map[string]any
}

// Options controls execution behavior for both streaming and non-streaming calls.
type Options struct {
	Stream          bool
	Alt             string
	Headers         http.Header
	Query           url.Values
	OriginalRequest []byte
	SourceFormat    sdktranslator.Format
	Metadata        map[string]any
}

// Response wraps either a full provider response or metadata for streaming flows.
type Response struct {
	Payload  []byte
	Metadata map[string]any
}

// StreamChunk represents a single streaming payload unit emitted by provider executors.
type StreamChunk struct {
	Payload []byte
	Err     error
}

// StatusError represents an error that carries an HTTP-like status code.
// Provider executors should implement this when possible to enable
// better auth state updates on failures (e.g., 401/402/429).
type StatusError interface {
	error
	StatusCode() int
}
