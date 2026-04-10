package agent

import (
	"context"
	"fmt"

	"github.com/cpra/muon/llm"
	"github.com/cpra/muon/message"
	"github.com/cpra/muon/tool"
)

// Session holds the mutable state for a multi-turn conversation with an agent.
type Session struct {
	agent   *Agent
	history []message.Message
	usage   llm.Usage
	cost    llm.CostInfo
}

// Continue appends a new user message and runs the agent loop until the model
// responds without tool calls (or max turns is exceeded).
func (s *Session) Continue(ctx context.Context, prompt string) (string, error) {
	s.history = append(s.history, message.TextMessage(message.User, prompt))
	return s.runLoop(ctx)
}

// Usage returns the accumulated token usage across all turns in this session.
func (s *Session) Usage() llm.Usage {
	return s.usage
}

// Cost returns the accumulated monetary cost across all turns in this session.
func (s *Session) Cost() llm.CostInfo {
	return s.cost
}

// History returns a copy of the conversation history.
func (s *Session) History() []message.Message {
	out := make([]message.Message, len(s.history))
	copy(out, s.history)
	return out
}

func (s *Session) runLoop(ctx context.Context) (string, error) {
	toolDefs := s.agent.tools.Definitions()

	for i := 0; i < s.agent.maxTurns; i++ {
		resp, usage, err := s.agent.client.Create(ctx, s.history, toolDefs)
		if err != nil {
			return "", fmt.Errorf("turn %d: LLM request failed: %w", i+1, err)
		}

		s.history = append(s.history, *resp)
		s.usage.PromptTokens += usage.PromptTokens
		s.usage.CompletionTokens += usage.CompletionTokens
		s.usage.TotalTokens += usage.TotalTokens

		turnCost, err := s.agent.client.CalculateCost(usage)
		if err != nil {
			return "", fmt.Errorf("turn %d: calculate cost: %w", i+1, err)
		}
		s.cost.PromptCost += turnCost.PromptCost
		s.cost.CompletionCost += turnCost.CompletionCost
		s.cost.TotalCost += turnCost.TotalCost

		content := ""
		if resp.Content != nil {
			content = *resp.Content
		}

		if s.agent.hook != nil {
			s.agent.hook(LLMResponseEvent{Turn: i + 1, Message: content, Usage: usage, Cost: turnCost})
		}

		if len(resp.ToolCalls) == 0 {
			if s.agent.hook != nil {
				s.agent.hook(TurnEndEvent{
					Turn:             i + 1,
					AccumulatedUsage: s.usage,
					AccumulatedCost:  s.cost,
				})
			}
			return content, nil
		}

		for _, tc := range resp.ToolCalls {
			result, err := s.agent.executeToolCall(ctx, tc)
			if err != nil {
				result = &toolCallResult{Result: tool.Result{Content: fmt.Sprintf("error: %v", err), IsError: true}}
			}
			s.history = append(s.history, message.ToolResultMessage(tc.ID, result.Content))

			if s.agent.hook != nil {
				s.agent.hook(ToolCallEvent{
					Turn:   i + 1,
					Name:   tc.Function.Name,
					Args:   result.args,
					Result: result.Result,
				})
			}
		}

		if s.agent.hook != nil {
			s.agent.hook(TurnEndEvent{
				Turn:             i + 1,
				AccumulatedUsage: s.usage,
				AccumulatedCost:  s.cost,
			})
		}
	}

	return "", fmt.Errorf("exceeded max turns (%d)", s.agent.maxTurns)
}
