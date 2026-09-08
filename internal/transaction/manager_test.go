package transaction

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/world"
)

type fakeTransactionalWorld struct {
	ref             world.BranchRef
	executeCalls    int
	rollbackCalls   int
	finalizeCalls   int
	applyCalls      int
	executeResult   world.Result
	executeErr      error
	applyErr        error
	verifyErr       error
	reconcileStatus world.PromotionStatus
	reconcileErr    error
}

func newFakeTransactionalWorld() *fakeTransactionalWorld {
	return &fakeTransactionalWorld{
		ref: world.BranchRef{WorldID: id.WorldID("wld_tx_test"), Type: world.TypeWorkspace, BaseIdentity: "base-tree", Ref: "test"},
		executeResult:   world.Result{Status: world.ResultSucceeded},
		reconcileStatus: world.PromotionNotApplied,
	}
}

func (w *fakeTransactionalWorld) Profile() world.Profile {
	return world.Profile{
		Type:       world.TypeWorkspace,
		Filesystem: world.FilesystemGuarantees{MutationIsolation: true},
		Promotion:  world.PromotionGuarantees{Supported: true, ThreeWay: true, Reconciliation: true},
	}
}

func (w *fakeTransactionalWorld) Execute(context.Context, world.Action) (world.Result, error) {
	w.executeCalls++
	return w.executeResult, w.executeErr
}

func (w *fakeTransactionalWorld) BranchRef() world.BranchRef { return w.ref }

func (w *fakeTransactionalWorld) PreparePromotion(_ context.Context, operationID id.OperationID) (world.PromotionPlan, error) {
	return world.PromotionPlan{OperationID: operationID, BaseIdentity: w.ref.BaseIdentity, TargetBefore: "target", Source: "source", Merged: "merged"}, nil
}

func (w *fakeTransactionalWorld) ApplyPromotion(context.Context, world.PromotionPlan) error {
	w.applyCalls++
	return w.applyErr
}

func (w *fakeTransactionalWorld) VerifyPromotion(context.Context, world.PromotionPlan) error {
	return w.verifyErr
}

func (w *fakeTransactionalWorld) ReconcilePromotion(context.Context, world.PromotionPlan) (world.PromotionStatus, error) {
	return w.reconcileStatus, w.reconcileErr
}

func (w *fakeTransactionalWorld) Rollback(context.Context) error {
	w.rollbackCalls++
	return nil
}

func (w *fakeTransactionalWorld) Finalize(context.Context) error {
	w.finalizeCalls++
	return nil
}

func (w *fakeTransactionalWorld) Close(context.Context) error { return nil }

