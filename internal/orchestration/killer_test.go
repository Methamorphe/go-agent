package orchestration_test

import (
	"context"
	"testing"
	"time"

	"github.com/Methamorphe/go-agent/internal/clock"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/orchestration"
	"github.com/Methamorphe/go-agent/internal/process"
	"github.com/Methamorphe/go-agent/internal/storage/sqlite"
)

func TestG5KillerThreeInvestigatorsSurviveRestartAndReturnStructuredEvidence(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	clk := clock.NewFakeClock(time.Date(2026, 8, 28, 22, 0, 0, 0, time.UTC))
	ids := id.NewGenerator()
	store, err := sqlite.Open(ctx, sqlite.Config{Path: dir + "/runtime.db", Clock: clk})
	if err != nil { t.Fatal(err) }
	processes := process.NewService(store, ids, clk)
	root, err := processes.CreateRoot(ctx, "investigate repository", process.CommandMeta{})
	if err != nil { t.Fatal(err) }
	service, err := orchestration.New(store, processes, ids, clk)
	if err != nil { t.Fatal(err) }
	if err := service.BootstrapRoot(ctx, root.AgentID, defaultPolicy()); err != nil { t.Fatal(err) }

	children := make([]orchestration.SpawnOutcome, 0, 3)
	for _, task := range []string{"inspect architecture", "inspect tests", "inspect persistence"} {
		outcome, err := service.Spawn(ctx, orchestration.SpawnSpec{
			ParentAgentID: root.AgentID,
			TaskIntent: task,
			Authority: []orchestration.CapabilityGrant{{Domain:"filesystem.read",Scope:"workspace/src"}},
			Budget: orchestration.Budget{Tokens:2000,ToolCalls:20,StorageBytes:4096,WorldMillis:2000,MoneyMicros:2000},
			Result: orchestration.ResultContract{EvidenceRequired:true,MaxInlineBytes:1024},
		})
		if err != nil { t.Fatal(err) }
		children = append(children, outcome)
	}

	expected := root.Version
	if _, err := processes.Activate(ctx, root.AgentID, id.RuntimeInstanceID("run_killer"), &expected, process.CommandMeta{}); err != nil { t.Fatal(err) }
	childIDs := []id.AgentID{children[0].Child.AgentID, children[1].Child.AgentID, children[2].Child.AgentID}
	wait, _, err := service.WaitFor(ctx, root.AgentID, childIDs, orchestration.WaitAll, 0, orchestration.FailureCollectAll)
	if err != nil { t.Fatal(err) }

	if err := store.Close(); err != nil { t.Fatal(err) }
	store, err = sqlite.Open(ctx, sqlite.Config{Path: dir + "/runtime.db", Clock: clk})
	if err != nil { t.Fatal(err) }
	defer store.Close()
	processes = process.NewService(store, ids, clk)
	service, err = orchestration.New(store, processes, ids, clk)
	if err != nil { t.Fatal(err) }

	persisted, err := store.WaitsForChild(ctx, children[1].Child.AgentID)
	if err != nil { t.Fatal(err) }
	if len(persisted) != 1 || persisted[0].ID != wait.ID { t.Fatalf("durable wait lost after restart: %+v", persisted) }

	for index, child := range children {
		state, err := processes.Inspect(ctx, child.Child.AgentID)
		if err != nil { t.Fatal(err) }
		expected := state.Version
		running, err := processes.Activate(ctx, state.AgentID, id.RuntimeInstanceID("run_child"), &expected, process.CommandMeta{})
		if err != nil { t.Fatal(err) }
		expected = running.Version
		if _, err := processes.Complete(ctx, state.AgentID, &expected, "object://child-result", process.CommandMeta{}); err != nil { t.Fatal(err) }
		if _, err := service.SettleAndNotify(ctx, child.Spawn.ID, orchestration.Budget{Tokens:500,ToolCalls:2}, orchestration.ChildResult{
			Summary: "bounded investigator report",
			ArtifactRef: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			EvidenceRefs: []string{"object://evidence/" + string(rune('a'+index))},
		}); err != nil { t.Fatal(err) }
	}

	rootState, err := processes.Inspect(ctx, root.AgentID)
	if err != nil { t.Fatal(err) }
	if rootState.Status != process.StatusReady { t.Fatalf("parent was not woken after all children: %s", rootState.Status) }
	for _, child := range children {
		result, ok, err := store.Result(ctx, child.Child.AgentID)
		if err != nil || !ok { t.Fatalf("missing structured result for %s: ok=%v err=%v", child.Child.AgentID, ok, err) }
		if len(result.Summary) == 0 || len(result.EvidenceRefs) == 0 || result.ArtifactRef == "" { t.Fatalf("unstructured child result: %+v", result) }
	}
}
