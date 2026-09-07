package app

import (
	"testing"

	"github.com/Methamorphe/go-agent/internal/clock"
	"github.com/Methamorphe/go-agent/internal/config"
	"github.com/Methamorphe/go-agent/internal/scheduler"
)

func TestBuildG6SchedulerFromDaemonConfig(t *testing.T) {
	t.Setenv("TEST_OPENAI_KEY", "test-key")

	cfg := config.DefaultSchedulerConfig()
	cfg.Enabled = true
	cfg.MaxProfiles = 8
	cfg.GlobalSlots = 4
	cfg.PerRootSlots = 2
	cfg.PerProviderSlots = 2
	cfg.ReservedOutputTokens = 2048
	cfg.DefaultRootBudget = scheduler.Resources{MoneyMicros: 2_000_000, Tokens: 250_000}
	cfg.Backends = []config.SchedulerBackendConfig{
		{ProviderID: "cloud", Type: "openai", APIKeyEnv: "TEST_OPENAI_KEY"},
		{ProviderID: "local", Type: "openai-compatible", BaseURL: "http://127.0.0.1:18000/v1"},
	}
	cfg.Profiles = []config.SchedulerProfileConfig{
		{
			ProviderID: "cloud", ModelID: "frontier", ContextWindow: 128_000, MaxOutputTokens: 8192,
			Capabilities: []scheduler.Capability{scheduler.CapabilityToolCalling, scheduler.CapabilityStreaming},
			Locality: scheduler.LocalityCloud, AllowsSensitive: true,
			InputCostMicrosPerMillion: 2_000_000, OutputCostMicrosPerMillion: 8_000_000,
			LatencyP95MS: 2500, Reliability: .995, Quality: map[scheduler.TaskKind]float64{scheduler.TaskCodeGeneration: .95},
			MaxConcurrency: 2, ProfileVersion: 1,
		},
		{
			ProviderID: "local", ModelID: "coder-local", ContextWindow: 32_768, MaxOutputTokens: 4096,
			Capabilities: []scheduler.Capability{scheduler.CapabilityToolCalling, scheduler.CapabilityStreaming},
			Locality: scheduler.LocalityLocal, AllowsSensitive: true,
			InputCostMicrosPerMillion: 0, OutputCostMicrosPerMillion: 0,
			LatencyP95MS: 1500, Reliability: .98, Quality: map[scheduler.TaskKind]float64{scheduler.TaskCodeGeneration: .78},
			MaxConcurrency: 1, ProfileVersion: 1,
		},
	}

	runtime, resolver, runnerCfg, err := buildG6Scheduler(cfg, clock.NewSystemClock())
	if err != nil {
		t.Fatal(err)
	}
	if got := len(runtime.Router.Registry().Snapshot()); got != 2 {
		t.Fatalf("profiles=%d want=2", got)
	}
	if runnerCfg.ModelMaxContext != 128_000 || runnerCfg.ReservedOutputTokens != 2048 {
		t.Fatalf("runner cfg=%+v", runnerCfg)
	}
	for _, profile := range runtime.Router.Registry().Snapshot() {
		backend, err := resolver.Resolve(profile)
		if err != nil {
			t.Fatalf("resolve %s: %v", profile.Key(), err)
		}
		if backend == nil || backend.Name() == "" {
			t.Fatalf("empty backend for %s", profile.Key())
		}
	}
}

func TestSchedulerConfigRejectsUnknownBackend(t *testing.T) {
	cfg := config.DefaultSchedulerConfig()
	cfg.Enabled = true
	cfg.DefaultRootBudget = scheduler.Resources{Tokens: 100_000}
	cfg.Backends = []config.SchedulerBackendConfig{{ProviderID: "known", Type: "openai-compatible"}}
	cfg.Profiles = []config.SchedulerProfileConfig{{
		ProviderID: "missing", ModelID: "model", ContextWindow: 8192, MaxOutputTokens: 4096,
		Capabilities: []scheduler.Capability{scheduler.CapabilityToolCalling, scheduler.CapabilityStreaming},
		Locality: scheduler.LocalityCloud, InputCostMicrosPerMillion: 0, OutputCostMicrosPerMillion: 0,
		Reliability: .99, Quality: map[scheduler.TaskKind]float64{scheduler.TaskCodeGeneration: .8}, MaxConcurrency: 1, ProfileVersion: 1,
	}}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected scheduler config validation failure")
	}
}
