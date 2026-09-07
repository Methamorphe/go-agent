package agent

import (
	"fmt"

	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/invocation"
	"github.com/Methamorphe/go-agent/internal/mmu"
	"github.com/Methamorphe/go-agent/internal/objectstore"
	"github.com/Methamorphe/go-agent/internal/process"
	"github.com/Methamorphe/go-agent/internal/scheduler"
)

type g6State struct {
	scheduled  *invocation.ScheduledService
	rootBudget scheduler.Resources
}

func NewG6(
	logger G4Logger,
	processes ProcessAPI,
	objects *objectstore.Store,
	ids G4IDGenerator,
	runtimeID id.RuntimeInstanceID,
	memory *mmu.Manager,
	cfg G4RunnerConfig,
	schedulerRuntime *scheduler.Runtime,
	resolver invocation.BackendResolver,
	rootBudget scheduler.Resources,
) (*G4Runner, error) {
	if schedulerRuntime == nil || resolver == nil {
		return nil, errs.New(errs.CodeInvalidArgument, "agent.g6.new", "scheduler runtime and backend resolver are required")
	}
	if !rootBudget.Valid() || rootBudget.Tokens <= 0 {
		return nil, errs.New(errs.CodeInvalidArgument, "agent.g6.new", "positive root token budget is required")
	}

	runner := NewG4WithProviderFactory(logger, processes, objects, ids, runtimeID, memory, cfg, newProvider)
	base := invocation.New(processes, objects, ids, runner.live)
	scheduled, err := invocation.NewScheduled(base, schedulerRuntime, resolver)
	if err != nil {
		return nil, err
	}
	runner.g6 = &g6State{scheduled: scheduled, rootBudget: rootBudget}
	return runner, nil
}

func (g *g6State) ensureRootBudget(root id.AgentID) error {
	if g == nil || g.scheduled == nil {
		return errs.New(errs.CodeInternal, "agent.g6.budget", "scheduled invocation service is not configured")
	}
	snapshot := g.scheduled.Budget(root)
	if snapshot.Limit == (scheduler.Resources{}) {
		return g.scheduled.SetRootBudget(root, g.rootBudget)
	}
	if snapshot.Limit != g.rootBudget {
		return g.scheduled.SetRootBudget(root, g.rootBudget)
	}
	return nil
}

func (g *g6State) cognitiveTask(
	request RunRequest,
	state process.State,
	correlationID id.CorrelationID,
	step int,
	manifest mmu.ContextManifest,
	reservedOutput int,
) scheduler.CognitiveTask {
	budget := g.rootBudget
	if snapshot := g.scheduled.Budget(state.RootAgentID); snapshot.Available.Valid() {
		budget = snapshot.Available
	}
	return scheduler.CognitiveTask{
		ID:                   scheduler.TaskID(fmt.Sprintf("ct-%s-%02d", correlationID.String(), step)),
		AgentID:              request.AgentID,
		RootAgentID:          state.RootAgentID,
		Kind:                 scheduler.TaskCodeGeneration,
		EstimatedInputTokens: manifest.EstimatedUsed,
		ReservedOutputTokens: reservedOutput,
		Requirements: scheduler.Requirements{
			RequiredCapabilities: []scheduler.Capability{
				scheduler.CapabilityToolCalling,
				scheduler.CapabilityStreaming,
			},
		},
		Objective: scheduler.ObjectiveBalanced,
		Budget:    budget,
		Privacy:   scheduler.PrivacyStandard,
		Risk:      scheduler.RiskNormal,
	}
}
