package agent

import (
	"github.com/cpra/muon/llm"
	"github.com/cpra/muon/tool"
)

// Event is a sealed interface for agent loop events. Only the concrete types
// defined in this package implement it.
type Event interface{ agentEvent() }

// LLMResponseEvent is emitted after each LLM response, before tool execution.
type LLMResponseEvent struct {
	Turn    int
	Message string // content text (empty if the response only contains tool calls)
	Usage   llm.Usage
	Cost    llm.CostInfo
}

func (LLMResponseEvent) agentEvent() {}

// ToolCallEvent is emitted after a tool finishes execution.
type ToolCallEvent struct {
	Turn   int
	Name   string
	Args   map[string]interface{}
	Result tool.Result
}

func (ToolCallEvent) agentEvent() {}

// TurnEndEvent is emitted at the end of each agent loop iteration with
// accumulated session statistics.
type TurnEndEvent struct {
	Turn             int
	AccumulatedUsage llm.Usage
	AccumulatedCost  llm.CostInfo
}

func (TurnEndEvent) agentEvent() {}
