package from_ir

import (
	"encoding/json"
	"testing"

	"github.com/tidwall/gjson"
	"github.com/nghyane/llm-mux/internal/translator/to_ir"
)

func TestLogprobsTranslation(t *testing.T) {
	// OpenAI request with logprobs=true
	openaiReq := []byte(`{
		"model": "gemini-2.5-flash",
		"messages": [{"role": "user", "content": "Hi"}],
		"logprobs": true,
		"top_logprobs": 3,
		"max_tokens": 5
	}`)

	// Parse to IR
	ir, err := to_ir.ParseOpenAIRequest(openaiReq)
	if err != nil {
		t.Fatalf("Error parsing: %v", err)
	}

	if ir.Logprobs == nil || !*ir.Logprobs {
		t.Errorf("IR.Logprobs should be true, got: %v", ir.Logprobs)
	}
	if ir.TopLogprobs == nil || *ir.TopLogprobs != 3 {
		t.Errorf("IR.TopLogprobs should be 3, got: %v", ir.TopLogprobs)
	}

	// Convert to Gemini
	geminiReq, err := (&GeminiProvider{}).ConvertRequest(ir)
	if err != nil {
		t.Fatalf("Error converting: %v", err)
	}

	// Check if responseLogprobs is set
	var result map[string]any
	json.Unmarshal(geminiReq, &result)

	genConfig, ok := result["generationConfig"].(map[string]any)
	if !ok {
		t.Fatalf("generationConfig not found")
	}

	if responseLogprobs, ok := genConfig["responseLogprobs"]; !ok || responseLogprobs != true {
		t.Errorf("responseLogprobs should be true, got: %v (genConfig: %v)", responseLogprobs, genConfig)
	}

	if logprobs, ok := genConfig["logprobs"]; !ok || logprobs != float64(3) {
		t.Errorf("logprobs should be 3, got: %v", logprobs)
	}
}

func TestLogprobsTranslationCLI(t *testing.T) {
	// OpenAI request with logprobs=true
	openaiReq := []byte(`{
		"model": "gemini-2.5-flash",
		"messages": [{"role": "user", "content": "Hi"}],
		"logprobs": true,
		"top_logprobs": 3,
		"max_tokens": 5
	}`)

	// Parse to IR
	ir, err := to_ir.ParseOpenAIRequest(openaiReq)
	if err != nil {
		t.Fatalf("Error parsing: %v", err)
	}

	// Convert to Gemini CLI format
	cliReq, err := (&GeminiCLIProvider{}).ConvertRequest(ir)
	if err != nil {
		t.Fatalf("Error converting: %v", err)
	}

	// Check the CLI envelope format
	t.Logf("CLI Request: %s", string(cliReq))

	// Parse and check
	parsed := gjson.ParseBytes(cliReq)
	responseLogprobs := parsed.Get("request.generationConfig.responseLogprobs").Bool()
	logprobs := parsed.Get("request.generationConfig.logprobs").Int()

	if !responseLogprobs {
		t.Errorf("request.generationConfig.responseLogprobs should be true")
	}
	if logprobs != 3 {
		t.Errorf("request.generationConfig.logprobs should be 3, got: %d", logprobs)
	}
}
