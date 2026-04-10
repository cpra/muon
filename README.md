# muon

A minimal yet functional Go agent loop for LLM-assisted coding. Sends a system prompt and user message to an OpenAI-compatible chat completions API, executes tool calls, and returns the final response. Supports multi-turn conversations via a `Session` type that preserves history across user prompts.

## Architecture

```
main.go          CLI entry point — loads config, wires packages together
config/          YAML configuration and provider resolution
llm/             HTTP client for OpenAI-compatible chat completions
message/         Message types matching the chat API wire format
tool/            Tool interface, registry, and schema helpers
agent/           Agent loop with hook-based event system and multi-turn sessions
```

## Configuration

Two YAML files control behavior:

**`config.yml`** — model selection, limits, and system prompt:

```yaml
model: zai/glm-5.1 # <provider>/<model> format
max_tokens: 8192 # max response tokens per turn
max_turns: 50 # agent loop iteration limit
# max_context_tokens: 128000   # override auto-detected context window (fetched from provider API)
system_prompt: |
  You are muon, a coding assistant.
```

The model provider name is looked up in `providers.yml` to resolve the base URL and API key.

**`providers.yml`** — API endpoints and credentials (gitignored):

```yaml
- name: zai
  url: https://api.z.ai/api/coding/paas/v4
  key: "${ZAI_API_KEY}"
```

API keys use `${ENV_VAR}` syntax — the value is read from the environment at startup.

## Usage

```bash
# Single-turn: run a prompt and exit
go run . "explain the agent loop"

# Interactive multi-turn: no arguments starts a REPL
go run .
# > write a function that checks if a number is prime
# ...response...
# > now add tests for it
# ...response...
# > /exit
# session cost: $0.001234 (1234 prompt + 567 completion tokens)

# List models available from the configured provider
go run . list-models
```

## The Agent Loop

The core loop lives in `agent.Session`. A conversation starts with `Agent.Start`, which returns a `Session` that can be continued with `Session.Continue`. Here is what happens on each turn:

```
                    ┌───────────────────────────┐
                    │   Build initial history   │
                    │   system + user message   │
                    └────────────┬──────────────┘
                                 │
                    ┌────────────▼──────────────┐
               ┌───►│  Send history to LLM API  │
               │    │  (with tool definitions)  │
               │    └────────────┬──────────────┘
               │                 │
               │    ┌────────────▼──────────────┐
               │    │  LLM returns response     │
               │    │  + token usage            │
               │    └────────────┬──────────────┘
               │                 │
               │         ┌───────▼───────┐
               │         │  Hook fires:  │
               │         │ LLMResponse   │
               │         │ Event         │
               │         └───────┬───────┘
               │                 │
               │          ┌──────▼──────┐    No tool calls
               │          │ Tool calls? ├───────────────► Return content
               │          └──────┬──────┘
               │                 │ Yes
               │          ┌──────▼──────┐
               │          │ For each    │
               │          │ tool call:  │
               │          │             │
               │          │ 1. Look up  │
               │          │    tool in  │
               │          │    registry │
               │          │ 2. Parse    │
               │          │    args     │
               │          │ 3. Execute  │
               │          │ 4. Append   │
               │          │    result   │
               │          │    to       │
               │          │    history  │
               │          │             │
               │          │ Hook fires: │
               │          │ ToolCall    │
               │          │ Event       │
               │          └──────┬──────┘
               │                 │
               └─────────────────┘
                  (next turn, unless max_turns exceeded)
```

### Turn-by-turn detail

1. **Initial history** — `Agent.Start` creates a `Session` with a `[]message.Message` containing the system prompt and user prompt. Subsequent calls to `Session.Continue` append the new user message to the existing history.
2. **LLM request** — The full history and tool definitions (from the registry) are sent via `llm.Client.Create`. This returns the assistant message, token usage, and any tool calls.
3. **`LLMResponseEvent` hook** — Fired once per turn with the response content text, `llm.Usage` (prompt/completion/total tokens), and `llm.CostInfo` (cost in USD, zero-valued if pricing is unavailable). If no hook is set, this is a no-op.
4. **Terminal check** — If the response has no tool calls, the content is returned as the final result.
5. **Tool execution** — Each tool call is dispatched to the matching `tool.Tool` from the registry. The parsed arguments and `tool.Result` (content + error flag) are appended to history as a `tool` role message.
6. **`ToolCallEvent` hook** — Fired after each tool finishes, carrying the tool name, parsed args, and result. Failed tool calls still fire the hook with `Result.IsError = true`.
7. **`TurnEndEvent` hook** — Fired at the end of each loop iteration with accumulated session statistics (`AccumulatedUsage` and `AccumulatedCost`).
8. **Loop** — Control returns to step 2 with the updated history. The loop stops after `max_turns` iterations.

