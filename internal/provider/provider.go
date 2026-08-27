package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
)

const MaxToolArgumentsBytes = 64 << 10

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type PartKind string

const (
	PartText       PartKind = "text"
	PartToolCall   PartKind = "tool_call"
	PartToolResult PartKind = "tool_result"
)

type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type ToolResult struct {
	CallID  string `json:"call_id"`
	Content string `json:"content"`
}

type ContentPart struct {
	Kind       PartKind    `json:"kind"`
	Text       string      `json:"text,omitempty"`
	ToolCall   *ToolCall   `json:"tool_call,omitempty"`
	ToolResult *ToolResult `json:"tool_result,omitempty"`
}

func TextPart(text string) ContentPart {
	return ContentPart{Kind: PartText, Text: text}
}

func ToolCallPart(call ToolCall) ContentPart {
	return ContentPart{Kind: PartToolCall, ToolCall: &call}
}

func ToolResultPart(result ToolResult) ContentPart {
	return ContentPart{Kind: PartToolResult, ToolResult: &result}
}

type Message struct {
	Role  Role          `json:"role"`
	Parts []ContentPart `json:"parts"`
}

type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type Request struct {
	AgentID      id.AgentID       `json:"agent_id"`
	InvocationID id.InvocationID  `json:"invocation_id"`
	Model        string           `json:"model"`
	Messages     []Message        `json:"messages"`
	Tools        []ToolDefinition `json:"tools,omitempty"`
}

type EventKind string

const (
	EventTextDelta     EventKind = "text_delta"
	EventToolCallDelta EventKind = "tool_call_delta"
	EventUsage         EventKind = "usage"
)

type ToolCallDelta struct {
	Index          int    `json:"index"`
	ID             string `json:"id,omitempty"`
	Name           string `json:"name,omitempty"`
	ArgumentsDelta string `json:"arguments_delta,omitempty"`
}

type Usage struct {
	InputTokens  *int64 `json:"input_tokens,omitempty"`
	OutputTokens *int64 `json:"output_tokens,omitempty"`
	TotalTokens  *int64 `json:"total_tokens,omitempty"`
}

type Event struct {
	Kind     EventKind      `json:"kind"`
	Text     string         `json:"text,omitempty"`
	ToolCall *ToolCallDelta `json:"tool_call,omitempty"`
	Usage    *Usage         `json:"usage,omitempty"`
}

type FinishReason string

const (
	FinishStop      FinishReason = "stop"
	FinishToolCalls FinishReason = "tool_calls"
	FinishLength    FinishReason = "length"
	FinishOther     FinishReason = "other"
)

type Result struct {
	Provider       string       `json:"provider"`
	Model          string       `json:"model"`
	ProviderResult string       `json:"provider_result_id,omitempty"`
	FinishReason   FinishReason `json:"finish_reason"`
	Usage          Usage        `json:"usage"`
	ToolCalls      []ToolCall   `json:"tool_calls,omitempty"`
}

type EventSink func(Event) error

type Provider interface {
	Name() string
	Stream(context.Context, Request, EventSink) (Result, error)
}

func (r Request) Validate() error {
	if r.AgentID == "" || r.InvocationID == "" {
		return errs.New(errs.CodeInvalidArgument, "provider.request.validate", "agent_id and invocation_id are required")
	}
	if strings.TrimSpace(r.Model) == "" {
		return errs.New(errs.CodeInvalidArgument, "provider.request.validate", "model is required")
	}
	if len(r.Messages) == 0 {
		return errs.New(errs.CodeInvalidArgument, "provider.request.validate", "at least one message is required")
	}
	for i, message := range r.Messages {
		if err := message.Validate(); err != nil {
			return fmt.Errorf("message %d: %w", i, err)
		}
	}
	for i, tool := range r.Tools {
		if err := tool.Validate(); err != nil {
			return fmt.Errorf("tool %d: %w", i, err)
		}
	}
	return nil
}

func (m Message) Validate() error {
	switch m.Role {
	case RoleSystem, RoleUser, RoleAssistant, RoleTool:
	default:
		return errs.New(errs.CodeInvalidArgument, "provider.message.validate", "unknown role")
	}
	if len(m.Parts) == 0 {
		return errs.New(errs.CodeInvalidArgument, "provider.message.validate", "message has no content parts")
	}
	for _, part := range m.Parts {
		switch part.Kind {
		case PartText:
			if part.ToolCall != nil || part.ToolResult != nil {
				return errs.New(errs.CodeInvalidArgument, "provider.message.validate", "text part contains tool data")
			}
		case PartToolCall:
			if m.Role != RoleAssistant || part.ToolCall == nil {
				return errs.New(errs.CodeInvalidArgument, "provider.message.validate", "tool call requires assistant role")
			}
			if err := part.ToolCall.Validate(); err != nil {
				return err
			}
		case PartToolResult:
			if m.Role != RoleTool || part.ToolResult == nil || strings.TrimSpace(part.ToolResult.CallID) == "" {
				return errs.New(errs.CodeInvalidArgument, "provider.message.validate", "tool result requires tool role and call id")
			}
		default:
			return errs.New(errs.CodeInvalidArgument, "provider.message.validate", "unknown content part kind")
		}
	}
	return nil
}

func (t ToolCall) Validate() error {
	if strings.TrimSpace(t.ID) == "" || strings.TrimSpace(t.Name) == "" {
		return errs.New(errs.CodeInvalidArgument, "provider.tool_call.validate", "tool call id and name are required")
	}
	if len(t.Arguments) > MaxToolArgumentsBytes {
		return errs.New(errs.CodeResourceExhausted, "provider.tool_call.validate", "tool arguments exceed limit")
	}
	if !json.Valid(t.Arguments) {
		return errs.New(errs.CodeInvalidArgument, "provider.tool_call.validate", "tool arguments must be valid JSON")
	}
	return nil
}

func (t ToolDefinition) Validate() error {
	if strings.TrimSpace(t.Name) == "" {
		return errs.New(errs.CodeInvalidArgument, "provider.tool.validate", "tool name is required")
	}
	if len(t.InputSchema) == 0 || !json.Valid(t.InputSchema) {
		return errs.New(errs.CodeInvalidArgument, "provider.tool.validate", "tool input schema must be valid JSON")
	}
	return nil
}

func (e Event) Validate() error {
	switch e.Kind {
	case EventTextDelta:
		if e.ToolCall != nil || e.Usage != nil {
			return errs.New(errs.CodeInvalidArgument, "provider.event.validate", "text event contains unrelated fields")
		}
	case EventToolCallDelta:
		if e.ToolCall == nil {
			return errs.New(errs.CodeInvalidArgument, "provider.event.validate", "tool call delta is missing")
		}
	case EventUsage:
		if e.Usage == nil {
			return errs.New(errs.CodeInvalidArgument, "provider.event.validate", "usage is missing")
		}
	default:
		return errs.New(errs.CodeInvalidArgument, "provider.event.validate", "unknown provider event kind")
	}
	return nil
}
