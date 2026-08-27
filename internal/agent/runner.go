package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Methamorphe/go-agent/internal/agentsyscall"
	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/invocation"
	"github.com/Methamorphe/go-agent/internal/ledger"
	"github.com/Methamorphe/go-agent/internal/live"
	"github.com/Methamorphe/go-agent/internal/objectstore"
	"github.com/Methamorphe/go-agent/internal/process"
	"github.com/Methamorphe/go-agent/internal/provider"
)

const (
	DefaultMaxSteps        = 24
	MaximumMaxSteps        = 64
	MaxToolCallsPerStep    = 8
	MaxWorkingContextBytes = 2 << 20
)

type ProcessAPI interface {
	Inspect(context.Context, id.AgentID) (process.State, error)
	Activate(context.Context, id.AgentID, id.RuntimeInstanceID, *uint64, process.CommandMeta) (process.State, error)
	Yield(context.Context, id.AgentID, *uint64, string, process.CommandMeta) (process.State, error)
	Complete(context.Context, id.AgentID, *uint64, string, process.CommandMeta) (process.State, error)
	Fail(context.Context, id.AgentID, *uint64, process.Failure, process.CommandMeta) (process.State, error)
	invocation.ProcessRecorder
	agentsyscall.ProcessAPI
}

type IDGenerator interface {
	Invocation() (id.InvocationID, error)
	Action() (id.ActionID, error)
	Correlation() (id.CorrelationID, error)
}

type RunRequest struct {
	AgentID   id.AgentID `json:"agent_id"`
	Provider  string     `json:"provider"`
	Model     string     `json:"model"`
	BaseURL   string     `json:"base_url,omitempty"`
	APIKeyEnv string     `json:"api_key_env,omitempty"`
	Workspace string     `json:"workspace"`
	MaxSteps  int        `json:"max_steps,omitempty"`
}

type RunResult struct {
	AgentID      id.AgentID     `json:"agent_id"`
	Steps        int            `json:"steps"`
	ResponseRef  string         `json:"response_ref"`
	FinalPreview string         `json:"final_preview"`
	Usage        provider.Usage `json:"usage"`
}

type ProviderFactory func(RunRequest) (provider.Provider, error)

type Runner struct {
	logger          *slog.Logger
	processes       ProcessAPI
	objects         *objectstore.Store
	ids             IDGenerator
	runtimeID       id.RuntimeInstanceID
	live            *live.Bus
	providerFactory ProviderFactory
}

func New(logger *slog.Logger, processes ProcessAPI, objects *objectstore.Store, ids IDGenerator, runtimeID id.RuntimeInstanceID) *Runner {
	return NewWithProviderFactory(logger, processes, objects, ids, runtimeID, newProvider)
}

func NewWithProviderFactory(logger *slog.Logger, processes ProcessAPI, objects *objectstore.Store, ids IDGenerator, runtimeID id.RuntimeInstanceID, factory ProviderFactory) *Runner {
	if factory == nil {
		factory = newProvider
	}
	return &Runner{
		logger:          logger,
		processes:       processes,
		objects:         objects,
		ids:             ids,
		runtimeID:       runtimeID,
		live:            live.New(live.DefaultMaxStreams, live.DefaultStreamBytes),
		providerFactory: factory,
	}
}

func (r *Runner) Live(agentID id.AgentID) (live.Snapshot, bool) {
	return r.live.Snapshot(agentID)
}

