package message

import "encoding/json"

type Role string

const (
	User      Role = "user"
	Assistant Role = "assistant"
	System    Role = "system"
	Tool      Role = "tool"
)

type Function struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ToolCall struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"`
	Function Function `json:"function"`
}

type Message struct {
	Role       Role       `json:"role"`
	Content    *string    `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

func TextMessage(role Role, content string) Message {
	return Message{Role: role, Content: &content}
}

func ToolResultMessage(toolCallID string, content string) Message {
	return Message{Role: Tool, ToolCallID: toolCallID, Content: &content}
}

func (m Message) MarshalJSON() ([]byte, error) {
	type Alias Message
	raw := struct {
		Alias
		Content interface{} `json:"content,omitempty"`
	}{
		Alias: Alias(m),
	}
	if m.Content != nil {
		raw.Content = *m.Content
	}
	return json.Marshal(raw)
}
