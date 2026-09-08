package sqlite

import (
	"context"
	"testing"

	"github.com/Methamorphe/go-agent/internal/id"
	agentprocess "github.com/Methamorphe/go-agent/internal/process"
)

func TestG8RestoreAsNewTimelineNeverRewritesSourceHistory(t *testing.T) {
	ctx, store, service, _ := newProcessHarness(t)
	historical := mustCreateRoot(t, ctx, service, "req_g8_source")

	advanced, err := service.Suspend(ctx, historical.AgentID, versionPtr(historical.Version), "advance source after checkpoint", commandMeta("req_g8_suspend"))
	if err != nil { t.Fatal(err) }
	advanced, err = service.Resume(ctx, advanced.AgentID, versionPtr(advanced.Version), "continue original timeline", commandMeta("req_g8_resume"))
	if err != nil { t.Fatal(err) }
	before, err := store.Events(ctx, historical.AgentID, 0, 100)
	if err != nil { t.Fatal(err) }

	forked, err := service.ForkTimelineFromState(ctx, historical, commandMeta("req_g8_restore"))
	if err != nil { t.Fatal(err) }
	if forked.AgentID == historical.AgentID { t.Fatal("restore reused source AgentID") }
	if forked.ParentAgentID == nil || *forked.ParentAgentID != historical.AgentID { t.Fatalf("fork parent=%v want %s", forked.ParentAgentID, historical.AgentID) }
	if forked.RootAgentID != historical.RootAgentID || forked.Status != agentprocess.StatusReady { t.Fatalf("fork state=%+v", forked) }

	current, err := service.Inspect(ctx, historical.AgentID)
	if err != nil { t.Fatal(err) }
	if current.Version != advanced.Version || current.LastEventID != advanced.LastEventID { t.Fatalf("restore changed source: got v%d/%s want v%d/%s", current.Version, current.LastEventID, advanced.Version, advanced.LastEventID) }
	after, err := store.Events(ctx, historical.AgentID, 0, 100)
	if err != nil { t.Fatal(err) }
	if len(after) != len(before) { t.Fatalf("source event count changed: before=%d after=%d", len(before), len(after)) }

	forkEvents, err := store.Events(ctx, forked.AgentID, 0, 100)
	if err != nil { t.Fatal(err) }
	if len(forkEvents) != 3 { t.Fatalf("fork event count=%d want 3", len(forkEvents)) }
	if forkEvents[0].CausationID == nil || *forkEvents[0].CausationID != historical.LastEventID { t.Fatalf("fork causation=%v want checkpoint event %s", forkEvents[0].CausationID, historical.LastEventID) }
}

func TestG8ExecutionFrontierDetectsOutstandingSyscallAtReadyBoundary(t *testing.T) {
	ctx, _, service, _ := newProcessHarness(t)
	state := mustCreateRoot(t, ctx, service, "req_g8_frontier_source")
	state = mustActivate(t, ctx, service, state, "req_g8_frontier_activate")
	actionID := id.ActionID("act_g8_inflight")
	var err error
	state, err = service.SyscallRequested(ctx, state.AgentID, versionPtr(state.Version), agentprocess.SyscallRequestedPayload{ActionID: actionID, Name: "tool", ArgumentsRef: "obj://args"}, commandMeta("req_g8_syscall_start"))
	if err != nil { t.Fatal(err) }
	state, err = service.Yield(ctx, state.AgentID, versionPtr(state.Version), "orchestration boundary", commandMeta("req_g8_frontier_yield"))
	if err != nil { t.Fatal(err) }
	if state.Status != agentprocess.StatusReady { t.Fatalf("status=%s want READY", state.Status) }

	frontier, err := service.ExecutionFrontier(ctx, state.AgentID)
	if err != nil { t.Fatal(err) }
	if len(frontier.InFlightActions) != 1 || frontier.InFlightActions[0] != actionID { t.Fatalf("in-flight actions=%v", frontier.InFlightActions) }

	state = mustActivate(t, ctx, service, state, "req_g8_frontier_reactivate")
	state, err = service.SyscallCompleted(ctx, state.AgentID, versionPtr(state.Version), agentprocess.SyscallCompletedPayload{ActionID: actionID, Name: "tool", Status: "ok", ResultRef: "obj://result"}, commandMeta("req_g8_syscall_done"))
	if err != nil { t.Fatal(err) }
	state, err = service.Yield(ctx, state.AgentID, versionPtr(state.Version), "safe boundary", commandMeta("req_g8_frontier_yield2"))
	if err != nil { t.Fatal(err) }
	frontier, err = service.ExecutionFrontier(context.Background(), state.AgentID)
	if err != nil { t.Fatal(err) }
	if len(frontier.InFlightActions) != 0 { t.Fatalf("in-flight actions survived completion: %v", frontier.InFlightActions) }
	found := false
	for _, ref := range frontier.CompletedResultRefs { if ref == "obj://result" { found = true } }
	if !found { t.Fatalf("completed result missing from frontier: %v", frontier.CompletedResultRefs) }
}
