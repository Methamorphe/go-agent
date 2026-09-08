package transaction

import (
	"context"
	"errors"
	"testing"

	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/world"
)

func TestUnknownDispatchedEffectEntersReconciliationAndCannotFalseRollback(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	manager := NewManager(store, nil, nil)
	branch := newFakeTransactionalWorld()
	branch.executeErr = errors.New("world connection lost after dispatch")
	tx := beginTestTransaction(t, manager, store, branch)
	action := world.Action{
		ID:      id.ActionID("act_unknown"),
		AgentID: tx.AgentID,
		Kind:    "fs.write_file",
		Purpose: "speculative edit",
		Effect:  world.CanonicalEffect("fs.write_file"),
	}

	_, err := manager.Execute(ctx, tx.ID, branch, action)
	if !errors.Is(err, ErrReconciliationRequired) {
		t.Fatalf("execute err=%v", err)
	}
	current, err := store.Get(ctx, tx.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.State != StateNeedsReconciliation {
		t.Fatalf("state=%s, want NEEDS_RECONCILIATION", current.State)
	}
	effects, err := store.ListEffects(ctx, tx.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(effects) != 1 || effects[0].State != EffectOutcomeUnknown || effects[0].OutcomeCertainty != OutcomeUnknown {
		t.Fatalf("effects=%+v", effects)
	}

	if _, err := manager.Reconcile(ctx, tx.ID, branch); !errors.Is(err, ErrReconciliationRequired) {
		t.Fatalf("reconcile err=%v", err)
	}
	if branch.rollbackCalls != 0 {
		t.Fatalf("unknown effect was falsely rolled back: calls=%d", branch.rollbackCalls)
	}

	if err := manager.ResolveEffect(ctx, tx.ID, EffectResolution{
		EffectID:  effects[0].ID,
		Certainty: OutcomeKnownAbsent,
		Evidence:  "adapter confirmed operation never crossed its mutation boundary",
	}); err != nil {
		t.Fatal(err)
	}
	reconciled, err := manager.Reconcile(ctx, tx.ID, branch)
	if err != nil {
		t.Fatal(err)
	}
	if reconciled.State != StateRolledBack || branch.rollbackCalls != 1 {
		t.Fatalf("reconciled=%+v rollback_calls=%d", reconciled, branch.rollbackCalls)
	}
}

func TestKnownAppliedCompensatableEffectCannotClaimCleanRollback(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	manager := NewManager(store, nil, nil)
	branch := newFakeTransactionalWorld()
	tx := beginTestTransaction(t, manager, store, branch)
	action := world.Action{
		ID:      id.ActionID("act_process_external"),
		AgentID: tx.AgentID,
		Kind:    "process.exec",
		Purpose: "run potentially effectful process",
		Effect:  world.CanonicalEffect("process.exec"),
	}
	if _, err := manager.Execute(ctx, tx.ID, branch, action); err != nil {
		t.Fatal(err)
	}

	rolled, err := manager.Rollback(ctx, tx.ID, branch)
	if !errors.Is(err, ErrReconciliationRequired) {
		t.Fatalf("rollback err=%v", err)
	}
	if rolled.State != StateNeedsReconciliation || branch.rollbackCalls != 0 {
		t.Fatalf("rolled=%+v rollback_calls=%d", rolled, branch.rollbackCalls)
	}
}

func TestCommitGuardIsRevalidatedImmediatelyBeforeCommit(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	manager := NewManager(store, nil, nil)
	branch := newFakeTransactionalWorld()
	allowed := true
	manager.SetCommitGuard(func(context.Context, Transaction) error {
		if !allowed {
			return errors.New("capability lease revoked")
		}
		return nil
	})
	tx := beginTestTransaction(t, manager, store, branch)
	check := CheckSpec{
		Name: "pass",
		Action: world.Action{
			ID:      id.ActionID("act_guard_check"),
			Kind:    "process.exec",
			Purpose: "verify",
			Effect:  world.CanonicalEffect("process.exec"),
		},
	}
	if _, err := manager.Verify(ctx, tx.ID, branch, []CheckSpec{check}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Prepare(ctx, tx.ID, branch); err != nil {
		t.Fatal(err)
	}
	allowed = false
	current, err := manager.Commit(ctx, tx.ID, branch)
	if err == nil || !errors.Is(err, errors.Unwrap(err)) && err.Error() == "" {
		t.Fatalf("commit unexpectedly allowed: tx=%+v err=%v", current, err)
	}
	if branch.applyCalls != 0 {
		t.Fatalf("revoked transaction reached APPLY: calls=%d", branch.applyCalls)
	}
	stored, err := store.Get(ctx, tx.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != StateReadyToCommit {
		t.Fatalf("revoked commit changed state=%s", stored.State)
	}
}

type unsupportedTransactionalWorld struct {
	*fakeTransactionalWorld
}

func (w *unsupportedTransactionalWorld) Profile() world.Profile {
	return world.Profile{
		Type:       world.TypeWorkspace,
		Filesystem: world.FilesystemGuarantees{MutationIsolation: false},
		Promotion:  world.PromotionGuarantees{Supported: false},
	}
}

func TestUnsupportedWorldCannotClaimTransactionSemantics(t *testing.T) {
	manager := NewManager(NewMemoryStore(), nil, nil)
	branch := &unsupportedTransactionalWorld{fakeTransactionalWorld: newFakeTransactionalWorld()}
	_, err := manager.Begin(context.Background(), BeginRequest{
		AgentID:        id.AgentID("agt_unsupported"),
		BaseCheckpoint: id.CheckpointID("chk_unsupported"),
	}, branch)
	if !errors.Is(err, world.ErrTransactionUnsupported) {
		t.Fatalf("begin err=%v", err)
	}
}
