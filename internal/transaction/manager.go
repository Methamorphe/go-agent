package transaction

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/world"
)

type CommitGuard func(context.Context, Transaction) error

type Manager struct {
	store       Store
	ids         *id.Generator
	now         func() time.Time
	commitGuard CommitGuard
}

type BeginRequest struct {
	AgentID        id.AgentID
	BaseCheckpoint id.CheckpointID
	CommitPolicy   CommitPolicy
}

type EffectResolution struct {
	EffectID  id.EffectRecordID
	Certainty OutcomeCertainty
	Evidence  string
}

func NewManager(store Store, generator *id.Generator, now func() time.Time) *Manager {
	if generator == nil {
		generator = id.NewGenerator()
	}
	if now == nil {
		now = time.Now
	}
	return &Manager{store: store, ids: generator, now: now}
}

// SetCommitGuard installs the current-policy/capability revalidation hook used
// immediately before PREPARE and again before COMMIT. It is intended to be
// configured during runtime construction, before concurrent use of Manager.
func (m *Manager) SetCommitGuard(guard CommitGuard) {
	if m != nil {
		m.commitGuard = guard
	}
}

func (m *Manager) Begin(ctx context.Context, request BeginRequest, branch world.TransactionalWorld) (Transaction, error) {
	if m == nil || m.store == nil || branch == nil {
		return Transaction{}, fmt.Errorf("transaction manager, store and world are required")
	}
	if request.AgentID == "" || request.BaseCheckpoint == "" {
		return Transaction{}, fmt.Errorf("agent id and base checkpoint are required")
	}
	profile := branch.Profile()
	if !profile.Promotion.Supported || !profile.Filesystem.MutationIsolation {
		return Transaction{}, world.ErrTransactionUnsupported
	}
	branchRef := branch.BranchRef()
	if branchRef.WorldID == "" || branchRef.BaseIdentity == "" {
		return Transaction{}, fmt.Errorf("transactional world branch reference is incomplete")
	}
	txID, err := m.ids.Transaction()
	if err != nil {
		return Transaction{}, err
	}
	policy := request.CommitPolicy
	if policy == "" {
		policy = CommitRequireVerification
	}
	if policy != CommitRequireVerification {
		return Transaction{}, fmt.Errorf("unsupported transaction commit policy %q", policy)
	}
	now := m.now().UTC()
	tx := Transaction{
		ID:               txID,
		AgentID:          request.AgentID,
		BaseCheckpoint:   request.BaseCheckpoint,
		WorldID:          branchRef.WorldID,
		IsolatedWorldRef: branchRef,
		State:            StateCreating,
		Version:          1,
		CommitPolicy:     policy,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := m.store.Create(ctx, tx, "TransactionCreated", payload(map[string]any{"world": branchRef, "base_checkpoint": request.BaseCheckpoint})); err != nil {
		_ = branch.Rollback(ctx)
		return Transaction{}, err
	}
	opened, err := m.store.Transition(ctx, tx.ID, tx.Version, []State{StateCreating}, StateOpen, nil, "", "TransactionOpened", nil)
	if err != nil {
		_ = branch.Rollback(ctx)
		return Transaction{}, err
	}
	return opened, nil
}

func (m *Manager) Execute(ctx context.Context, txID id.TransactionID, branch world.TransactionalWorld, action world.Action) (world.Result, error) {
	tx, err := m.loadForWorld(ctx, txID, branch)
	if err != nil {
		return world.Result{}, err
	}
	if tx.State != StateOpen {
		return world.Result{}, fmt.Errorf("%w: execute requires OPEN, got %s", ErrInvalidState, tx.State)
	}
	if action.AgentID != tx.AgentID {
		return world.Result{}, fmt.Errorf("action agent does not own transaction")
	}
	canonical := world.CanonicalEffect(action.Kind)
	if action.Effect != canonical {
		return world.Result{}, fmt.Errorf("action effect does not match canonical classification")
	}

	effectID, err := m.ids.EffectRecord()
	if err != nil {
		return world.Result{}, err
	}
	now := m.now().UTC()
	record := EffectRecord{
		ID:               effectID,
		TransactionID:    tx.ID,
		ActionID:         action.ID,
		Kind:             action.Kind,
		Effect:           canonical,
		State:            EffectPrepared,
		OutcomeCertainty: OutcomeNotApplicable,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := m.store.CreateEffect(ctx, record, "EffectPrepared", payload(map[string]any{"action_id": action.ID, "effect": canonical})); err != nil {
		return world.Result{}, err
	}
	if canonical.Class == world.EffectIrreversible {
		if err := m.store.UpdateEffect(ctx, effectID, EffectDeferred, OutcomeKnownAbsent, "", "IrreversibleEffectDeferred", payload(map[string]any{"action_id": action.ID})); err != nil {
			return world.Result{}, err
		}
		return world.Result{Status: world.ResultDenied, Error: ErrIrreversibleDeferred.Error()}, ErrIrreversibleDeferred
	}

	// DISPATCHED is durable before the World boundary. If the runtime loses
	// certainty after this point, the transaction itself enters reconciliation;
	// it never remains OPEN and eligible for a false clean rollback.
	if err := m.store.UpdateEffect(ctx, effectID, EffectDispatched, OutcomeUnknown, "", "EffectDispatched", payload(map[string]any{"action_id": action.ID})); err != nil {
		return world.Result{}, err
	}
	result, executeErr := branch.Execute(ctx, action)
	if executeErr != nil {
		persistErr := m.store.UpdateEffect(ctx, effectID, EffectOutcomeUnknown, OutcomeUnknown, executeErr.Error(), "EffectOutcomeUnknown", payload(map[string]any{"action_id": action.ID, "error": executeErr.Error()}))
		if persistErr != nil {
			return result, errors.Join(executeErr, persistErr)
		}
		_, reconcileErr := m.markNeedsReconciliation(ctx, tx, executeErr)
		return result, reconcileErr
	}
	if err := m.store.UpdateEffect(ctx, effectID, EffectCompleted, OutcomeKnownApplied, "", "EffectCompleted", payload(map[string]any{"action_id": action.ID, "status": result.Status})); err != nil {
		return result, err
	}
	return result, nil
}

func (m *Manager) Verify(ctx context.Context, txID id.TransactionID, branch world.TransactionalWorld, checks []CheckSpec) (Verification, error) {
	tx, err := m.loadForWorld(ctx, txID, branch)
	if err != nil {
		return Verification{}, err
	}
	if tx.State != StateOpen {
		return Verification{}, fmt.Errorf("%w: verify requires OPEN, got %s", ErrInvalidState, tx.State)
	}
	if len(checks) == 0 {
		return Verification{}, fmt.Errorf("at least one verification check is required")
	}
	verifying, err := m.store.Transition(ctx, tx.ID, tx.Version, []State{StateOpen}, StateVerifying, tx.PreparedPlan, "", "VerificationStarted", payload(map[string]any{"check_count": len(checks)}))
	if err != nil {
		return Verification{}, err
	}
	verificationID, err := m.ids.Verification()
	if err != nil {
		return Verification{}, err
	}
	started := m.now().UTC()
	verification := Verification{ID: verificationID, TransactionID: tx.ID, Checks: append([]CheckSpec(nil), checks...), Status: VerificationRunning, StartedAt: started}
	verification.Results = make([]CheckResult, 0, len(checks))
	passed := true
	for _, check := range checks {
		action := check.Action
		if action.AgentID == "" {
			action.AgentID = tx.AgentID
		}
		if action.AgentID != tx.AgentID || action.Effect != world.CanonicalEffect(action.Kind) || action.Effect.Class == world.EffectIrreversible {
			verification.Results = append(verification.Results, CheckResult{Name: check.Name, Status: world.ResultFailed, Error: "verification action is not admissible"})
			passed = false
			continue
		}
		result, checkErr := branch.Execute(ctx, action)
		checkResult := CheckResult{Name: check.Name, Status: result.Status, ExitCode: result.ExitCode, Error: result.Error}
		if len(result.Data) > 0 {
			sum := sha256.Sum256(result.Data)
			checkResult.DataSHA256 = hex.EncodeToString(sum[:])
		}
		if checkErr != nil {
			checkResult.Error = checkErr.Error()
		}
		if checkErr != nil || result.Status != world.ResultSucceeded {
			passed = false
		}
		verification.Results = append(verification.Results, checkResult)
	}
	completed := m.now().UTC()
	verification.CompletedAt = &completed
	if passed {
		verification.Status = VerificationPassed
	} else {
		verification.Status = VerificationFailed
	}
	if err := m.store.CreateVerification(ctx, verification, "VerificationCompleted", payload(map[string]any{"verification_id": verification.ID, "status": verification.Status})); err != nil {
		return verification, err
	}
	if !passed {
		_, transitionErr := m.store.Transition(ctx, tx.ID, verifying.Version, []State{StateVerifying}, StateOpen, verifying.PreparedPlan, "", "VerificationRejected", payload(map[string]any{"verification_id": verification.ID}))
		if transitionErr != nil {
			return verification, errors.Join(ErrVerificationFailed, transitionErr)
		}
		return verification, ErrVerificationFailed
	}
	_, err = m.store.Transition(ctx, tx.ID, verifying.Version, []State{StateVerifying}, StateReadyToCommit, verifying.PreparedPlan, "", "VerificationAccepted", payload(map[string]any{"verification_id": verification.ID}))
	if err != nil {
		return verification, err
	}
	return verification, nil
}

func (m *Manager) Prepare(ctx context.Context, txID id.TransactionID, branch world.TransactionalWorld) (world.PromotionPlan, error) {
	tx, err := m.loadForWorld(ctx, txID, branch)
	if err != nil {
		return world.PromotionPlan{}, err
	}
	if tx.State != StateReadyToCommit {
		return world.PromotionPlan{}, fmt.Errorf("%w: prepare requires READY_TO_COMMIT, got %s", ErrInvalidState, tx.State)
	}
	if err := m.checkCommitGuard(ctx, tx); err != nil {
		return world.PromotionPlan{}, err
	}
	if tx.PreparedPlan != nil && tx.PreparedPlan.Merged != "" {
		return *tx.PreparedPlan, nil
	}

	operationID := id.OperationID("")
	if tx.PreparedPlan != nil {
		operationID = tx.PreparedPlan.OperationID
	}
	if operationID == "" {
		operationID, err = m.ids.Operation()
		if err != nil {
			return world.PromotionPlan{}, err
		}
		provisional := &world.PromotionPlan{OperationID: operationID, BaseIdentity: tx.IsolatedWorldRef.BaseIdentity}
		tx, err = m.store.Transition(ctx, tx.ID, tx.Version, []State{StateReadyToCommit}, StateReadyToCommit, provisional, "", "PromotionPrepareStarted", payload(map[string]any{"operation_id": operationID}))
		if err != nil {
			return world.PromotionPlan{}, err
		}
	}

	plan, err := branch.PreparePromotion(ctx, operationID)
	if err != nil {
		return world.PromotionPlan{}, err
	}
	prepared, err := m.store.Transition(ctx, tx.ID, tx.Version, []State{StateReadyToCommit}, StateReadyToCommit, &plan, "", "TransactionPrepared", payload(plan))
	if err != nil {
		return world.PromotionPlan{}, err
	}
	return *prepared.PreparedPlan, nil
}

func (m *Manager) Commit(ctx context.Context, txID id.TransactionID, branch world.TransactionalWorld) (Transaction, error) {
	tx, err := m.loadForWorld(ctx, txID, branch)
	if err != nil {
		return Transaction{}, err
	}
	if tx.State != StateReadyToCommit || tx.PreparedPlan == nil || tx.PreparedPlan.Merged == "" {
		return Transaction{}, fmt.Errorf("%w: commit requires a fully prepared transaction", ErrInvalidState)
	}
	if err := m.checkCommitGuard(ctx, tx); err != nil {
		return tx, err
	}
	committing, err := m.store.Transition(ctx, tx.ID, tx.Version, []State{StateReadyToCommit}, StateCommitting, tx.PreparedPlan, "", "CommitStarted", payload(map[string]any{"operation_id": tx.PreparedPlan.OperationID}))
	if err != nil {
		return Transaction{}, err
	}
	if err := branch.ApplyPromotion(ctx, *committing.PreparedPlan); err != nil {
		return m.markNeedsReconciliation(ctx, committing, err)
	}
	if err := branch.VerifyPromotion(ctx, *committing.PreparedPlan); err != nil {
		return m.markNeedsReconciliation(ctx, committing, err)
	}
	committed, err := m.store.Transition(ctx, tx.ID, committing.Version, []State{StateCommitting}, StateCommitted, committing.PreparedPlan, "", "CommitFinalized", payload(map[string]any{"operation_id": committing.PreparedPlan.OperationID}))
	if err != nil {
		return Transaction{}, err
	}
	if err := branch.Finalize(ctx); err != nil {
		return committed, fmt.Errorf("transaction committed but branch cleanup failed: %w", err)
	}
	return committed, nil
}

func (m *Manager) Rollback(ctx context.Context, txID id.TransactionID, branch world.TransactionalWorld) (Transaction, error) {
	tx, err := m.loadForWorld(ctx, txID, branch)
	if err != nil {
		return Transaction{}, err
	}
	if !containsState([]State{StateOpen, StateVerifying, StateReadyToCommit}, tx.State) {
		return Transaction{}, fmt.Errorf("%w: rollback not allowed from %s", ErrInvalidState, tx.State)
	}
	if reason, err := m.rollbackHazard(ctx, tx.ID); err != nil {
		return Transaction{}, err
	} else if reason != "" {
		return m.markNeedsReconciliation(ctx, tx, errors.New(reason))
	}
	rolling, err := m.store.Transition(ctx, tx.ID, tx.Version, []State{tx.State}, StateRollingBack, tx.PreparedPlan, "", "RollbackStarted", nil)
	if err != nil {
		return Transaction{}, err
	}
	if err := branch.Rollback(ctx); err != nil {
		return m.markNeedsReconciliation(ctx, rolling, err)
	}
	return m.store.Transition(ctx, tx.ID, rolling.Version, []State{StateRollingBack}, StateRolledBack, rolling.PreparedPlan, "", "RollbackFinalized", nil)
}

func (m *Manager) Reconcile(ctx context.Context, txID id.TransactionID, branch world.TransactionalWorld) (Transaction, error) {
	tx, err := m.loadForWorld(ctx, txID, branch)
	if err != nil {
		return Transaction{}, err
	}
	if tx.State == StateCommitting || tx.State == StateRollingBack {
		tx, err = m.store.Transition(ctx, tx.ID, tx.Version, []State{tx.State}, StateNeedsReconciliation, tx.PreparedPlan, "interrupted critical transaction phase", "ReconciliationRequired", nil)
		if err != nil {
			return Transaction{}, err
		}
	}
	if tx.State != StateNeedsReconciliation {
		return Transaction{}, fmt.Errorf("%w: reconcile requires NEEDS_RECONCILIATION, got %s", ErrInvalidState, tx.State)
	}

	if tx.PreparedPlan == nil || tx.PreparedPlan.Merged == "" {
		if reason, err := m.rollbackHazard(ctx, tx.ID); err != nil {
			return tx, err
		} else if reason != "" {
			return tx, errors.Join(ErrReconciliationRequired, errors.New(reason))
		}
		if err := branch.Rollback(ctx); err != nil {
			return tx, errors.Join(ErrReconciliationRequired, err)
		}
		return m.store.Transition(ctx, tx.ID, tx.Version, []State{StateNeedsReconciliation}, StateRolledBack, tx.PreparedPlan, "", "ReconciledAsRolledBack", nil)
	}
	status, err := branch.ReconcilePromotion(ctx, *tx.PreparedPlan)
	if err != nil {
		return tx, errors.Join(ErrReconciliationRequired, err)
	}
	switch status {
	case world.PromotionApplied:
		if reason, err := m.unresolvedEffectHazard(ctx, tx.ID); err != nil {
			return tx, err
		} else if reason != "" {
			return tx, errors.Join(ErrReconciliationRequired, errors.New(reason))
		}
		if err := branch.VerifyPromotion(ctx, *tx.PreparedPlan); err != nil {
			return tx, errors.Join(ErrReconciliationRequired, err)
		}
		committed, err := m.store.Transition(ctx, tx.ID, tx.Version, []State{StateNeedsReconciliation}, StateCommitted, tx.PreparedPlan, "", "ReconciledAsCommitted", payload(map[string]any{"operation_id": tx.PreparedPlan.OperationID}))
		if err != nil {
			return Transaction{}, err
		}
		if err := branch.Finalize(ctx); err != nil {
			return committed, fmt.Errorf("reconciled commit but branch cleanup failed: %w", err)
		}
		return committed, nil
	case world.PromotionNotApplied:
		if reason, err := m.rollbackHazard(ctx, tx.ID); err != nil {
			return tx, err
		} else if reason != "" {
			return tx, errors.Join(ErrReconciliationRequired, errors.New(reason))
		}
		if err := branch.Rollback(ctx); err != nil {
			return tx, errors.Join(ErrReconciliationRequired, err)
		}
		return m.store.Transition(ctx, tx.ID, tx.Version, []State{StateNeedsReconciliation}, StateRolledBack, tx.PreparedPlan, "", "ReconciledAsRolledBack", payload(map[string]any{"operation_id": tx.PreparedPlan.OperationID}))
	default:
		return tx, ErrReconciliationRequired
	}
}

func (m *Manager) ResolveEffect(ctx context.Context, txID id.TransactionID, resolution EffectResolution) error {
	if resolution.EffectID == "" || resolution.Evidence == "" {
		return fmt.Errorf("effect id and reconciliation evidence are required")
	}
	if resolution.Certainty != OutcomeKnownApplied && resolution.Certainty != OutcomeKnownAbsent {
		return fmt.Errorf("effect reconciliation requires a known outcome")
	}
	tx, err := m.store.Get(ctx, txID)
	if err != nil {
		return err
	}
	if tx.State != StateNeedsReconciliation {
		return fmt.Errorf("%w: effect resolution requires NEEDS_RECONCILIATION", ErrInvalidState)
	}
	effects, err := m.store.ListEffects(ctx, txID)
	if err != nil {
		return err
	}
	var target *EffectRecord
	for i := range effects {
		if effects[i].ID == resolution.EffectID {
			target = &effects[i]
			break
		}
	}
	if target == nil {
		return ErrNotFound
	}
	if target.OutcomeCertainty != OutcomeUnknown && target.State != EffectDispatched && target.State != EffectOutcomeUnknown {
		return fmt.Errorf("effect outcome is already resolved")
	}
	state := EffectFailedBeforeEffect
	if resolution.Certainty == OutcomeKnownApplied {
		state = EffectCompleted
	}
	return m.store.UpdateEffect(ctx, target.ID, state, resolution.Certainty, "", "EffectReconciled", payload(map[string]any{
		"effect_id": target.ID,
		"certainty": resolution.Certainty,
		"evidence":  resolution.Evidence,
	}))
}

func (m *Manager) Recover(ctx context.Context, txID id.TransactionID, branch world.TransactionalWorld) (Transaction, error) {
	tx, err := m.loadForWorld(ctx, txID, branch)
	if err != nil {
		return Transaction{}, err
	}
	switch tx.State {
	case StateCreating:
		if err := branch.Rollback(ctx); err != nil {
			return m.markNeedsReconciliation(ctx, tx, err)
		}
		return m.store.Transition(ctx, tx.ID, tx.Version, []State{StateCreating}, StateFailed, tx.PreparedPlan, "incomplete transaction creation recovered", "CreationRecoveryFailedClosed", nil)
	case StateVerifying:
		return m.store.Transition(ctx, tx.ID, tx.Version, []State{StateVerifying}, StateOpen, tx.PreparedPlan, "", "VerificationInterrupted", nil)
	case StateCommitting, StateRollingBack, StateNeedsReconciliation:
		return m.Reconcile(ctx, tx.ID, branch)
	default:
		return tx, nil
	}
}

func (m *Manager) loadForWorld(ctx context.Context, txID id.TransactionID, branch world.TransactionalWorld) (Transaction, error) {
	if m == nil || m.store == nil || branch == nil || txID == "" {
		return Transaction{}, fmt.Errorf("transaction, store and world are required")
	}
	tx, err := m.store.Get(ctx, txID)
	if err != nil {
		return Transaction{}, err
	}
	ref := branch.BranchRef()
	if ref.WorldID != tx.WorldID || ref.BaseIdentity != tx.IsolatedWorldRef.BaseIdentity {
		return Transaction{}, fmt.Errorf("transaction world reference mismatch")
	}
	return tx, nil
}

func (m *Manager) checkCommitGuard(ctx context.Context, tx Transaction) error {
	if m.commitGuard == nil {
		return nil
	}
	if err := m.commitGuard(ctx, tx); err != nil {
		return fmt.Errorf("transaction commit authorization is no longer valid: %w", err)
	}
	return nil
}

func (m *Manager) rollbackHazard(ctx context.Context, txID id.TransactionID) (string, error) {
	effects, err := m.store.ListEffects(ctx, txID)
	if err != nil {
		return "", err
	}
	for _, effect := range effects {
		if effect.OutcomeCertainty == OutcomeUnknown || effect.State == EffectDispatched || effect.State == EffectOutcomeUnknown {
			return fmt.Sprintf("effect %s has unresolved outcome", effect.ID), nil
		}
		if effect.OutcomeCertainty == OutcomeKnownApplied && (effect.Effect.Class == world.EffectCompensatable || effect.Effect.Class == world.EffectIrreversible) {
			return fmt.Sprintf("effect %s is externally visible and has no proven compensation", effect.ID), nil
		}
	}
	return "", nil
}

func (m *Manager) unresolvedEffectHazard(ctx context.Context, txID id.TransactionID) (string, error) {
	effects, err := m.store.ListEffects(ctx, txID)
	if err != nil {
		return "", err
	}
	for _, effect := range effects {
		if effect.OutcomeCertainty == OutcomeUnknown || effect.State == EffectDispatched || effect.State == EffectOutcomeUnknown {
			return fmt.Sprintf("effect %s has unresolved outcome", effect.ID), nil
		}
	}
	return "", nil
}

func (m *Manager) markNeedsReconciliation(ctx context.Context, tx Transaction, cause error) (Transaction, error) {
	reconciled, err := m.store.Transition(ctx, tx.ID, tx.Version, []State{tx.State}, StateNeedsReconciliation, tx.PreparedPlan, cause.Error(), "ReconciliationRequired", payload(map[string]any{"cause": cause.Error()}))
	if err != nil {
		return Transaction{}, errors.Join(cause, err)
	}
	return reconciled, errors.Join(ErrReconciliationRequired, cause)
}