## Event Hooks

Hooks allow callers to observe the agent loop without modifying it. Pass a function to `agent.New` via the `WithHook` option:

```go
a := agent.New(client, registry, 50, systemPrompt,
    agent.WithHook(func(e agent.Event) {
        switch e := e.(type) {
        case agent.LLMResponseEvent:
            fmt.Fprintf(os.Stderr, "turn %d: %d/%d tokens, cost $%.6f\n",
                e.Turn, e.Usage.PromptTokens, client.ContextLength(), e.Cost.TotalCost)
        case agent.ToolCallEvent:
            fmt.Fprintf(os.Stderr, "  tool: %s (error=%v)\n",
                e.Name, e.Result.IsError)
        case agent.TurnEndEvent:
            fmt.Fprintf(os.Stderr, "  session total: $%.6f\n",
                e.AccumulatedCost.TotalCost)
        }
    }),
)
```

For multi-turn conversations, use `Start` to begin a session and `Continue` to send follow-up prompts:

```go
session, reply, err := a.Start(ctx, "explain the agent loop")
// ... later ...
reply, err = session.Continue(ctx, "now add tests for it")
```

For single-shot use, use the `Run` convenience method:

```go
reply, err := a.Run(ctx, "explain the agent loop")
```

### Event types

`Event` is a sealed interface — only the types defined in the `agent` package implement it. Use a type switch to handle each kind:

| Event              | When                                           | Key fields                                            |
| ------------------ | ---------------------------------------------- | ----------------------------------------------------- |
| `LLMResponseEvent` | After each LLM response, before tool execution | `Turn`, `Message` (text content), `Usage`, `Cost`     |
| `ToolCallEvent`    | After each tool finishes execution             | `Turn`, `Name`, `Args`, `Result` (includes `IsError`) |
| `TurnEndEvent`     | At the end of each loop iteration              | `Turn`, `AccumulatedUsage`, `AccumulatedCost`         |

## Token Budget & Cost Tracking

`llm.Client` tracks context window limits and per-turn costs:

### Context window

- On first use, `Agent.Start` calls `Client.EnsureModelInfo()` which fetches the model list from the provider's `/models` endpoint and caches metadata for the configured model.
- `Client.ContextLength()` returns the model's maximum input tokens. It checks (1) an explicit config value, (2) the dynamically fetched `context_length` from the provider, (3) a 128k default.
- Each `Create` call returns `Usage` with `PromptTokens`, `CompletionTokens`, and `TotalTokens` decoded from the API response.

### Cost calculation

- If the provider returns pricing data (e.g. OpenRouter's `pricing.prompt` / `pricing.completion` fields), `Client.CalculateCost(usage)` returns a `CostInfo` with `PromptCost`, `CompletionCost`, and `TotalCost` in USD.
- `LLMResponseEvent.Cost` is populated automatically — zero-valued when pricing is unavailable.

## Tools

Tools implement the `tool.Tool` interface:

```go
type Tool interface {
    Name() string
    Description() string
    Parameters() map[string]interface{}
    Run(ctx context.Context, args map[string]interface{}) (Result, error)
}
```

Register tools with a `tool.Registry`. The registry produces OpenAI function-calling definitions in registration order (deterministic). Schema helpers (`ObjectSchema`, `StringParam`, `IntParam`) build parameter definitions.

Built-in tool types exist in `tool/`, but they are currently stubs and are not
registered by default:

| Tool    | Description                        |
| ------- | ---------------------------------- |
| `bash`  | Execute shell commands             |
| `read`  | Read files from the filesystem     |
| `write` | Write content to files             |
| `edit`  | Exact string replacements in files |

## LLM Client

`llm.Client` wraps an OpenAI-compatible chat completions API:

- **Model metadata** — `EnsureModelInfo()` lazily fetches and caches context length and pricing from the provider's `/models` endpoint. OpenRouter returns `context_length` and `pricing` per model; other providers return zero-valued fields harmlessly.
- **Cost tracking** — `CalculateCost(usage)` multiplies token counts by cached per-token prices.
- **Retries** — Exponential backoff (1s base, 2x growth, 5 max) on HTTP 429.
- **Endpoints** — `POST /chat/completions` for chat, `GET /models` for model listing.
- **Auth** — Bearer token from the resolved provider API key.
- **Timeout** — 5 minute per-request HTTP timeout.

## Development

The project uses a Nix flake for the development environment. Enter it with `nix develop`.