func TestIrreversibleEffectIsDeferredBeforeWorldExecution(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	manager := NewManager(store, nil, func() time.Time { return time.Unix(100, 0) })
	branch := newFakeTransactionalWorld()
	tx := beginTestTransaction(t, manager, store, branch)
	action := world.Action{ID: id.ActionID("act_irreversible"), AgentID: tx.AgentID, Kind: "network.request", Purpose: "publish", Effect: world.CanonicalEffect("network.request")}

	result, err := manager.Execute(ctx, tx.ID, branch, action)
	if !errors.Is(err, ErrIrreversibleDeferred) || result.Status != world.ResultDenied {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if branch.executeCalls != 0 {
		t.Fatalf("irreversible action reached World: calls=%d", branch.executeCalls)
	}
	events, err := store.TransactionEvents(ctx, tx.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !hasEvent(events, "IrreversibleEffectDeferred") {
		t.Fatalf("deferred effect missing from audit: %+v", events)
	}
}

type failDispatchStore struct{ *MemoryStore }

func (s *failDispatchStore) UpdateEffect(ctx context.Context, effectID id.EffectRecordID, state EffectState, certainty OutcomeCertainty, effectError, eventType string, eventPayload json.RawMessage) error {
	if state == EffectDispatched {
		return errors.New("injected durable write failure")
	}
	return s.MemoryStore.UpdateEffect(ctx, effectID, state, certainty, effectError, eventType, eventPayload)
}

func TestStorageFailureBeforeDispatchPreventsMutation(t *testing.T) {
	ctx := context.Background()
	store := &failDispatchStore{MemoryStore: NewMemoryStore()}
	manager := NewManager(store, nil, nil)
	branch := newFakeTransactionalWorld()
	tx := beginTestTransaction(t, manager, store.MemoryStore, branch)
	action := world.Action{ID: id.ActionID("act_write"), AgentID: tx.AgentID, Kind: "fs.write_file", Purpose: "edit", Effect: world.CanonicalEffect("fs.write_file")}

	_, err := manager.Execute(ctx, tx.ID, branch, action)
	if err == nil || err.Error() != "injected durable write failure" {
		t.Fatalf("execute err=%v", err)
	}
	if branch.executeCalls != 0 {
		t.Fatalf("mutation crossed World boundary after storage failure: calls=%d", branch.executeCalls)
	}
}

func TestFailedVerificationReturnsOpenThenRollsBack(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	manager := NewManager(store, nil, nil)
	branch := newFakeTransactionalWorld()
	branch.executeResult = world.Result{Status: world.ResultFailed, Error: "tests failed"}
	tx := beginTestTransaction(t, manager, store, branch)
	check := CheckSpec{Name: "go test", Action: world.Action{ID: id.ActionID("act_check"), Kind: "process.exec", Purpose: "verify", Effect: world.CanonicalEffect("process.exec")}}

	verification, err := manager.Verify(ctx, tx.ID, branch, []CheckSpec{check})
	if !errors.Is(err, ErrVerificationFailed) || verification.Status != VerificationFailed {
		t.Fatalf("verification=%+v err=%v", verification, err)
	}
	current, err := store.Get(ctx, tx.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.State != StateOpen {
		t.Fatalf("failed verification state=%s, want OPEN", current.State)
	}
	rolled, err := manager.Rollback(ctx, tx.ID, branch)
	if err != nil {
		t.Fatal(err)
	}
	if rolled.State != StateRolledBack || branch.rollbackCalls != 1 {
		t.Fatalf("rolled=%+v rollback_calls=%d", rolled, branch.rollbackCalls)
	}
	events, _ := store.TransactionEvents(ctx, tx.ID)
	if !hasEvent(events, "VerificationRejected") || !hasEvent(events, "RollbackFinalized") {
		t.Fatalf("rollback audit incomplete: %+v", events)
	}
}

func TestCrashLikeApplyFailureNeverProducesFalseCommit(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	manager := NewManager(store, nil, nil)
	branch := newFakeTransactionalWorld()
	tx := beginTestTransaction(t, manager, store, branch)
	check := CheckSpec{Name: "pass", Action: world.Action{ID: id.ActionID("act_pass"), Kind: "process.exec", Purpose: "verify", Effect: world.CanonicalEffect("process.exec")}}
	if _, err := manager.Verify(ctx, tx.ID, branch, []CheckSpec{check}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Prepare(ctx, tx.ID, branch); err != nil {
		t.Fatal(err)
	}
	branch.applyErr = errors.New("simulated crash during apply")
	current, err := manager.Commit(ctx, tx.ID, branch)
	if !errors.Is(err, ErrReconciliationRequired) {
		t.Fatalf("commit err=%v", err)
	}
	if current.State != StateNeedsReconciliation {
		t.Fatalf("state=%s, want NEEDS_RECONCILIATION", current.State)
	}
	stored, _ := store.Get(ctx, tx.ID)
	if stored.State == StateCommitted {
		t.Fatal("uncertain apply was falsely committed")
	}
}

func TestReconciliationCanFinalizeKnownAppliedCommit(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	manager := NewManager(store, nil, nil)
	branch := newFakeTransactionalWorld()
	tx := beginTestTransaction(t, manager, store, branch)
	check := CheckSpec{Name: "pass", Action: world.Action{ID: id.ActionID("act_pass_reconcile"), Kind: "process.exec", Purpose: "verify", Effect: world.CanonicalEffect("process.exec")}}
	if _, err := manager.Verify(ctx, tx.ID, branch, []CheckSpec{check}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Prepare(ctx, tx.ID, branch); err != nil {
		t.Fatal(err)
	}
	branch.applyErr = errors.New("transport lost after apply")
	if _, err := manager.Commit(ctx, tx.ID, branch); !errors.Is(err, ErrReconciliationRequired) {
		t.Fatalf("commit err=%v", err)
	}
	branch.reconcileStatus = world.PromotionApplied
	branch.applyErr = nil
	reconciled, err := manager.Reconcile(ctx, tx.ID, branch)
	if err != nil {
		t.Fatal(err)
	}
	if reconciled.State != StateCommitted || branch.finalizeCalls != 1 {
		t.Fatalf("reconciled=%+v finalize_calls=%d", reconciled, branch.finalizeCalls)
	}
}

func beginTestTransaction(t *testing.T, manager *Manager, _ *MemoryStore, branch *fakeTransactionalWorld) Transaction {
	t.Helper()
	tx, err := manager.Begin(context.Background(), BeginRequest{AgentID: id.AgentID("agt_tx_test"), BaseCheckpoint: id.CheckpointID("chk_tx_test")}, branch)
	if err != nil {
		t.Fatal(err)
	}
	if tx.State != StateOpen {
		t.Fatalf("begin state=%s", tx.State)
	}
	return tx
}

func hasEvent(events []Event, eventType string) bool {
	for _, event := range events {
		if event.Type == eventType {
			return true
		}
	}
	return false
}
