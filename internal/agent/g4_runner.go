package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Methamorphe/go-agent/internal/agentsyscall"
	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/invocation"
	"github.com/Methamorphe/go-agent/internal/ledger"
	"github.com/Methamorphe/go-agent/internal/live"
	"github.com/Methamorphe/go-agent/internal/mmu"
	"github.com/Methamorphe/go-agent/internal/objectstore"
	"github.com/Methamorphe/go-agent/internal/process"
	"github.com/Methamorphe/go-agent/internal/provider"
)

const (
	g4ContextEnvelopeReserveTokens = 4_096
	g4MaxAssistantMemoryBytes      = 8 << 10
	g4MaxToolPreviewBytes          = 4 << 10
	g4MaxQueryBytes                = 12 << 10
)

type G4IDGenerator interface {
	IDGenerator
	mmu.IDGenerator
}

type G4RunnerConfig struct {
	ModelMaxContext      int
	ReservedOutputTokens int
	ProviderOverhead     int
	SafetyMargin         int
}

func DefaultG4RunnerConfig() G4RunnerConfig {
	return G4RunnerConfig{
		ModelMaxContext:      mmu.DefaultModelMaxContext,
		ReservedOutputTokens: mmu.DefaultReservedOutputTokens,
		ProviderOverhead:     mmu.DefaultProviderOverhead,
		SafetyMargin:         mmu.DefaultSafetyMargin,
	}
}

type G4Logger interface {
	Warn(msg string, args ...any)
}

type G4Runner struct {
	logger          G4Logger
	processes       ProcessAPI
	objects         *objectstore.Store
	ids             G4IDGenerator
	runtimeID       id.RuntimeInstanceID
	live            *live.Bus
	memory          *mmu.Manager
	providerFactory ProviderFactory
	cfg             G4RunnerConfig
}

func NewG4(logger G4Logger, processes ProcessAPI, objects *objectstore.Store, ids G4IDGenerator, runtimeID id.RuntimeInstanceID, memory *mmu.Manager) *G4Runner {
	return NewG4WithProviderFactory(logger, processes, objects, ids, runtimeID, memory, DefaultG4RunnerConfig(), newProvider)
}

func NewG4WithProviderFactory(logger G4Logger, processes ProcessAPI, objects *objectstore.Store, ids G4IDGenerator, runtimeID id.RuntimeInstanceID, memory *mmu.Manager, cfg G4RunnerConfig, factory ProviderFactory) *G4Runner {
	if factory == nil {
		factory = newProvider
	}
	defaults := DefaultG4RunnerConfig()
	if cfg.ModelMaxContext <= 0 {
		cfg.ModelMaxContext = defaults.ModelMaxContext
	}
	if cfg.ReservedOutputTokens <= 0 {
		cfg.ReservedOutputTokens = defaults.ReservedOutputTokens
	}
	if cfg.ProviderOverhead <= 0 {
		cfg.ProviderOverhead = defaults.ProviderOverhead
	}
	if cfg.SafetyMargin <= 0 {
		cfg.SafetyMargin = defaults.SafetyMargin
	}
	return &G4Runner{
		logger: logger, processes: processes, objects: objects, ids: ids, runtimeID: runtimeID,
		live: live.New(live.DefaultMaxStreams, live.DefaultStreamBytes), memory: memory,
		providerFactory: factory, cfg: cfg,
	}
}

func (r *G4Runner) Live(agentID id.AgentID) (live.Snapshot, bool) { return r.live.Snapshot(agentID) }

