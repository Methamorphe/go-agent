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

type OpenAIResponses struct {
	client  *http.Client
	baseURL string
	apiKey  string
}

func NewOpenAIResponses(client *http.Client, baseURL, apiKey string) (*OpenAIResponses, error) {
	if client == nil {
		client = http.DefaultClient
	}
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "https://api.openai.com/v1"
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, errs.New(errs.CodeInvalidArgument, "provider.openai.new", "API key is required")
	}
	return &OpenAIResponses{
		client:  client,
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
	}, nil
}

func (p *OpenAIResponses) Name() string { return "openai" }

type responsesTool struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
	Strict      bool            `json:"strict"`
}

type responsesRequest struct {
	Model  string          `json:"model"`
	Input  []any           `json:"input"`
	Tools  []responsesTool `json:"tools,omitempty"`
	Stream bool            `json:"stream"`
	Store  bool            `json:"store"`
}

type responseCallAccumulator struct {
	Index  int
	ItemID string
	CallID string
	Name   string
	Args   strings.Builder
}

func (p *OpenAIResponses) Stream(ctx context.Context, req Request, sink EventSink) (Result, error) {
	if err := req.Validate(); err != nil {
		return Result{}, err
	}
	if sink == nil {
		return Result{}, errs.New(errs.CodeInvalidArgument, "provider.openai.stream", "sink is nil")
	}

	input, err := responsesInput(req.Messages)
	if err != nil {
		return Result{}, err
	}
	tools := make([]responsesTool, 0, len(req.Tools))
	for _, tool := range req.Tools {
		tools = append(tools, responsesTool{
			Type:        "function",
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  tool.InputSchema,
			Strict:      false,
		})
	}

	body, err := json.Marshal(responsesRequest{
		Model:  req.Model,
		Input:  input,
		Tools:  tools,
		Stream: true,
		Store:  false,
	})
	if err != nil {
		return Result{}, errs.Wrap(errs.CodeInvalidArgument, "provider.openai.stream", "encode request", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/responses", bytes.NewReader(body))
	if err != nil {
		return Result{}, errs.Wrap(errs.CodeInvalidArgument, "provider.openai.stream", "create request", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		if ctx.Err() != nil {
			return Result{}, ctx.Err()
		}
		return Result{}, errs.Wrap(errs.CodeUnavailable, "provider.openai.stream", "send request", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		code := errs.CodeInvalidArgument
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			code = errs.CodeUnavailable
		}
		return Result{}, errs.New(code, "provider.openai.stream", fmt.Sprintf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(message))))
	}

	calls := map[string]*responseCallAccumulator{}
	var result Result
	result.Provider = p.Name()
	result.Model = req.Model

	err = readSSE(ctx, resp.Body, func(data []byte) error {
		if string(data) == "[DONE]" {
			return nil
		}

		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(data, &envelope); err != nil {
			return errs.Wrap(errs.CodeCorruption, "provider.openai.stream", "decode SSE event type", err)
		}

		switch envelope.Type {
		case "response.output_text.delta":
			var event struct {
				Delta string `json:"delta"`
			}
			if err := json.Unmarshal(data, &event); err != nil {
				return errs.Wrap(errs.CodeCorruption, "provider.openai.stream", "decode text delta", err)
			}
			return sink(Event{Kind: EventTextDelta, Text: event.Delta})

		case "response.output_item.added", "response.output_item.done":
			var event struct {
				OutputIndex int `json:"output_index"`
				Item        struct {
					ID        string `json:"id"`
					Type      string `json:"type"`
					CallID    string `json:"call_id"`
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"item"`
			}
			if err := json.Unmarshal(data, &event); err != nil {
				return errs.Wrap(errs.CodeCorruption, "provider.openai.stream", "decode output item", err)
			}
			if event.Item.Type != "function_call" {
				return nil
			}
			acc := calls[event.Item.ID]
			if acc == nil {
				acc = &responseCallAccumulator{Index: event.OutputIndex, ItemID: event.Item.ID}
				calls[event.Item.ID] = acc
			}
			if event.Item.CallID != "" {
				acc.CallID = event.Item.CallID
			}
			if event.Item.Name != "" {
				acc.Name = event.Item.Name
			}
			if envelope.Type == "response.output_item.done" && event.Item.Arguments != "" {
				if len(event.Item.Arguments) > MaxToolArgumentsBytes {
					return errs.New(errs.CodeResourceExhausted, "provider.openai.stream", "tool arguments exceed limit")
				}
				acc.Args.Reset()
				acc.Args.WriteString(event.Item.Arguments)
			}
			return nil

		case "response.function_call_arguments.delta":
			var event struct {
				ItemID      string `json:"item_id"`
				OutputIndex int    `json:"output_index"`
				Delta       string `json:"delta"`
			}
			if err := json.Unmarshal(data, &event); err != nil {
				return errs.Wrap(errs.CodeCorruption, "provider.openai.stream", "decode tool delta", err)
			}
			acc := calls[event.ItemID]
			if acc == nil {
				acc = &responseCallAccumulator{Index: event.OutputIndex, ItemID: event.ItemID}
				calls[event.ItemID] = acc
			}
			if acc.Args.Len()+len(event.Delta) > MaxToolArgumentsBytes {
				return errs.New(errs.CodeResourceExhausted, "provider.openai.stream", "tool arguments exceed limit")
			}
			acc.Args.WriteString(event.Delta)
			return sink(Event{Kind: EventToolCallDelta, ToolCall: &ToolCallDelta{
				Index: event.OutputIndex, ID: acc.CallID, Name: acc.Name, ArgumentsDelta: event.Delta,
			}})

		case "response.function_call_arguments.done":
			var event struct {
				ItemID      string `json:"item_id"`
				OutputIndex int    `json:"output_index"`
				Name        string `json:"name"`
				Arguments   string `json:"arguments"`
			}
			if err := json.Unmarshal(data, &event); err != nil {
				return errs.Wrap(errs.CodeCorruption, "provider.openai.stream", "decode tool completion", err)
			}
			if len(event.Arguments) > MaxToolArgumentsBytes {
				return errs.New(errs.CodeResourceExhausted, "provider.openai.stream", "tool arguments exceed limit")
			}
			acc := calls[event.ItemID]
			if acc == nil {
				acc = &responseCallAccumulator{Index: event.OutputIndex, ItemID: event.ItemID}
				calls[event.ItemID] = acc
			}
			acc.Name = event.Name
			acc.Args.Reset()
			acc.Args.WriteString(event.Arguments)
			return nil

		case "response.completed":
			var event struct {
				Response struct {
					ID    string `json:"id"`
					Model string `json:"model"`
					Usage struct {
						InputTokens  int64 `json:"input_tokens"`
						OutputTokens int64 `json:"output_tokens"`
						TotalTokens  int64 `json:"total_tokens"`
					} `json:"usage"`
				} `json:"response"`
			}
			if err := json.Unmarshal(data, &event); err != nil {
				return errs.Wrap(errs.CodeCorruption, "provider.openai.stream", "decode completion", err)
			}
			result.ProviderResult = event.Response.ID
			if event.Response.Model != "" {
				result.Model = event.Response.Model
			}
			result.Usage = knownUsage(event.Response.Usage.InputTokens, event.Response.Usage.OutputTokens, event.Response.Usage.TotalTokens)
			if err := sink(Event{Kind: EventUsage, Usage: &result.Usage}); err != nil {
				return err
			}
			return nil

		case "response.failed", "error":
			var event struct {
				Error struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
				Message string `json:"message"`
			}
			_ = json.Unmarshal(data, &event)
			message := event.Error.Message
			if message == "" {
				message = event.Message
			}
			if message == "" {
				message = "provider response failed"
			}
			return errs.New(errs.CodeUnavailable, "provider.openai.stream", message)
		}
		return nil
	})
	if err != nil {
		if ctx.Err() != nil {
			return Result{}, ctx.Err()
		}
		return Result{}, err
	}

	result.ToolCalls, err = finalizeResponseCalls(calls)
	if err != nil {
		return Result{}, err
	}
	if len(result.ToolCalls) > 0 {
		result.FinishReason = FinishToolCalls
	} else if result.FinishReason == "" {
		result.FinishReason = FinishStop
	}
	return result, nil
}

func responsesInput(messages []Message) ([]any, error) {
	input := make([]any, 0, len(messages))
	for _, message := range messages {
		var text strings.Builder
		var calls []ToolCall
		var results []ToolResult
		for _, part := range message.Parts {
			switch part.Kind {
			case PartText:
				text.WriteString(part.Text)
			case PartToolCall:
				calls = append(calls, *part.ToolCall)
			case PartToolResult:
				results = append(results, *part.ToolResult)
			}
		}
		if text.Len() > 0 {
			input = append(input, map[string]any{
				"role":    string(message.Role),
				"content": text.String(),
			})
		}
		for _, call := range calls {
			input = append(input, map[string]any{
				"type":      "function_call",
				"call_id":   call.ID,
				"name":      call.Name,
				"arguments": string(call.Arguments),
			})
		}
		for _, result := range results {
			input = append(input, map[string]any{
				"type":    "function_call_output",
				"call_id": result.CallID,
				"output":  result.Content,
			})
		}
	}
	return input, nil
}
func finalizeResponseCalls(calls map[string]*responseCallAccumulator) ([]ToolCall, error) {
	ordered := make([]*responseCallAccumulator, 0, len(calls))
	for _, call := range calls {
		ordered = append(ordered, call)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Index < ordered[j].Index })

	result := make([]ToolCall, 0, len(ordered))
	for _, call := range ordered {
		if call.CallID == "" {
			call.CallID = call.ItemID
		}
		toolCall := ToolCall{ID: call.CallID, Name: call.Name, Arguments: json.RawMessage(call.Args.String())}
		if err := toolCall.Validate(); err != nil {
			return nil, err
		}
		result = append(result, toolCall)
	}
	return result, nil
}

func knownUsage(input, output, total int64) Usage {
	return Usage{InputTokens: &input, OutputTokens: &output, TotalTokens: &total}
}
