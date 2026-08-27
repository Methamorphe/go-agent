package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAIResponsesMapsTextToolsAndUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintln(w, `data: {"type":"response.output_text.delta","delta":"hello "}`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, `data: {"type":"response.output_item.added","output_index":1,"item":{"id":"item_1","type":"function_call","call_id":"call_1","name":"observe","arguments":""}}`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, `data: {"type":"response.function_call_arguments.delta","item_id":"item_1","output_index":1,"delta":"{\"operation\":\"read_file\","}`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, `data: {"type":"response.function_call_arguments.done","item_id":"item_1","output_index":1,"name":"observe","arguments":"{\"operation\":\"read_file\",\"path\":\"README.md\"}"}`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, `data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-test","usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15}}}`)
		fmt.Fprintln(w)
	}))
	defer server.Close()

	client, err := NewOpenAIResponses(server.Client(), server.URL+"/v1", "key")
	if err != nil {
		t.Fatal(err)
	}
	var text string
	result, err := client.Stream(context.Background(), validRequest(), func(event Event) error {
		if event.Kind == EventTextDelta {
			text += event.Text
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if text != "hello " {
		t.Fatalf("text = %q", text)
	}
	if result.ProviderResult != "resp_1" || result.FinishReason != FinishToolCalls || len(result.ToolCalls) != 1 {
		t.Fatalf("result = %+v", result)
	}
	if !json.Valid(result.ToolCalls[0].Arguments) {
		t.Fatalf("arguments = %q", result.ToolCalls[0].Arguments)
	}
}

func TestOpenAICompatibleMapsStreamingToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintln(w, `data: {"id":"chat_1","model":"local","choices":[{"delta":{"content":"checking"},"finish_reason":""}]}`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, `data: {"id":"chat_1","model":"local","choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"execute","arguments":"{\"executable\":\"go\""}}]},"finish_reason":""}]}`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, `data: {"id":"chat_1","model":"local","choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":",\"args\":[\"test\",\"./...\"]}"}}]},"finish_reason":"tool_calls"}]}`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, `data: {"id":"chat_1","model":"local","choices":[],"usage":{"prompt_tokens":8,"completion_tokens":4,"total_tokens":12}}`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, `data: [DONE]`)
		fmt.Fprintln(w)
	}))
	defer server.Close()

	client, err := NewOpenAICompatible(server.Client(), server.URL+"/v1", "")
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.Stream(context.Background(), validRequest(), func(Event) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if result.FinishReason != FinishToolCalls || len(result.ToolCalls) != 1 {
		t.Fatalf("result = %+v", result)
	}
	if result.ToolCalls[0].Name != "execute" || !json.Valid(result.ToolCalls[0].Arguments) {
		t.Fatalf("tool call = %+v", result.ToolCalls[0])
	}
	if result.Usage.TotalTokens == nil || *result.Usage.TotalTokens != 12 {
		t.Fatalf("usage = %+v", result.Usage)
	}
}
