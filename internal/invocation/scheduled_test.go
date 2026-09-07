package invocation

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/objectstore"
	"github.com/Methamorphe/go-agent/internal/process"
	"github.com/Methamorphe/go-agent/internal/provider"
	"github.com/Methamorphe/go-agent/internal/scheduler"
)

type scheduledIDs struct{ next int }

func (g *scheduledIDs) Invocation() (id.InvocationID, error) {
	g.next++
	return id.InvocationID(fmt.Sprintf("inv_%d", g.next)), nil
}

type scheduledRecorder struct {
	state  process.State
	events []string
	routes []process.CognitiveRoutingDecidedPayload
}

func (r *scheduledRecorder) bump(event string) process.State {
	r.events = append(r.events, event)
	r.state.Version++
	r.state.Status = process.StatusRunning
	return r.state
}

func (r *scheduledRecorder) CognitiveRoutingDecided(_ context.Context, _ id.AgentID, _ *uint64, payload process.CognitiveRoutingDecidedPayload, _ process.CommandMeta) (process.State, error) {
	r.routes = append(r.routes, payload)
	return r.bump("route:" + payload.Provider + "/" + payload.Model), nil
}

func (r *scheduledRecorder) ModelInvocationStarted(_ context.Context, _ id.AgentID, _ *uint64, payload process.ModelInvocationStartedPayload, _ process.CommandMeta) (process.State, error) {
	return r.bump("start:" + payload.Provider + "/" + payload.Model), nil
}

func (r *scheduledRecorder) ModelInvocationCompleted(_ context.Context, _ id.AgentID, _ *uint64, payload process.ModelInvocationCompletedPayload, _ process.CommandMeta) (process.State, error) {
	return r.bump("complete:" + payload.InvocationID.String()), nil
}

func (r *scheduledRecorder) ModelInvocationFailed(_ context.Context, _ id.AgentID, _ *uint64, payload process.ModelInvocationFailedPayload, _ process.CommandMeta) (process.State, error) {
	return r.bump("failed:" + payload.InvocationID.String()), nil
}

type scheduledResolver map[string]provider.Provider

func (r scheduledResolver) Resolve(profile scheduler.ModelProfile) (provider.Provider, error) {
	model := r[profile.Key()]
	if model == nil {
		return nil, fmt.Errorf("backend %s not found", profile.Key())
	}
	return model, nil
}

func TestScheduledInvokeFallsBackAndSettlesExactly(t *testing.T) {
	objects, err := objectstore.New(filepath.Join(t.TempDir(), "objects"))
	if err != nil {
		t.Fatal(err)
	}
	recorder := &scheduledRecorder{state: process.State{AgentID: id.AgentID("agt_child"), RootAgentID: id.AgentID("agt_root"), Status: process.StatusRunning}}
	base := New(recorder, objects, &scheduledIDs{}, nil)

	registry := scheduler.NewRegistry(8)
	profiles := []scheduler.ModelProfile{
		{
			ProviderID: "a", ModelID: "primary", ContextWindow: 8192, MaxOutputTokens: 1024,
			Capabilities: []scheduler.Capability{scheduler.CapabilityStreaming}, Locality: scheduler.LocalityCloud,
			InputCostMicrosPerMillion: 1_000_000, OutputCostMicrosPerMillion: 1_000_000,
			LatencyP95: 1, Reliability: .99, Quality: map[scheduler.TaskKind]float64{scheduler.TaskCodeGeneration: .95},
			MaxConcurrency: 2, ProfileVersion: 1,
		},
		{
			ProviderID: "b", ModelID: "fallback", ContextWindow: 8192, MaxOutputTokens: 1024,
			Capabilities: []scheduler.Capability{scheduler.CapabilityStreaming}, Locality: scheduler.LocalityCloud,
			InputCostMicrosPerMillion: 1_000_000, OutputCostMicrosPerMillion: 1_000_000,
			LatencyP95: 1, Reliability: .98, Quality: map[scheduler.TaskKind]float64{scheduler.TaskCodeGeneration: .80},
			MaxConcurrency: 2, ProfileVersion: 1,
		},
	}
	for _, profile := range profiles {
		if err := registry.Register(profile); err != nil {
			t.Fatal(err)
		}
	}
	runtime := scheduler.NewRuntime(scheduler.NewRouter(registry, nil, nil), scheduler.NewBudgetLedger(), scheduler.NewSlots(scheduler.SlotConfig{Global: 4, PerRoot: 2, DefaultPerProvider: 2}))
	primary := provider.NewFake(provider.FakeScript{Err: errs.New(errs.CodeUnavailable, "test.primary", "transient outage")})
	inputTokens, outputTokens, totalTokens := int64(10), int64(5), int64(15)
	fallback := provider.NewFake(provider.FakeScript{
		Events: []provider.Event{{Kind: provider.EventTextDelta, Text: "fallback succeeded"}},
		Result: provider.Result{FinishReason: provider.FinishStop, Usage: provider.Usage{InputTokens: &inputTokens, OutputTokens: &outputTokens, TotalTokens: &totalTokens}},
	})
	scheduled, err := NewScheduled(base, runtime, scheduledResolver{"a/primary": primary, "b/fallback": fallback})
	if err != nil {
		t.Fatal(err)
	}
	rootBudget := scheduler.Resources{MoneyMicros: 100_000, Tokens: 10_000}
	if err := scheduled.SetRootBudget(id.AgentID("agt_root"), rootBudget); err != nil {
		t.Fatal(err)
	}

	task := scheduler.CognitiveTask{
		ID: "ct_test", AgentID: "agt_child", RootAgentID: "agt_root", Kind: scheduler.TaskCodeGeneration,
		EstimatedInputTokens: 100, ReservedOutputTokens: 50,
		Requirements: scheduler.Requirements{RequiredCapabilities: []scheduler.Capability{scheduler.CapabilityStreaming}},
		Objective: scheduler.ObjectiveQualityFirst, Budget: rootBudget, Privacy: scheduler.PrivacyStandard, Risk: scheduler.RiskNormal,
	}
	outcome, _, decision, err := scheduled.Invoke(context.Background(), task, []provider.Message{{Role: provider.RoleUser, Parts: []provider.ContentPart{provider.TextPart("test")}}}, nil, nil, process.CommandMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.TextPreview != "fallback succeeded" || decision.Selected.Key() != "b/fallback" {
		t.Fatalf("outcome=%+v decision=%+v", outcome, decision)
	}
	wantEvents := []string{
		"route:a/primary", "start:fake/primary", "failed:inv_1",
		"route:b/fallback", "start:fake/fallback", "complete:inv_2",
	}
	if !reflect.DeepEqual(recorder.events, wantEvents) {
		t.Fatalf("events=%v want=%v", recorder.events, wantEvents)
	}
	if len(recorder.routes) != 2 || recorder.routes[0].DecisionRef == "" || recorder.routes[1].DecisionRef == "" {
		t.Fatalf("routing decisions were not durably referenced: %+v", recorder.routes)
	}
	budget := scheduled.Budget(id.AgentID("agt_root"))
	if budget.Reserved != (scheduler.Resources{}) {
		t.Fatalf("reservation leaked: %+v", budget)
	}
	if budget.Spent != (scheduler.Resources{MoneyMicros: 15, Tokens: 15}) {
		t.Fatalf("spent=%+v want money=15 tokens=15", budget.Spent)
	}
	metrics := scheduled.Metrics()
	if metrics.FallbackRoutes == 0 || metrics.Completed != 1 || metrics.Failed != 1 {
		t.Fatalf("metrics=%+v", metrics)
	}
}
