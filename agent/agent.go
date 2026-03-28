package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cpra/muon/llm"
	"github.com/cpra/muon/message"
	"github.com/cpra/muon/tool"
)

// Hook is a function that receives events during the agent loop.
// Hooks must not modify events and should return quickly.
type Hook func(Event)

// Option configures an Agent.
type Option func(*Agent)

// WithHook returns an Option that sends agent loop events to h.
func WithHook(h Hook) Option {
	return func(a *Agent) { a.hook = h }
}

type Agent struct {
	client       *llm.Client
	tools        *tool.Registry
	systemPrompt string
	maxTurns     int
	hook         Hook
}

func New(client *llm.Client, tools *tool.Registry, maxTurns int, systemPrompt string, opts ...Option) *Agent {
	a := &Agent{
		client:       client,
		tools:        tools,
		systemPrompt: systemPrompt,
		maxTurns:     maxTurns,
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// Start begins a new multi-turn conversation. It returns a Session that can be
// continued with subsequent prompts, along with the model's first response text.
func (a *Agent) Start(ctx context.Context, prompt string) (*Session, string, error) {
	_ = a.client.EnsureModelInfo(ctx)

	s := &Session{
		agent: a,
		history: []message.Message{
			message.TextMessage(message.System, a.systemPrompt),
			message.TextMessage(message.User, prompt),
		},
	}

	content, err := s.runLoop(ctx)
	if err != nil {
		return nil, "", err
	}
	return s, content, nil
}

// Run executes a single-turn agent loop (no session persistence). This is a
// convenience wrapper around Start for callers that don't need multi-turn.
func (a *Agent) Run(ctx context.Context, prompt string) (string, error) {
	_, content, err := a.Start(ctx, prompt)
	return content, err
}

type toolCallResult struct {
	tool.Result
	args map[string]interface{}
}

func (a *Agent) executeToolCall(ctx context.Context, tc message.ToolCall) (*toolCallResult, error) {
	t, ok := a.tools.Get(tc.Function.Name)
	if !ok {
		return nil, fmt.Errorf("unknown tool: %s", tc.Function.Name)
	}

	var args map[string]interface{}
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
		return nil, fmt.Errorf("parse arguments: %w", err)
	}

	result, err := t.Run(ctx, args)
	if err != nil {
		return nil, err
	}

	return &toolCallResult{Result: result, args: args}, nil
}
