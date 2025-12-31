package translator

import (
	"testing"

	"github.com/nghyane/llm-mux/internal/provider"
	"github.com/nghyane/llm-mux/internal/translator/ir"
)

type mockToIRParser struct {
	format string
}

func (m mockToIRParser) Parse(payload []byte) (*ir.UnifiedChatRequest, error) {
	return &ir.UnifiedChatRequest{Model: "mock-model"}, nil
}

func (m mockToIRParser) ParseResponse(payload []byte) ([]ir.Message, *ir.Usage, error) {
	return nil, nil, nil
}

func (m mockToIRParser) ParseChunk(payload []byte) ([]ir.UnifiedEvent, error) {
	return nil, nil
}

func (m mockToIRParser) Format() string { return m.format }

type mockFromIRConverter struct {
	providerName string
}

func (m mockFromIRConverter) ConvertRequest(req *ir.UnifiedChatRequest) ([]byte, error) {
	return []byte(`{"mock":true}`), nil
}

func (m mockFromIRConverter) ToResponse(messages []ir.Message, usage *ir.Usage, model string) ([]byte, error) {
	return []byte(`{"response":true}`), nil
}

func (m mockFromIRConverter) ToChunk(event ir.UnifiedEvent, model string) ([]byte, error) {
	return []byte(`{"chunk":true}`), nil
}

func (m mockFromIRConverter) Provider() string { return m.providerName }

func newTestRegistry() *Registry {
	return &Registry{
		toIR:       make(map[string]ToIRParser),
		fromIR:     make(map[string]FromIRConverter),
		formatToIR: make(map[provider.Format]ToIRParser),
	}
}

func TestRegistryToIRRegistrationAndLookup(t *testing.T) {
	registry := newTestRegistry()

	parser := mockToIRParser{format: "test-format"}
	registry.RegisterToIR("test-format", parser)

	got, ok := registry.GetToIR("test-format")
	if !ok {
		t.Fatal("expected to find registered parser")
	}
	if got.Format() != "test-format" {
		t.Errorf("expected format 'test-format', got %s", got.Format())
	}
}

func TestRegistryFromIRRegistrationAndLookup(t *testing.T) {
	registry := newTestRegistry()

	converter := mockFromIRConverter{providerName: "test-provider"}
	registry.RegisterFromIR("test-provider", converter)

	got, ok := registry.GetFromIR("test-provider")
	if !ok {
		t.Fatal("expected to find registered converter")
	}
	if got.Provider() != "test-provider" {
		t.Errorf("expected provider 'test-provider', got %s", got.Provider())
	}
}

func TestRegistryLookupNotFound(t *testing.T) {
	registry := newTestRegistry()

	_, ok := registry.GetToIR("nonexistent")
	if ok {
		t.Error("expected GetToIR to return false for nonexistent format")
	}

	_, ok = registry.GetFromIR("nonexistent")
	if ok {
		t.Error("expected GetFromIR to return false for nonexistent provider")
	}
}

func TestRegistryMustGetPanics(t *testing.T) {
	registry := newTestRegistry()

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected MustGetToIR to panic for nonexistent format")
		}
	}()
	registry.MustGetToIR("nonexistent")
}

func TestRegistryMustGetFromIRPanics(t *testing.T) {
	registry := newTestRegistry()

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected MustGetFromIR to panic for nonexistent provider")
		}
	}()
	registry.MustGetFromIR("nonexistent")
}

func TestRegistryListFormatsAndProviders(t *testing.T) {
	registry := newTestRegistry()

	registry.RegisterToIR("format-a", mockToIRParser{format: "format-a"})
	registry.RegisterToIR("format-b", mockToIRParser{format: "format-b"})
	registry.RegisterFromIR("provider-x", mockFromIRConverter{providerName: "provider-x"})
	registry.RegisterFromIR("provider-y", mockFromIRConverter{providerName: "provider-y"})

	formats := registry.ListToIRFormats()
	if len(formats) != 2 {
		t.Errorf("expected 2 formats, got %d", len(formats))
	}

	providers := registry.ListFromIRProviders()
	if len(providers) != 2 {
		t.Errorf("expected 2 providers, got %d", len(providers))
	}
}

func TestGlobalRegistrySingleton(t *testing.T) {
	r1 := GetRegistry()
	r2 := GetRegistry()

	if r1 != r2 {
		t.Error("expected GetRegistry to return same instance")
	}
}

func TestParseRequestWithUnregisteredFormat(t *testing.T) {
	_, err := ParseRequest("nonexistent-format-xyz", []byte(`{}`))
	if err == nil {
		t.Error("expected error for unregistered format")
	}
}
