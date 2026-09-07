package agent

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Methamorphe/go-agent/internal/clock"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/ledger"
	"github.com/Methamorphe/go-agent/internal/mmu"
	"github.com/Methamorphe/go-agent/internal/objectstore"
	"github.com/Methamorphe/go-agent/internal/process"
	"github.com/Methamorphe/go-agent/internal/provider"
	"github.com/Methamorphe/go-agent/internal/scheduler"
	"github.com/Methamorphe/go-agent/internal/storage/sqlite"
)

type g6TestResolver map[string]provider.Provider

func (r g6TestResolver) Resolve(profile scheduler.ModelProfile) (provider.Provider, error) {
	return r[profile.Key()], nil
}

func TestG6RunnerPersistsRoutingAndPreservesAgentIdentity(t *testing.T) {
	ctx := context.Background()
	source := clock.NewFakeClock(time.Date(2026, time.September, 7, 15, 0, 0, 0, time.UTC))
	store, err := sqlite.Open(ctx, sqlite.Config{
		Path:         filepath.Join(t.TempDir(), "runtime.db"),
		BusyTimeout:  5 * time.Second,
		MaxOpenConns: 8,
		Clock:        source,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	objects, err := objectstore.New(filepath.Join(t.TempDir(), "objects"))
	if err != nil {
		t.Fatal(err)
	}
	ids := id.NewGenerator()
	processes := process.NewService(store, ids, source)
	memory, err := mmu.New(store, objects, ids, source, mmu.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	root, err := processes.CreateRoot(ctx, "Return a short final answer.", process.CommandMeta{
		RequestID:     id.RequestID("req_g6_create"),
		CorrelationID: id.CorrelationID("cor_g6_create"),
		Actor:         ledger.ActorRef{Kind: ledger.ActorSystem, ID: "g6-test"},
	})
	if err != nil {
		t.Fatal(err)
	}

	registry := scheduler.NewRegistry(4)
	profile := scheduler.ModelProfile{
		ProviderID: "local", ModelID: "coder", ContextWindow: 16_384, MaxOutputTokens: 2048,
		Capabilities: []scheduler.Capability{scheduler.CapabilityToolCalling, scheduler.CapabilityStreaming},
		Locality: scheduler.LocalityLocal, DataPolicy: scheduler.DataPolicy{AllowsSensitive: true, TrainingOptOut: true},
		InputCostMicrosPerMillion: 0, OutputCostMicrosPerMillion: 0,
		LatencyP95: time.Second, Reliability: .99,
		Quality: map[scheduler.TaskKind]float64{scheduler.TaskCodeGeneration: .9},
		MaxConcurrency: 2, ProfileVersion: 1,
	}
	if err := registry.Register(profile); err != nil {
		t.Fatal(err)
	}
	runtime := scheduler.NewRuntime(
		scheduler.NewRouter(registry, scheduler.NewTelemetry(4, source), source),
		scheduler.NewBudgetLedger(),
		scheduler.NewSlots(scheduler.SlotConfig{Global: 2, PerRoot: 1, DefaultPerProvider: 2}),
	)
	fake := provider.NewFake(provider.FakeScript{
		Events: []provider.Event{{Kind: provider.EventTextDelta, Text: "G6 scheduled response."}},
		Result: provider.Result{FinishReason: provider.FinishStop},
	})
	runnerCfg := DefaultG4RunnerConfig()
	runnerCfg.ModelMaxContext = 16_384
	runnerCfg.ReservedOutputTokens = 2048
	runner, err := NewG6(
		nil, processes, objects, ids, id.RuntimeInstanceID("runtime_g6"), memory,
		runnerCfg, runtime, g6TestResolver{"local/coder": fake}, scheduler.Resources{Tokens: 100_000},
	)
	if err != nil {
		t.Fatal(err)
	}

	result, err := runner.Run(ctx, RunRequest{AgentID: root.AgentID, Workspace: t.TempDir(), MaxSteps: 2})
	if err != nil {
		t.Fatal(err)
	}
	if result.AgentID != root.AgentID || result.FinalPreview != "G6 scheduled response." {
		t.Fatalf("result=%+v", result)
	}
	current, err := processes.Reconstruct(ctx, root.AgentID)
	if err != nil {
		t.Fatal(err)
	}
	if current.AgentID != root.AgentID || current.RootAgentID != root.RootAgentID || current.Status != process.StatusCompleted {
		t.Fatalf("identity/state changed across scheduled invocation: %+v", current)
	}

	events, err := processes.Events(ctx, root.AgentID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	want := []ledger.EventType{
		process.EventAgentCreated,
		process.EventRootIntentBound,
		process.EventAgentReadied,
		process.EventAgentActivated,
		process.EventCognitiveRoutingDecided,
		process.EventModelInvocationStarted,
		process.EventModelInvocationCompleted,
		process.EventAgentCompleted,
	}
	if len(events) != len(want) {
		t.Fatalf("events=%d want=%d: %+v", len(events), len(want), events)
	}
	for i, eventType := range want {
		if events[i].Type != eventType {
			t.Fatalf("event[%d]=%s want=%s", i, events[i].Type, eventType)
		}
	}
	if len(fake.Calls()) != 1 || fake.Calls()[0].AgentID != root.AgentID || fake.Calls()[0].Model != "coder" {
		t.Fatalf("provider calls=%+v", fake.Calls())
	}
}