func (r *Runner) Run(ctx context.Context, request RunRequest) (RunResult, error) {
	if request.AgentID == "" {
		return RunResult{}, errs.New(errs.CodeInvalidArgument, "agent.run", "agent id is required")
	}
	if strings.TrimSpace(request.Model) == "" {
		return RunResult{}, errs.New(errs.CodeInvalidArgument, "agent.run", "model is required")
	}
	if strings.TrimSpace(request.Workspace) == "" {
		return RunResult{}, errs.New(errs.CodeInvalidArgument, "agent.run", "workspace is required")
	}
	if request.MaxSteps <= 0 {
		request.MaxSteps = DefaultMaxSteps
	}
	if request.MaxSteps > MaximumMaxSteps {
		return RunResult{}, errs.New(errs.CodeInvalidArgument, "agent.run", "max_steps exceeds maximum")
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
		return RunResult{}, errs.New(errs.CodeConflict, "agent.run", "process must be READY")
	}
	if state.RootIntent == nil {
		return RunResult{}, errs.New(errs.CodeCorruption, "agent.run", "process has no root intent")
	}

	correlationID, err := r.ids.Correlation()
	if err != nil {
		return RunResult{}, errs.Wrap(errs.CodeInternal, "agent.run", "generate correlation id", err)
	}
	meta := process.CommandMeta{
		CorrelationID: correlationID,
		Actor:         ledger.ActorRef{Kind: ledger.ActorAgent, ID: request.AgentID.String()},
	}

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

	messages := []provider.Message{
		{Role: provider.RoleSystem, Parts: []provider.ContentPart{provider.TextPart(systemPrompt)}},
		{Role: provider.RoleUser, Parts: []provider.ContentPart{provider.TextPart(state.RootIntent.Goal)}},
	}

	var aggregateUsage provider.Usage
	var finalRef string
	var finalPreview string

	for step := 1; step <= request.MaxSteps; step++ {
		if workingContextBytes(messages) > MaxWorkingContextBytes {
			r.bestEffortYield(ctx, state, meta, "working_context_limit")
			return RunResult{}, errs.New(errs.CodeResourceExhausted, "agent.run", "G2 working context limit reached; G4 Cognitive MMU is required for longer runs")
		}

		expected = state.Version
		outcome, nextState, invokeErr := invocations.Invoke(ctx, model, request.AgentID, request.Model, messages, agentsyscall.Tools(), &expected, meta)
		state = nextState
		if invokeErr != nil {
			r.bestEffortYield(ctx, state, meta, "model_invocation_failed")
			r.live.Fail(request.AgentID, invokeErr.Error())
			finished = true
			return RunResult{}, invokeErr
		}

		aggregateUsage = addUsage(aggregateUsage, outcome.Result.Usage)
		finalRef = outcome.ResponseRef
		finalPreview = outcome.TextPreview

		if len(outcome.Result.ToolCalls) == 0 {
			if strings.TrimSpace(outcome.TextPreview) == "" {
				r.bestEffortYield(ctx, state, meta, "empty_model_response")
				return RunResult{}, errs.New(errs.CodeUnavailable, "agent.run", "model returned neither text nor tool calls")
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
			return RunResult{}, errs.New(errs.CodeResourceExhausted, "agent.run", "too many tool calls in one model step")
		}

		assistantParts := make([]provider.ContentPart, 0, len(outcome.Result.ToolCalls)+1)
		if outcome.TextPreview != "" {
			assistantParts = append(assistantParts, provider.TextPart(outcome.TextPreview))
		}
		for _, call := range outcome.Result.ToolCalls {
			assistantParts = append(assistantParts, provider.ToolCallPart(call))
		}
		messages = append(messages, provider.Message{Role: provider.RoleAssistant, Parts: assistantParts})

		for _, call := range outcome.Result.ToolCalls {
			toolResult, next, callErr := dispatcher.Call(ctx, state, call, meta)
			if callErr != nil {
				r.bestEffortYield(ctx, state, meta, "syscall_recording_failed")
				r.live.Fail(request.AgentID, callErr.Error())
				finished = true
				return RunResult{}, callErr
			}
			state = next
			body, err := json.Marshal(toolResult)
			if err != nil {
				return RunResult{}, errs.Wrap(errs.CodeInternal, "agent.run", "encode tool result for model", err)
			}
			messages = append(messages, provider.Message{
				Role: provider.RoleTool,
				Parts: []provider.ContentPart{provider.ToolResultPart(provider.ToolResult{CallID: call.ID, Content: string(body)})},
			})
		}
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
		return RunResult{}, errorsJoin(errs.New(errs.CodeResourceExhausted, "agent.run", message), failErr)
	}
	return RunResult{}, errs.New(errs.CodeResourceExhausted, "agent.run", message)
}

func (r *Runner) bestEffortYield(ctx context.Context, state process.State, meta process.CommandMeta, reason string) {
	if state.Status != process.StatusRunning {
		return
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	expected := state.Version
	if _, err := r.processes.Yield(cleanupCtx, state.AgentID, &expected, reason, meta); err != nil && r.logger != nil {
		r.logger.Warn("failed to yield agent after interrupted run", "agent_id", state.AgentID, "error", err)
	}
}

func newProvider(request RunRequest) (provider.Provider, error) {
	apiKeyEnv := request.APIKeyEnv
	if apiKeyEnv == "" {
		apiKeyEnv = "OPENAI_API_KEY"
	}
	apiKey := os.Getenv(apiKeyEnv)
	client := &http.Client{}

	switch request.Provider {
	case "openai":
		return provider.NewOpenAIResponses(client, request.BaseURL, apiKey)
	case "openai-compatible":
		baseURL := request.BaseURL
		if baseURL == "" {
			baseURL = "http://127.0.0.1:8000/v1"
		}
		return provider.NewOpenAICompatible(client, baseURL, apiKey)
	default:
		return nil, errs.New(errs.CodeInvalidArgument, "agent.provider", "provider must be openai or openai-compatible")
	}
}

func workingContextBytes(messages []provider.Message) int {
	total := 0
	for _, message := range messages {
		for _, part := range message.Parts {
			total += len(part.Text)
			if part.ToolCall != nil {
				total += len(part.ToolCall.ID) + len(part.ToolCall.Name) + len(part.ToolCall.Arguments)
			}
			if part.ToolResult != nil {
				total += len(part.ToolResult.CallID) + len(part.ToolResult.Content)
			}
		}
	}
	return total
}

func addUsage(left, right provider.Usage) provider.Usage {
	return provider.Usage{
		InputTokens:  addOptional(left.InputTokens, right.InputTokens),
		OutputTokens: addOptional(left.OutputTokens, right.OutputTokens),
		TotalTokens:  addOptional(left.TotalTokens, right.TotalTokens),
	}
}

func addOptional(left, right *int64) *int64 {
	if left == nil && right == nil {
		return nil
	}
	var total int64
	if left != nil {
		total += *left
	}
	if right != nil {
		total += *right
	}
	return &total
}

func errorsJoin(left, right error) error {
	if left == nil {
		return right
	}
	if right == nil {
		return left
	}
	return fmt.Errorf("%v; additionally: %w", left, right)
}

const systemPrompt = `You are the minimal G2 developer agent running inside a durable Agent Process.
Use the provided syscalls when you need evidence from the workspace.
- observe reads files or lists directories.
- execute runs an executable with argv; do not assume shell expansion.
- checkpoint creates a durable process checkpoint marker.
Do not claim that a file, command, test, or repository fact was inspected unless a syscall returned evidence for it.
When the task is complete, return the final answer as normal assistant text without a tool call.`
