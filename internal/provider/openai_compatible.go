package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/Methamorphe/go-agent/internal/errs"
)

type OpenAICompatible struct {
	client  *http.Client
	baseURL string
	apiKey  string
}

func NewOpenAICompatible(client *http.Client, baseURL, apiKey string) (*OpenAICompatible, error) {
	if client == nil {
		client = http.DefaultClient
	}
	if strings.TrimSpace(baseURL) == "" {
		return nil, errs.New(errs.CodeInvalidArgument, "provider.compat.new", "base URL is required")
	}
	return &OpenAICompatible{
		client:  client,
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  strings.TrimSpace(apiKey),
	}, nil
}

func (p *OpenAICompatible) Name() string { return "openai-compatible" }

type chatToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type chatMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content,omitempty"`
	ToolCalls  []chatToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

type chatTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description,omitempty"`
		Parameters  json.RawMessage `json:"parameters"`
	} `json:"function"`
}

type chatRequest struct {
	Model         string        `json:"model"`
	Messages      []chatMessage `json:"messages"`
	Tools         []chatTool    `json:"tools,omitempty"`
	Stream        bool          `json:"stream"`
	StreamOptions struct {
		IncludeUsage bool `json:"include_usage"`
	} `json:"stream_options"`
}

type chatCallAccumulator struct {
	Index int
	ID    string
	Name  string
	Args  strings.Builder
}

func (p *OpenAICompatible) Stream(ctx context.Context, req Request, sink EventSink) (Result, error) {
	if err := req.Validate(); err != nil {
		return Result{}, err
	}
	if sink == nil {
		return Result{}, errs.New(errs.CodeInvalidArgument, "provider.compat.stream", "sink is nil")
	}

	messages, err := chatMessages(req.Messages)
	if err != nil {
		return Result{}, err
	}
	tools := make([]chatTool, 0, len(req.Tools))
	for _, definition := range req.Tools {
		var tool chatTool
		tool.Type = "function"
		tool.Function.Name = definition.Name
		tool.Function.Description = definition.Description
		tool.Function.Parameters = definition.InputSchema
		tools = append(tools, tool)
	}

	payload := chatRequest{Model: req.Model, Messages: messages, Tools: tools, Stream: true}
	payload.StreamOptions.IncludeUsage = true
	body, err := json.Marshal(payload)
	if err != nil {
		return Result{}, errs.Wrap(errs.CodeInvalidArgument, "provider.compat.stream", "encode request", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return Result{}, errs.Wrap(errs.CodeInvalidArgument, "provider.compat.stream", "create request", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		if ctx.Err() != nil {
			return Result{}, ctx.Err()
		}
		return Result{}, errs.Wrap(errs.CodeUnavailable, "provider.compat.stream", "send request", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		code := errs.CodeInvalidArgument
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			code = errs.CodeUnavailable
		}
		return Result{}, errs.New(code, "provider.compat.stream", fmt.Sprintf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(message))))
	}

	calls := map[int]*chatCallAccumulator{}
	result := Result{Provider: p.Name(), Model: req.Model}

	err = readSSE(ctx, resp.Body, func(data []byte) error {
		if string(data) == "[DONE]" {
			return nil
		}
		var chunk struct {
			ID    string `json:"id"`
			Model string `json:"model"`
			Usage *struct {
				PromptTokens     int64 `json:"prompt_tokens"`
				CompletionTokens int64 `json:"completion_tokens"`
				TotalTokens      int64 `json:"total_tokens"`
			} `json:"usage"`
			Choices []struct {
				FinishReason string `json:"finish_reason"`
				Delta        struct {
					Content   string `json:"content"`
					ToolCalls []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(data, &chunk); err != nil {
			return errs.Wrap(errs.CodeCorruption, "provider.compat.stream", "decode stream chunk", err)
		}
		if chunk.ID != "" {
			result.ProviderResult = chunk.ID
		}
		if chunk.Model != "" {
			result.Model = chunk.Model
		}
		if chunk.Usage != nil {
			result.Usage = knownUsage(chunk.Usage.PromptTokens, chunk.Usage.CompletionTokens, chunk.Usage.TotalTokens)
			if err := sink(Event{Kind: EventUsage, Usage: &result.Usage}); err != nil {
				return err
			}
		}

		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				if err := sink(Event{Kind: EventTextDelta, Text: choice.Delta.Content}); err != nil {
					return err
				}
			}
			for _, delta := range choice.Delta.ToolCalls {
				acc := calls[delta.Index]
				if acc == nil {
					acc = &chatCallAccumulator{Index: delta.Index}
					calls[delta.Index] = acc
				}
				if delta.ID != "" {
					acc.ID = delta.ID
				}
				if delta.Function.Name != "" {
					acc.Name = delta.Function.Name
				}
				if acc.Args.Len()+len(delta.Function.Arguments) > MaxToolArgumentsBytes {
					return errs.New(errs.CodeResourceExhausted, "provider.compat.stream", "tool arguments exceed limit")
				}
				acc.Args.WriteString(delta.Function.Arguments)
				if err := sink(Event{Kind: EventToolCallDelta, ToolCall: &ToolCallDelta{
					Index: delta.Index, ID: acc.ID, Name: acc.Name, ArgumentsDelta: delta.Function.Arguments,
				}}); err != nil {
					return err
				}
			}
			if choice.FinishReason != "" {
				result.FinishReason = mapChatFinishReason(choice.FinishReason)
			}
		}
		return nil
	})
	if err != nil {
		if ctx.Err() != nil {
			return Result{}, ctx.Err()
		}
		return Result{}, err
	}

	indexes := make([]int, 0, len(calls))
	for index := range calls {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	for _, index := range indexes {
		acc := calls[index]
		call := ToolCall{ID: acc.ID, Name: acc.Name, Arguments: json.RawMessage(acc.Args.String())}
		if err := call.Validate(); err != nil {
			return Result{}, err
		}
		result.ToolCalls = append(result.ToolCalls, call)
	}
	if len(result.ToolCalls) > 0 {
		result.FinishReason = FinishToolCalls
	} else if result.FinishReason == "" {
		result.FinishReason = FinishStop
	}
	return result, nil
}

func chatMessages(messages []Message) ([]chatMessage, error) {
	result := make([]chatMessage, 0, len(messages))
	for _, message := range messages {
		if message.Role == RoleTool {
			for _, part := range message.Parts {
				if part.Kind != PartToolResult {
					continue
				}
				result = append(result, chatMessage{
					Role:       "tool",
					Content:    part.ToolResult.Content,
					ToolCallID: part.ToolResult.CallID,
				})
			}
			continue
		}

		chat := chatMessage{Role: string(message.Role)}
		var text strings.Builder
		for _, part := range message.Parts {
			switch part.Kind {
			case PartText:
				text.WriteString(part.Text)
			case PartToolCall:
				var toolCall chatToolCall
				toolCall.ID = part.ToolCall.ID
				toolCall.Type = "function"
				toolCall.Function.Name = part.ToolCall.Name
				toolCall.Function.Arguments = string(part.ToolCall.Arguments)
				chat.ToolCalls = append(chat.ToolCalls, toolCall)
			}
		}
		chat.Content = text.String()
		result = append(result, chat)
	}
	return result, nil
}

func mapChatFinishReason(reason string) FinishReason {
	switch reason {
	case "stop":
		return FinishStop
	case "tool_calls", "function_call":
		return FinishToolCalls
	case "length":
		return FinishLength
	default:
		return FinishOther
	}
}
