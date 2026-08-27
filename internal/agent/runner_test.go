package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/objectstore"
	"github.com/Methamorphe/go-agent/internal/process"
	"github.com/Methamorphe/go-agent/internal/provider"
)

type runnerIDs struct{ invocation, action, correlation int }

func (g *runnerIDs) Invocation() (id.InvocationID, error) {
	g.invocation++
	return id.InvocationID(fmt.Sprintf("inv_%d", g.invocation)), nil
}
func (g *runnerIDs) Action() (id.ActionID, error) {
	g.action++
	return id.ActionID(fmt.Sprintf("act_%d", g.action)), nil
}
func (g *runnerIDs) Correlation() (id.CorrelationID, error) {
	g.correlation++
	return id.CorrelationID(fmt.Sprintf("cor_%d", g.correlation)), nil
}

type runnerProcesses struct{ state process.State }

func (p *runnerProcesses) Inspect(context.Context, id.AgentID) (process.State, error) { return p.state, nil }
func (p *runnerProcesses) bump(status process.Status) process.State {
	p.state.Version++
	p.state.Status = status
	return p.state
}
func (p *runnerProcesses) Activate(context.Context, id.AgentID, id.RuntimeInstanceID, *uint64, process.CommandMeta) (process.State, error) {
	return p.bump(process.StatusRunning), nil
}
func (p *runnerProcesses) Yield(context.Context, id.AgentID, *uint64, string, process.CommandMeta) (process.State, error) {
	return p.bump(process.StatusReady), nil
}
func (p *runnerProcesses) Complete(_ context.Context, _ id.AgentID, _ *uint64, ref string, _ process.CommandMeta) (process.State, error) {
	p.state.CompletionRef = ref
	return p.bump(process.StatusCompleted), nil
}
func (p *runnerProcesses) Fail(_ context.Context, _ id.AgentID, _ *uint64, failure process.Failure, _ process.CommandMeta) (process.State, error) {
	p.state.Failure = &failure
	return p.bump(process.StatusFailed), nil
}
func (p *runnerProcesses) ModelInvocationStarted(context.Context, id.AgentID, *uint64, process.ModelInvocationStartedPayload, process.CommandMeta) (process.State, error) {
	return p.bump(process.StatusRunning), nil
}
func (p *runnerProcesses) ModelInvocationCompleted(context.Context, id.AgentID, *uint64, process.ModelInvocationCompletedPayload, process.CommandMeta) (process.State, error) {
	return p.bump(process.StatusRunning), nil
}
func (p *runnerProcesses) ModelInvocationFailed(context.Context, id.AgentID, *uint64, process.ModelInvocationFailedPayload, process.CommandMeta) (process.State, error) {
	return p.bump(process.StatusRunning), nil
}
func (p *runnerProcesses) SyscallRequested(context.Context, id.AgentID, *uint64, process.SyscallRequestedPayload, process.CommandMeta) (process.State, error) {
	return p.bump(process.StatusRunning), nil
}
func (p *runnerProcesses) SyscallCompleted(context.Context, id.AgentID, *uint64, process.SyscallCompletedPayload, process.CommandMeta) (process.State, error) {
	return p.bump(process.StatusRunning), nil
}
func (p *runnerProcesses) SyscallFailed(context.Context, id.AgentID, *uint64, process.SyscallFailedPayload, process.CommandMeta) (process.State, error) {
	return p.bump(process.StatusRunning), nil
}
func (p *runnerProcesses) Checkpoint(context.Context, id.AgentID, *uint64, process.CommandMeta) (process.State, error) {
	p.state.LastCheckpointID = id.CheckpointID("chk_test")
	return p.bump(process.StatusRunning), nil
}

func TestMinimalAgentLoopObservesExecutesAndAnswers(t *testing.T) {
	workspace := t.TempDir()
	objects, err := objectstore.New(filepath.Join(t.TempDir(), "objects"))
	if err != nil {
		t.Fatal(err)
	}

	observeArgs := json.RawMessage(`{"operation":"list_directory","path":"."}`)
	executeArgs := json.RawMessage(`{"executable":"go","args":["version"],"timeout_ms":10000}`)
	fakeModel := provider.NewFake(
		provider.FakeScript{Result: provider.Result{FinishReason: provider.FinishToolCalls, ToolCalls: []provider.ToolCall{{ID: "call_observe", Name: "observe", Arguments: observeArgs}}}},
		provider.FakeScript{Result: provider.Result{FinishReason: provider.FinishToolCalls, ToolCalls: []provider.ToolCall{{ID: "call_execute", Name: "execute", Arguments: executeArgs}}}},
		provider.FakeScript{Events: []provider.Event{{Kind: provider.EventTextDelta, Text: "Repository inspected and command executed."}}, Result: provider.Result{FinishReason: provider.FinishStop}},
	)

	processes := &runnerProcesses{state: process.State{
		AgentID: id.AgentID("agt_test"), RootAgentID: id.AgentID("agt_test"), Version: 3, Status: process.StatusReady,
		RootIntent: &process.Intent{Goal: "Inspect this repository, run a command, and answer."},
	}}
	ids := &runnerIDs{}
	runner := NewWithProviderFactory(nil, processes, objects, ids, id.RuntimeInstanceID("run_test"), func(RunRequest) (provider.Provider, error) {
		return fakeModel, nil
	})

	result, err := runner.Run(context.Background(), RunRequest{
		AgentID: id.AgentID("agt_test"), Provider: "fake", Model: "fake-model", Workspace: workspace, MaxSteps: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Steps != 3 || result.ResponseRef == "" || result.FinalPreview == "" {
		t.Fatalf("result = %+v", result)
	}
	if processes.state.Status != process.StatusCompleted {
		t.Fatalf("status = %s", processes.state.Status)
	}

	calls := fakeModel.Calls()
	if len(calls) != 3 {
		t.Fatalf("provider calls = %d, want 3", len(calls))
	}
	if !requestContainsToolResult(calls[1], "call_observe") {
		t.Fatal("second model invocation did not receive observe result")
	}
	if !requestContainsToolResult(calls[2], "call_execute") {
		t.Fatal("third model invocation did not receive execute result")
	}
}

func requestContainsToolResult(req provider.Request, callID string) bool {
	for _, message := range req.Messages {
		for _, part := range message.Parts {
			if part.ToolResult != nil && part.ToolResult.CallID == callID {
				return true
			}
		}
	}
	return false
}
