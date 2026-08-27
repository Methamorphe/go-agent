package provider

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Methamorphe/go-agent/internal/id"
)

func validRequest() Request {
	return Request{
		AgentID:      id.AgentID("agt_test"),
		InvocationID: id.InvocationID("inv_test"),
		Model:        "test-model",
		Messages: []Message{{
			Role:  RoleUser,
			Parts: []ContentPart{TextPart("hello")},
		}},
		Tools: []ToolDefinition{{
			Name:        "observe",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		}},
	}
}

func TestFakeProviderDeterministicOrderAndUsage(t *testing.T) {
	input, output, total := int64(4), int64(2), int64(6)
	fake := NewFake(FakeScript{
		Events: []Event{
			{Kind: EventTextDelta, Text: "hello"},
			{Kind: EventTextDelta, Text: " world"},
			{Kind: EventUsage, Usage: &Usage{InputTokens: &input, OutputTokens: &output, TotalTokens: &total}},
		},
		Result: Result{
			FinishReason: FinishStop,
			Usage:        Usage{InputTokens: &input, OutputTokens: &output, TotalTokens: &total},
		},
	})

	var got []EventKind
	result, err := fake.Stream(context.Background(), validRequest(), func(event Event) error {
		got = append(got, event.Kind)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []EventKind{EventTextDelta, EventTextDelta, EventUsage}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
	if result.Usage.TotalTokens == nil || *result.Usage.TotalTokens != 6 {
		t.Fatalf("usage = %+v", result.Usage)
	}
	if len(fake.Calls()) != 1 {
		t.Fatalf("calls = %d, want 1", len(fake.Calls()))
	}
}

func TestFakeProviderCancellationIsDetectable(t *testing.T) {
	fake := NewFake(FakeScript{Events: []Event{
		{Kind: EventTextDelta, Text: "one"},
		{Kind: EventTextDelta, Text: "two"},
	}})
	ctx, cancel := context.WithCancel(context.Background())
	_, err := fake.Stream(ctx, validRequest(), func(event Event) error {
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestFakeProviderDeadlineIsDetectable(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	fake := NewFake(FakeScript{})
	_, err := fake.Stream(ctx, validRequest(), func(Event) error { return nil })
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context.DeadlineExceeded", err)
	}
}

func TestFakeProviderPropagatesSinkError(t *testing.T) {
	want := errors.New("stop sink")
	fake := NewFake(FakeScript{Events: []Event{
		{Kind: EventTextDelta, Text: "one"},
		{Kind: EventTextDelta, Text: "two"},
	}})
	count := 0
	_, err := fake.Stream(context.Background(), validRequest(), func(Event) error {
		count++
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want sink error", err)
	}
	if count != 1 {
		t.Fatalf("sink calls = %d, want 1", count)
	}
}

func TestToolCallIsProviderIndependentJSON(t *testing.T) {
	call := ToolCall{ID: "call_1", Name: "observe", Arguments: json.RawMessage(`{"operation":"read_file","path":"README.md"}`)}
	if err := call.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestInvalidRoleAndSchemaRejected(t *testing.T) {
	req := validRequest()
	req.Messages[0].Role = Role("mystery")
	if err := req.Validate(); err == nil {
		t.Fatal("expected invalid role error")
	}

	req = validRequest()
	req.Tools[0].InputSchema = json.RawMessage(`{broken`)
	if err := req.Validate(); err == nil {
		t.Fatal("expected invalid schema error")
	}
}
