package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Methamorphe/go-agent/internal/clock"
	"github.com/Methamorphe/go-agent/internal/id"
	agenttx "github.com/Methamorphe/go-agent/internal/transaction"
	"github.com/Methamorphe/go-agent/internal/world"
)

func TestG7TransactionStateAndPreparedPlanSurviveReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "runtime.db")
	cfg := Config{Path: path, BusyTimeout: 5 * time.Second, MaxOpenConns: 8, Clock: clock.NewFakeClock(time.Date(2026, 9, 8, 8, 0, 0, 0, time.UTC))}
	store, err := Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 8, 8, 0, 0, 0, time.UTC)
	tx := agenttx.Transaction{
		ID:             id.TransactionID("tx_durable"),
		AgentID:        id.AgentID("agt_durable"),
		BaseCheckpoint: id.CheckpointID("chk_durable"),
		WorldID:        id.WorldID("wld_durable"),
		IsolatedWorldRef: world.BranchRef{WorldID: id.WorldID("wld_durable"), Type: world.TypeWorkspace, BaseIdentity: "base", Ref: "worktree"},
		State:        agenttx.StateCreating,
		Version:      1,
		CommitPolicy: agenttx.CommitRequireVerification,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := store.Create(ctx, tx, "TransactionCreated", nil); err != nil {
		t.Fatal(err)
	}
	opened, err := store.Transition(ctx, tx.ID, 1, []agenttx.State{agenttx.StateCreating}, agenttx.StateOpen, nil, "", "TransactionOpened", nil)
	if err != nil {
		t.Fatal(err)
	}
	ready, err := store.Transition(ctx, tx.ID, opened.Version, []agenttx.State{agenttx.StateOpen}, agenttx.StateReadyToCommit, nil, "", "VerificationAccepted", nil)
	if err != nil {
		t.Fatal(err)
	}
	plan := &world.PromotionPlan{OperationID: id.OperationID("op_durable"), BaseIdentity: "base", TargetBefore: "target", Source: "source", Merged: "merged"}
	prepared, err := store.Transition(ctx, tx.ID, ready.Version, []agenttx.State{agenttx.StateReadyToCommit}, agenttx.StateReadyToCommit, plan, "", "TransactionPrepared", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	got, err := store.Get(ctx, tx.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != agenttx.StateReadyToCommit || got.Version != prepared.Version || got.PreparedPlan == nil || got.PreparedPlan.Merged != "merged" {
		t.Fatalf("reopened transaction=%+v", got)
	}
	if _, err := store.Transition(ctx, tx.ID, ready.Version, []agenttx.State{agenttx.StateReadyToCommit}, agenttx.StateCommitting, plan, "", "stale", nil); !errors.Is(err, agenttx.ErrConflict) {
		t.Fatalf("stale transition err=%v", err)
	}
	events, err := store.TransactionEvents(ctx, tx.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 4 {
		t.Fatalf("event count=%d, want 4", len(events))
	}
}

func TestG7CommittingStateSurvivesRestartForReconciliation(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "runtime.db")
	cfg := Config{Path: path, Clock: clock.NewFakeClock(time.Date(2026, 9, 8, 8, 30, 0, 0, time.UTC))}
	store, err := Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	plan := &world.PromotionPlan{OperationID: id.OperationID("op_crash"), BaseIdentity: "base", TargetBefore: "target", Source: "source", Merged: "merged"}
	tx := agenttx.Transaction{ID: id.TransactionID("tx_crash"), AgentID: id.AgentID("agt_crash"), BaseCheckpoint: id.CheckpointID("chk_crash"), WorldID: id.WorldID("wld_crash"), IsolatedWorldRef: world.BranchRef{WorldID: id.WorldID("wld_crash"), Type: world.TypeWorkspace, BaseIdentity: "base", Ref: "worktree"}, State: agenttx.StateCommitting, Version: 7, CommitPolicy: agenttx.CommitRequireVerification, PreparedPlan: plan, CreatedAt: now, UpdatedAt: now}
	if err := store.Create(ctx, tx, "CommitStarted", nil); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	items, err := store.ListByStates(ctx, agenttx.StateCommitting, agenttx.StateNeedsReconciliation)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].State != agenttx.StateCommitting || items[0].PreparedPlan == nil {
		t.Fatalf("recovery candidates=%+v", items)
	}
}

func TestG7BreakingMultiFileChangeFailsVerificationAndRollsBackTarget(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	ctx := context.Background()
	repo := t.TempDir()
	runG7Git(t, repo, "init")
	runG7Git(t, repo, "config", "user.name", "G7 Test")
	runG7Git(t, repo, "config", "user.email", "g7@example.invalid")
	writeG7File(t, repo, "a.txt", "base-a\n")
	writeG7File(t, repo, "b.txt", "base-b\n")
	runG7Git(t, repo, "add", "-A")
	runG7Git(t, repo, "commit", "-m", "base")

	branch, err := world.NewWorkspaceWorld(ctx, world.WorkspaceConfig{Repository: repo, WorldID: id.WorldID("wld_integration")})
	if err != nil {
		t.Fatal(err)
	}
	store, err := Open(ctx, Config{Path: filepath.Join(t.TempDir(), "runtime.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	manager := agenttx.NewManager(store, nil, nil)
	tx, err := manager.Begin(ctx, agenttx.BeginRequest{AgentID: id.AgentID("agt_integration"), BaseCheckpoint: id.CheckpointID("chk_integration")}, branch)
	if err != nil {
		t.Fatal(err)
	}
	for i, file := range []string{"a.txt", "b.txt"} {
		params, _ := json.Marshal(map[string]string{"content": "broken trailing whitespace   \n"})
		action := world.Action{ID: id.ActionID("act_write_" + string(rune('a'+i))), AgentID: tx.AgentID, Kind: "fs.write_file", Purpose: "speculative edit", Resource: file, Params: params, Effect: world.CanonicalEffect("fs.write_file")}
		if _, err := manager.Execute(ctx, tx.ID, branch, action); err != nil {
			t.Fatal(err)
		}
	}
	checkParams, _ := json.Marshal(map[string]any{"executable": "git", "args": []string{"diff", "--check"}, "cwd": "."})
	verification, err := manager.Verify(ctx, tx.ID, branch, []agenttx.CheckSpec{{Name: "git diff --check", Action: world.Action{ID: id.ActionID("act_verify"), Kind: "process.exec", Purpose: "reject whitespace errors", Params: checkParams, Effect: world.CanonicalEffect("process.exec")}}})
	if !errors.Is(err, agenttx.ErrVerificationFailed) || verification.Status != agenttx.VerificationFailed {
		t.Fatalf("verification=%+v err=%v", verification, err)
	}
	rolled, err := manager.Rollback(ctx, tx.ID, branch)
	if err != nil {
		t.Fatal(err)
	}
	if rolled.State != agenttx.StateRolledBack {
		t.Fatalf("rollback state=%s", rolled.State)
	}
	if got := readG7File(t, repo, "a.txt"); got != "base-a\n" {
		t.Fatalf("target a=%q", got)
	}
	if got := readG7File(t, repo, "b.txt"); got != "base-b\n" {
		t.Fatalf("target b=%q", got)
	}
}

func runG7Git(t *testing.T, directory string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = directory
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}

func writeG7File(t *testing.T, root, path, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, path), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readG7File(t *testing.T, root, path string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, path))
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
