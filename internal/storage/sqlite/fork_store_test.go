package sqlite

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/Methamorphe/go-agent/internal/clock"
	cogfork "github.com/Methamorphe/go-agent/internal/fork"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/scheduler"
	"github.com/Methamorphe/go-agent/internal/world"
)

func TestG8ForkCheckpointGroupAndBranchSurviveRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "g8.db")
	fakeClock := clock.NewFakeClock(time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC))
	store, err := Open(ctx, Config{Path: path, Clock: fakeClock})
	if err != nil { t.Fatal(err) }
	now := fakeClock.Now().UTC()
	retain := now.Add(24 * time.Hour)
	cp := cogfork.Checkpoint{
		ID: id.CheckpointID("chk_g8_sqlite"), Class: cogfork.CheckpointCommittable,
		SourceAgentID: id.AgentID("agt_g8_source"), RootAgentID: id.AgentID("agt_g8_root"), LastEventID: id.EventID("evt_g8"),
		Frontier: cogfork.ExecutionFrontier{AgentID: id.AgentID("agt_g8_source"), ProcessVersion: 9, LedgerSequence: 99, CompletedResultRefs: []string{"obj://result"}, ContextRefs: []id.ContextPageID{"ctx_1"}, WorldSnapshotRef: world.BranchRef{WorldID: id.WorldID("wld_g8_base"), Type: world.TypeWorkspace, BaseIdentity: "tree-base", Ref: "/tmp/base", Metadata: json.RawMessage(`{"base":"meta"}`)}},
		Authority: cogfork.AuthoritySnapshot{Intent: world.Intent{Version: 5, Goal: "goal", AllowedDomains: []string{"fs.read"}}, Capabilities: []world.Capability{{Domain: "fs.read", Scope: "."}}, CapturedAt: now},
		ProcessStateJSON: json.RawMessage(`{"agent_id":"agt_g8_source","version":9}`), StateHash: "hash", CreatedAt: now, RetainUntil: &retain,
	}
	if err := store.PutCheckpoint(ctx, cp); err != nil { t.Fatal(err) }
	group := cogfork.Group{ID: id.ForkGroupID("fkg_g8"), CheckpointID: cp.ID, SourceAgentID: cp.SourceAgentID, State: cogfork.GroupEvaluated, WinnerForkID: id.ForkID("frk_g8"), SelectionReason: "objective winner", CreatedAt: now, UpdatedAt: now}
	if err := store.CreateGroup(ctx, group); err != nil { t.Fatal(err) }
	evaluation := cogfork.Evaluation{Metrics: cogfork.Metrics{Correctness: 1, LatencyMS: 12}, Score: 1.2, Eligible: true, Reason: "fastest", EvidenceRef: "bench://g8", EvaluatedAt: now}
	branch := cogfork.Branch{ID: id.ForkID("frk_g8"), GroupID: group.ID, AgentID: id.AgentID("agt_g8_branch"), WorldID: id.WorldID("wld_g8_branch"), WorldRef: world.BranchRef{WorldID: id.WorldID("wld_g8_branch"), Type: world.TypeWorkspace, BaseIdentity: "tree-base", Ref: "/tmp/branch"}, Authority: cp.Authority, BudgetLimit: scheduler.Resources{MoneyMicros: 1000, Tokens: 2000}, BudgetSpent: scheduler.Resources{MoneyMicros: 100, Tokens: 500}, BudgetReservation: scheduler.ReservationID("br-g8"), BudgetSettled: true, State: cogfork.BranchSelected, Evaluation: &evaluation, CognitiveOverlay: []cogfork.OverlayEntry{{Kind: cogfork.OverlayDecision, Key: "winner", Value: json.RawMessage(`{"ok":true}`), Promotable: true, ProvenanceRef: "fork://g8", CreatedAt: now}}, CreatedAt: now, UpdatedAt: now}
	if err := store.CreateBranch(ctx, branch); err != nil { t.Fatal(err) }
	if err := store.Close(); err != nil { t.Fatal(err) }

	store, err = Open(ctx, Config{Path: path, Clock: fakeClock})
	if err != nil { t.Fatal(err) }
	defer store.Close()
	gotCP, err := store.Checkpoint(ctx, cp.ID)
	if err != nil { t.Fatal(err) }
	if gotCP.Class != cp.Class || gotCP.Frontier.ProcessVersion != 9 || gotCP.Frontier.WorldSnapshotRef.BaseIdentity != "tree-base" || len(gotCP.Authority.Capabilities) != 1 || gotCP.RetainUntil == nil { t.Fatalf("checkpoint after restart=%+v", gotCP) }
	gotGroup, err := store.Group(ctx, group.ID)
	if err != nil { t.Fatal(err) }
	if gotGroup.WinnerForkID != branch.ID || gotGroup.SelectionReason != "objective winner" { t.Fatalf("group after restart=%+v", gotGroup) }
	gotBranch, err := store.Branch(ctx, branch.ID)
	if err != nil { t.Fatal(err) }
	if gotBranch.BudgetReservation != branch.BudgetReservation || !gotBranch.BudgetSettled || gotBranch.Evaluation == nil || gotBranch.Evaluation.EvidenceRef != "bench://g8" || len(gotBranch.CognitiveOverlay) != 1 { t.Fatalf("branch after restart=%+v", gotBranch) }
}
