package app

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	controlapi "github.com/Methamorphe/go-agent/internal/control/api"
	"github.com/Methamorphe/go-agent/internal/id"
	agentprocess "github.com/Methamorphe/go-agent/internal/process"
	"github.com/Methamorphe/go-agent/internal/transaction"
	"github.com/Methamorphe/go-agent/internal/world"
)

type g13ProcessPolicyFixture struct {
	state agentprocess.State
}

func (f *g13ProcessPolicyFixture) Inspect(context.Context, id.AgentID) (agentprocess.State, error) {
	return f.state, nil
}

func TestG13TransactionOperatorVerifyPrepareCommit(t *testing.T) {
	ctx := context.Background()
	repo := initG13OperatorRepo(t)
	ids := id.NewGenerator()
	worldID, err := ids.World()
	if err != nil {
		t.Fatal(err)
	}
	branch, err := world.NewWorkspaceWorld(ctx, world.WorkspaceConfig{Repository: repo, WorldID: worldID})
	if err != nil {
		t.Fatal(err)
	}
	committed := false
	t.Cleanup(func() {
		if !committed {
			_ = branch.Rollback(context.Background())
		}
	})

	agentID := id.AgentID("agt_g13_operator")
	now := time.Date(2026, 9, 8, 14, 0, 0, 0, time.UTC)
	policy := &g13ProcessPolicyFixture{state: agentprocess.State{
		AgentID: agentID,
		RootAgentID: agentID,
		Status: agentprocess.StatusReady,
		Version: 5,
		RootIntent: &agentprocess.Intent{ID: id.IntentID("int_g13_operator"), SchemaVersion: agentprocess.IntentSchemaVersion, Goal: "ship verified workspace changes", CreatedAt: now},
	}}
	store := transaction.NewMemoryStore()
	op := newG13TransactionOperator(store, policy, ids, func() time.Time { return now })
	tx, err := op.manager.Begin(ctx, transaction.BeginRequest{AgentID: agentID, BaseCheckpoint: id.CheckpointID("chk_g13_operator")}, branch)
	if err != nil {
		t.Fatal(err)
	}

	params, _ := json.Marshal(map[string]string{"content": "updated\n"})
	actionID, err := ids.Action()
	if err != nil {
		t.Fatal(err)
	}
	result, err := op.manager.Execute(ctx, tx.ID, branch, world.Action{
		ID: actionID,
		AgentID: agentID,
		Kind: "fs.write_file",
		Purpose: "exercise verified G13 promotion",
		Resource: "value.txt",
		Params: params,
		Effect: world.CanonicalEffect("fs.write_file"),
	})
	if err != nil || result.Status != world.ResultSucceeded {
		t.Fatalf("write result=%+v err=%v", result, err)
	}
	if got := readG13OperatorFile(t, repo, "value.txt"); got != "base\n" {
		t.Fatalf("speculation leaked before commit: %q", got)
	}

	verified, err := op.Operate(ctx, controlapi.WorkspaceTransactionOperateRequest{
		TransactionID: tx.ID,
		Operation: controlapi.WorkspaceTransactionVerify,
		Command: []string{"git", "status", "--porcelain"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if verified.Verification == nil || verified.Verification.Status != transaction.VerificationPassed || verified.Transaction.State != transaction.StateReadyToCommit {
		t.Fatalf("unexpected verification response: %+v", verified)
	}

	prepared, err := op.Operate(ctx, controlapi.WorkspaceTransactionOperateRequest{TransactionID: tx.ID, Operation: controlapi.WorkspaceTransactionPrepare})
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Promotion == nil || prepared.Promotion.Merged == "" {
		t.Fatal("prepare did not return a durable promotion plan")
	}

	finished, err := op.Operate(ctx, controlapi.WorkspaceTransactionOperateRequest{TransactionID: tx.ID, Operation: controlapi.WorkspaceTransactionCommit})
	if err != nil {
		t.Fatal(err)
	}
	committed = true
	if finished.Transaction.State != transaction.StateCommitted {
		t.Fatalf("state=%s", finished.Transaction.State)
	}
	if got := readG13OperatorFile(t, repo, "value.txt"); got != "updated\n" {
		t.Fatalf("target=%q", got)
	}
}

func TestG13TransactionOperatorCommitGuardFailsClosedAfterCancellation(t *testing.T) {
	ctx := context.Background()
	repo := initG13OperatorRepo(t)
	ids := id.NewGenerator()
	worldID, _ := ids.World()
	branch, err := world.NewWorkspaceWorld(ctx, world.WorkspaceConfig{Repository: repo, WorldID: worldID})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = branch.Rollback(context.Background()) })

	agentID := id.AgentID("agt_g13_cancelled")
	now := time.Date(2026, 9, 8, 14, 0, 0, 0, time.UTC)
	policy := &g13ProcessPolicyFixture{state: agentprocess.State{
		AgentID: agentID,
		RootAgentID: agentID,
		Status: agentprocess.StatusReady,
		Version: 2,
		RootIntent: &agentprocess.Intent{ID: id.IntentID("int_g13_cancelled"), SchemaVersion: agentprocess.IntentSchemaVersion, Goal: "verify cancellation guard", CreatedAt: now},
	}}
	store := transaction.NewMemoryStore()
	op := newG13TransactionOperator(store, policy, ids, func() time.Time { return now })
	tx, err := op.manager.Begin(ctx, transaction.BeginRequest{AgentID: agentID, BaseCheckpoint: id.CheckpointID("chk_g13_cancelled")}, branch)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := op.Operate(ctx, controlapi.WorkspaceTransactionOperateRequest{TransactionID: tx.ID, Operation: controlapi.WorkspaceTransactionVerify, Command: []string{"git", "status", "--porcelain"}}); err != nil {
		t.Fatal(err)
	}
	policy.state.Status = agentprocess.StatusCancelled
	policy.state.Cancel.Requested = true
	if _, err := op.Operate(ctx, controlapi.WorkspaceTransactionOperateRequest{TransactionID: tx.ID, Operation: controlapi.WorkspaceTransactionPrepare}); err == nil {
		t.Fatal("cancelled Agent unexpectedly passed promotion guard")
	}
}

func initG13OperatorRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runG13Git(t, repo, "init")
	runG13Git(t, repo, "config", "user.email", "g13@example.invalid")
	runG13Git(t, repo, "config", "user.name", "G13 Test")
	if err := os.WriteFile(filepath.Join(repo, "value.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runG13Git(t, repo, "add", "value.txt")
	runG13Git(t, repo, "commit", "-m", "base")
	return repo
}

func runG13Git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	body, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, body)
	}
	return string(body)
}

func readG13OperatorFile(t *testing.T, repo, name string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(repo, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
