package world

import (
	"context"
	"fmt"

	"github.com/Methamorphe/go-agent/internal/id"
)

// SpeculativeTransactionalWorld enforces the execution-edit irreversible
// barrier even when a caller bypasses transaction.Manager.Execute and invokes
// the branch World directly. Promotion mechanics still delegate to the inner
// transactional World.
type SpeculativeTransactionalWorld struct {
	inner TransactionalWorld
}

func NewSpeculativeTransactionalWorld(inner TransactionalWorld) *SpeculativeTransactionalWorld {
	if inner == nil { return nil }
	return &SpeculativeTransactionalWorld{inner: inner}
}
func (w *SpeculativeTransactionalWorld) Profile() Profile { return w.inner.Profile() }
func (w *SpeculativeTransactionalWorld) Execute(ctx context.Context, action Action) (Result, error) {
	canonical := CanonicalEffect(action.Kind)
	if action.Effect != canonical {
		return Result{Status: ResultDenied, Error: "effect classification mismatch"}, fmt.Errorf("effect classification mismatch")
	}
	if canonical.Class == EffectIrreversible {
		return Result{Status: ResultDenied, Error: "irreversible effect forbidden in speculative branch"}, fmt.Errorf("irreversible effect forbidden in speculative branch")
	}
	return w.inner.Execute(ctx, action)
}
func (w *SpeculativeTransactionalWorld) BranchRef() BranchRef { return w.inner.BranchRef() }
func (w *SpeculativeTransactionalWorld) PreparePromotion(ctx context.Context, operationID id.OperationID) (PromotionPlan, error) { return w.inner.PreparePromotion(ctx, operationID) }
func (w *SpeculativeTransactionalWorld) ApplyPromotion(ctx context.Context, plan PromotionPlan) error { return w.inner.ApplyPromotion(ctx, plan) }
func (w *SpeculativeTransactionalWorld) VerifyPromotion(ctx context.Context, plan PromotionPlan) error { return w.inner.VerifyPromotion(ctx, plan) }
func (w *SpeculativeTransactionalWorld) ReconcilePromotion(ctx context.Context, plan PromotionPlan) (PromotionStatus, error) { return w.inner.ReconcilePromotion(ctx, plan) }
func (w *SpeculativeTransactionalWorld) Rollback(ctx context.Context) error { return w.inner.Rollback(ctx) }
func (w *SpeculativeTransactionalWorld) Finalize(ctx context.Context) error { return w.inner.Finalize(ctx) }
func (w *SpeculativeTransactionalWorld) Close(ctx context.Context) error { return w.inner.Close(ctx) }
