package app

import (
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/Methamorphe/go-agent/internal/agent"
	"github.com/Methamorphe/go-agent/internal/clock"
	"github.com/Methamorphe/go-agent/internal/config"
	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/invocation"
	"github.com/Methamorphe/go-agent/internal/mmu"
	"github.com/Methamorphe/go-agent/internal/objectstore"
	"github.com/Methamorphe/go-agent/internal/provider"
	"github.com/Methamorphe/go-agent/internal/scheduler"
)

type configuredBackendResolver struct {
	cfg     config.SchedulerConfig
	client  *http.Client
	mu      sync.Mutex
	clients map[scheduler.ProviderID]provider.Provider
}

var _ invocation.BackendResolver = (*configuredBackendResolver)(nil)

func newConfiguredBackendResolver(cfg config.SchedulerConfig) *configuredBackendResolver {
	return &configuredBackendResolver{cfg: cfg, client: &http.Client{}, clients: make(map[scheduler.ProviderID]provider.Provider)}
}

func (r *configuredBackendResolver) Resolve(profile scheduler.ModelProfile) (provider.Provider, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if model, ok := r.clients[profile.ProviderID]; ok {
		return model, nil
	}
	backend, err := r.cfg.Backend(profile.ProviderID)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInvalidArgument, "app.g6.resolve", "resolve scheduler backend", err)
	}
	apiKeyEnv := strings.TrimSpace(backend.APIKeyEnv)
	if apiKeyEnv == "" {
		apiKeyEnv = "OPENAI_API_KEY"
	}
	apiKey := os.Getenv(apiKeyEnv)

	var model provider.Provider
	switch backend.Type {
	case "openai":
		model, err = provider.NewOpenAIResponses(r.client, backend.BaseURL, apiKey)
	case "openai-compatible":
		baseURL := strings.TrimSpace(backend.BaseURL)
		if baseURL == "" {
			baseURL = "http://127.0.0.1:8000/v1"
		}
		model, err = provider.NewOpenAICompatible(r.client, baseURL, apiKey)
	default:
		return nil, errs.New(errs.CodeInvalidArgument, "app.g6.resolve", "unsupported scheduler backend type")
	}
	if err != nil {
		return nil, err
	}
	r.clients[profile.ProviderID] = model
	return model, nil
}

func buildG6Scheduler(cfg config.SchedulerConfig, source clock.Clock, budgetStores ...scheduler.BudgetStore) (*scheduler.Runtime, invocation.BackendResolver, agent.G4RunnerConfig, error) {
	if err := cfg.Validate(); err != nil {
		return nil, nil, agent.G4RunnerConfig{}, err
	}
	registry := scheduler.NewRegistry(cfg.MaxProfiles)
	maxContext := 0
	for _, raw := range cfg.Profiles {
		profile := raw.ModelProfile()
		if err := registry.Register(profile); err != nil {
			return nil, nil, agent.G4RunnerConfig{}, errs.Wrap(errs.CodeInvalidArgument, "app.g6.build", "register model profile", err)
		}
		if profile.ContextWindow > maxContext {
			maxContext = profile.ContextWindow
		}
	}
	telemetry := scheduler.NewTelemetry(cfg.MaxProfiles, source)
	router := scheduler.NewRouter(registry, telemetry, source)
	slots := scheduler.NewSlots(scheduler.SlotConfig{Global: cfg.GlobalSlots, PerRoot: cfg.PerRootSlots, DefaultPerProvider: cfg.PerProviderSlots})
	var budgets scheduler.BudgetStore
	if len(budgetStores) > 0 {
		budgets = budgetStores[0]
	}
	if budgets == nil {
		budgets = scheduler.NewBudgetLedger()
	}
	runtime := scheduler.NewRuntime(router, budgets, slots)
	runnerCfg := agent.DefaultG4RunnerConfig()
	runnerCfg.ModelMaxContext = maxContext
	runnerCfg.ReservedOutputTokens = cfg.ReservedOutputTokens
	return runtime, newConfiguredBackendResolver(cfg), runnerCfg, nil
}

func buildAgentRunner(
	logger agent.G4Logger,
	cfg config.Config,
	processes agent.ProcessAPI,
	objects *objectstore.Store,
	ids agent.G4IDGenerator,
	runtimeID id.RuntimeInstanceID,
	memory *mmu.Manager,
	source clock.Clock,
	budgets scheduler.BudgetStore,
) (*agent.G4Runner, error) {
	if !cfg.Scheduler.Enabled {
		return agent.NewG4(logger, processes, objects, ids, runtimeID, memory), nil
	}
	runtime, resolver, runnerCfg, err := buildG6Scheduler(cfg.Scheduler, source, budgets)
	if err != nil {
		return nil, err
	}
	return agent.NewG6(logger, processes, objects, ids, runtimeID, memory, runnerCfg, runtime, resolver, cfg.Scheduler.DefaultRootBudget)
}