func (r *G4Runner) Run(ctx context.Context, request RunRequest) (RunResult, error) {
	if r.memory == nil {
		return RunResult{}, errs.New(errs.CodeInternal, "agent.g4.run", "cognitive MMU is not configured")
	}
	if request.AgentID == "" {
		return RunResult{}, errs.New(errs.CodeInvalidArgument, "agent.g4.run", "agent id is required")
	}
	if strings.TrimSpace(request.Model) == "" {
		return RunResult{}, errs.New(errs.CodeInvalidArgument, "agent.g4.run", "model is required")
	}
	if strings.TrimSpace(request.Workspace) == "" {
		return RunResult{}, errs.New(errs.CodeInvalidArgument, "agent.g4.run", "workspace is required")
	}
	if request.MaxSteps <= 0 {
		request.MaxSteps = DefaultMaxSteps
	}
	if request.MaxSteps > MaximumMaxSteps {
		return RunResult{}, errs.New(errs.CodeInvalidArgument, "agent.g4.run", "max_steps exceeds maximum")
	}

	model, err := r.providerFactory(request)
	if err != nil {
		return RunResult{}, err
	}
	state, err := r.processes.Inspect(ctx, request.AgentID)
	if err != nil {
		return RunResult{}, err
	}
	if state.Status != process.StatusReady {
		return RunResult{}, errs.New(errs.CodeConflict, "agent.g4.run", "process must be READY")
	}
	if state.RootIntent == nil {
		return RunResult{}, errs.New(errs.CodeCorruption, "agent.g4.run", "process has no root intent")
	}

	correlationID, err := r.ids.Correlation()
	if err != nil {
		return RunResult{}, errs.Wrap(errs.CodeInternal, "agent.g4.run", "generate correlation id", err)
	}
	meta := process.CommandMeta{CorrelationID: correlationID, Actor: ledger.ActorRef{Kind: ledger.ActorAgent, ID: request.AgentID.String()}}
	expected := state.Version
	state, err = r.processes.Activate(ctx, request.AgentID, r.runtimeID, &expected, meta)
	if err != nil {
		return RunResult{}, err
	}
	if err := r.live.Begin(request.AgentID, time.Now().UTC()); err != nil {
		r.bestEffortYield(ctx, state, meta, "live_stream_unavailable")
		return RunResult{}, err
	}
	finished := false
	defer func() {
		if !finished {
			r.live.Fail(request.AgentID, "agent run ended before completion")
		}
	}()

	dispatcher, err := agentsyscall.New(r.processes, r.objects, r.ids, request.Workspace)
	if err != nil {
		r.bestEffortYield(ctx, state, meta, "invalid_workspace")
		return RunResult{}, err
	}
	invocations := invocation.New(r.processes, r.objects, r.ids, r.live)
	tools := append(agentsyscall.Tools(), recallToolDefinition())

	systemPage, err := r.memory.CreatePage(ctx, mmu.PageInput{AgentID: request.AgentID, Type: mmu.PageDocumentation, Scope: mmu.ScopeProcess, SourceRef: "runtime:g4-system-prompt", Content: g4SystemPrompt, Importance: 1, Confidence: 1})
	if err != nil {
		r.bestEffortYield(ctx, state, meta, "context_page_create_failed")
		return RunResult{}, err
	}
	intentPage, err := r.memory.CreatePage(ctx, mmu.PageInput{AgentID: request.AgentID, Type: mmu.PageUserMessage, Scope: mmu.ScopeRootTask, SourceRef: "agent:" + request.AgentID.String() + ":root-intent", Content: state.RootIntent.Goal, Importance: 1, Confidence: 1})
	if err != nil {
		r.bestEffortYield(ctx, state, meta, "context_page_create_failed")
		return RunResult{}, err
	}
	mandatory := []id.ContextPageID{systemPage.ID, intentPage.ID}
	allowedScopes := []mmu.Scope{mmu.ScopeProcess, mmu.ScopeRootTask, mmu.ScopeProject, mmu.ScopeWorkspace, mmu.ScopeWorld}
	var recent []provider.Message
	query := state.RootIntent.Goal
	var aggregateUsage provider.Usage
	var finalRef, finalPreview string

	for step := 1; step <= request.MaxSteps; step++ {
		messages, manifest, buildErr := r.buildWorkingMessages(ctx, request, tools, mandatory, allowedScopes, recent, query)
		if buildErr != nil {
			r.bestEffortYield(ctx, state, meta, "context_budget_impossible")
			return RunResult{}, buildErr
		}

		expected = state.Version
		outcome, nextState, invokeErr := invocations.Invoke(ctx, model, request.AgentID, request.Model, messages, tools, &expected, meta)
		state = nextState
		if outcome.InvocationID != "" {
			if _, manifestErr := r.memory.PersistManifest(context.WithoutCancel(ctx), outcome.InvocationID, manifest); manifestErr != nil {
				if invokeErr != nil {
					invokeErr = errors.Join(invokeErr, manifestErr)
				} else {
					invokeErr = manifestErr
				}
			}
		}
		if invokeErr != nil {
			r.bestEffortYield(ctx, state, meta, "model_invocation_failed")
			r.live.Fail(request.AgentID, invokeErr.Error())
			finished = true
			return RunResult{}, invokeErr
		}
		aggregateUsage = addUsage(aggregateUsage, outcome.Result.Usage)
		finalRef, finalPreview = outcome.ResponseRef, outcome.TextPreview
		if strings.TrimSpace(outcome.TextPreview) != "" {
			if _, err := r.remember(ctx, request.AgentID, mmu.PageAssistantSummary, "invocation:"+outcome.InvocationID.String(), outcome.TextPreview, 0.65); err != nil {
				return RunResult{}, err
			}
		}

		if len(outcome.Result.ToolCalls) == 0 {
			if strings.TrimSpace(outcome.TextPreview) == "" {
				r.bestEffortYield(ctx, state, meta, "empty_model_response")
				return RunResult{}, errs.New(errs.CodeUnavailable, "agent.g4.run", "model returned neither text nor tool calls")
			}
			expected = state.Version
			completed, err := r.processes.Complete(ctx, request.AgentID, &expected, outcome.ResponseRef, meta)
			if err != nil {
				return RunResult{}, err
			}
			state = completed
			r.live.Complete(request.AgentID, outcome.ResponseRef)
			finished = true
			return RunResult{AgentID: request.AgentID, Steps: step, ResponseRef: finalRef, FinalPreview: finalPreview, Usage: aggregateUsage}, nil
		}
		if len(outcome.Result.ToolCalls) > MaxToolCallsPerStep {
			r.bestEffortYield(ctx, state, meta, "too_many_tool_calls")
			return RunResult{}, errs.New(errs.CodeResourceExhausted, "agent.g4.run", "too many tool calls in one model step")
		}

		assistantParts := make([]provider.ContentPart, 0, len(outcome.Result.ToolCalls)+1)
		if outcome.TextPreview != "" {
			assistantParts = append(assistantParts, provider.TextPart(truncateUTF8(outcome.TextPreview, g4MaxAssistantMemoryBytes)))
		}
		for _, call := range outcome.Result.ToolCalls {
			assistantParts = append(assistantParts, provider.ToolCallPart(call))
		}
		recent = []provider.Message{{Role: provider.RoleAssistant, Parts: assistantParts}}
		queryParts := []string{state.RootIntent.Goal, truncateUTF8(outcome.TextPreview, g4MaxAssistantMemoryBytes)}

		for _, call := range outcome.Result.ToolCalls {
			var modelBody []byte
			if call.Name == "recall" {
				recallResult, next, callErr := r.callRecall(ctx, state, call, meta, allowedScopes)
				if callErr != nil {
					r.bestEffortYield(ctx, state, meta, "recall_recording_failed")
					return RunResult{}, callErr
				}
				state = next
				modelBody, err = json.Marshal(recallResult)
				if err == nil {
					_, err = r.remember(ctx, request.AgentID, mmu.PageRecallResult, "recall:"+recallResult.ActionID.String(), string(modelBody), 0.6)
				}
			} else {
				toolResult, next, callErr := dispatcher.Call(ctx, state, call, meta)
				if callErr != nil {
					r.bestEffortYield(ctx, state, meta, "syscall_recording_failed")
					r.live.Fail(request.AgentID, callErr.Error())
					finished = true
					return RunResult{}, callErr
				}
				state = next
				modelBody, err = json.Marshal(compactToolResult(toolResult))
				if err == nil {
					_, err = r.remember(ctx, request.AgentID, mmu.PageToolResultSummary, "tool:"+call.Name+":"+toolResult.ActionID.String(), string(modelBody), 0.8)
				}
			}
			if err != nil {
				return RunResult{}, errs.Wrap(errs.CodeInternal, "agent.g4.run", "encode durable tool result", err)
			}
			body := string(modelBody)
			recent = append(recent, provider.Message{Role: provider.RoleTool, Parts: []provider.ContentPart{provider.ToolResultPart(provider.ToolResult{CallID: call.ID, Content: body})}})
			queryParts = append(queryParts, call.Name, body)
		}
		query = truncateUTF8(strings.Join(queryParts, "\n"), g4MaxQueryBytes)
	}

	expected = state.Version
	failed, failErr := r.processes.Fail(ctx, request.AgentID, &expected, process.Failure{Code: "agent_loop_exhausted", Message: fmt.Sprintf("agent exceeded %d model steps", request.MaxSteps)}, meta)
	if failErr == nil {
		state = failed
	}
	message := fmt.Sprintf("agent exceeded %d model steps", request.MaxSteps)
	r.live.Fail(request.AgentID, message)
	finished = true
	if failErr != nil {
		return RunResult{}, errorsJoin(errs.New(errs.CodeResourceExhausted, "agent.g4.run", message), failErr)
	}
	return RunResult{}, errs.New(errs.CodeResourceExhausted, "agent.g4.run", message)
}

func (r *G4Runner) bestEffortYield(ctx context.Context, state process.State, meta process.CommandMeta, reason string) {
	if state.Status != process.StatusRunning {
		return
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	expected := state.Version
	if _, err := r.processes.Yield(cleanupCtx, state.AgentID, &expected, reason, meta); err != nil && r.logger != nil {
		r.logger.Warn("failed to yield G4 agent after interrupted run", "agent_id", state.AgentID, "error", err)
	}
}
