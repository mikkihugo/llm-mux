# llm-mux

**Free LLM API gateway** that converts OAuth-authenticated CLI tools into OpenAI-compatible endpoints.

Use Claude, Gemini, GPT, and other models **without API keys** - authenticate once with your existing accounts.

## Why llm-mux?

| Traditional API Access | llm-mux |
|------------------------|---------|
| Requires API keys | Uses OAuth from CLI tools |
| Pay per token | Free (within CLI quotas) |
| One provider per key | All providers, one endpoint |
| Different APIs per provider | Unified OpenAI-compatible API |

## Quick Start

```bash
# Install
brew tap nghyane/tap && brew install llm-mux

# Authenticate with any provider
llm-mux --login              # Gemini CLI
llm-mux --antigravity-login  # Antigravity (Gemini + Claude + GPT-OSS)
llm-mux --copilot-login      # GitHub Copilot

# Start service
brew services start llm-mux

# Use
curl http://localhost:8318/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model": "gemini-2.5-flash", "messages": [{"role": "user", "content": "Hello!"}]}'
```

## Supported Providers

### Google

| Provider | Login | Models |
|----------|-------|--------|
| **Gemini CLI** | `--login` | gemini-2.5-pro, gemini-2.5-flash, gemini-2.5-flash-lite, gemini-3-pro-preview |
| **Antigravity** | `--antigravity-login` | Gemini models + Claude Sonnet/Opus 4.5 + GPT-OSS + Computer Use |
| **AI Studio** | `--login` | Gemini models + image generation models |
| **Vertex AI** | API Key | Gemini models |

### Anthropic

| Provider | Login | Models |
|----------|-------|--------|
| **Claude** | `--claude-login` | claude-sonnet-4-5, claude-opus-4-5 |
| **Kiro** | `--kiro-login` | Claude models via Amazon Q |

### OpenAI

| Provider | Login | Models |
|----------|-------|--------|
| **Codex** | `--codex-login` | gpt-5.1, gpt-5.1-codex, gpt-5.1-codex-max |
| **GitHub Copilot** | `--copilot-login` | gpt-4.1, gpt-4o, gpt-5-mini, gpt-5.1-codex-max |

### Others

| Provider | Login | Models |
|----------|-------|--------|
| **iFlow** | `--iflow-login` | qwen3-coder-plus, deepseek-r1, kimi-k2, glm-4.6 |
| **Cline** | `--cline-login` | minimax-m2, grok-code-fast-1 |
| **Qwen** | `--qwen-login` | qwen3-coder-plus, qwen3-coder-flash |

## API Endpoints

```
POST /v1/chat/completions     # OpenAI Chat API
POST /v1/completions          # Completions API
GET  /v1/models               # List available models
POST /v1beta/models/*         # Gemini-native API
POST /api/chat                # Ollama-compatible
```

## Architecture

Hub-and-spoke translation via Intermediate Representation (IR):

```
  Request Formats              Providers
  ───────────────              ─────────
     OpenAI ───┐            ┌─── Gemini CLI
     Claude ───┤            ├─── Antigravity
     Gemini ───┼── IR ──────┼─── Claude
     Ollama ───┤            ├─── Codex/Copilot
       Kiro ───┘            └─── iFlow/Kiro
```

Each provider implements 2 translations (to/from IR) instead of n² format converters.

**Smart Tool Call Normalization**: Auto-fixes parameter naming mismatches (`filePath` ↔ `file_path`, semantic synonyms).

**Dynamic Model Registry**: Reference counting tracks OAuth sessions, auto-hides models when quota exceeded.

## Installation

### Homebrew
```bash
brew tap nghyane/tap
brew install llm-mux
brew services start llm-mux
```

### Docker
```bash
docker pull nghyane/llm-mux
docker run -p 8318:8318 -v ~/.config/llm-mux:/root/.config/llm-mux nghyane/llm-mux
```

### From Source
```bash
go build -o llm-mux ./cmd/server/
./llm-mux -config config.yaml
```

## Configuration

```yaml
port: 8318
auth-dir: "~/.config/llm-mux/auth"
use-canonical-translator: true
```

Tokens are stored in `~/.config/llm-mux/auth/` and auto-refresh.

## How It Works

1. **OAuth Capture**: Performs same OAuth flow as official CLI tools
2. **Token Management**: Stores and auto-refreshes tokens
3. **Request Translation**: Converts OpenAI-format requests to provider-native format via IR
4. **Response Translation**: Converts provider responses back to OpenAI format
5. **Load Balancing**: Routes to available OAuth sessions, handles quota limits

## License

MIT License - see [LICENSE](LICENSE)
