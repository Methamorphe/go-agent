package world

import (
	"context"
	"time"

	"github.com/Methamorphe/go-agent/internal/id"
)

// SecureTransactionalWorld composes the G3 authority gate with a real
// TransactionalWorld without making non-transactional Worlds advertise
// guarantees they do not provide.
type SecureTransactionalWorld struct {
	secure *SecureWorld
	inner  TransactionalWorld
}

func NewSecureTransactionalWorld(authorizer *Authorizer, inner TransactionalWorld, now func() time.Time) *SecureTransactionalWorld {
	if inner == nil {
		return nil
	}
	return &SecureTransactionalWorld{
		secure: NewSecureWorld(authorizer, inner, now),
		inner:  inner,
	}
}

func (w *SecureTransactionalWorld) Profile() Profile {
	return w.inner.Profile()
}

func (w *SecureTransactionalWorld) Execute(ctx context.Context, action Action) (Result, error) {
	return w.secure.Execute(ctx, action)
}

func (w *SecureTransactionalWorld) BranchRef() BranchRef {
	return w.inner.BranchRef()
}

func (w *SecureTransactionalWorld) PreparePromotion(ctx context.Context, operationID id.OperationID) (PromotionPlan, error) {
	return w.inner.PreparePromotion(ctx, operationID)
}

func (w *SecureTransactionalWorld) ApplyPromotion(ctx context.Context, plan PromotionPlan) error {
	return w.inner.ApplyPromotion(ctx, plan)
}

func (w *SecureTransactionalWorld) VerifyPromotion(ctx context.Context, plan PromotionPlan) error {
	return w.inner.VerifyPromotion(ctx, plan)
}

func (w *SecureTransactionalWorld) ReconcilePromotion(ctx context.Context, plan PromotionPlan) (PromotionStatus, error) {
	return w.inner.ReconcilePromotion(ctx, plan)
}

func (w *SecureTransactionalWorld) Rollback(ctx context.Context) error {
	return w.inner.Rollback(ctx)
}

func (w *SecureTransactionalWorld) Finalize(ctx context.Context) error {
	return w.inner.Finalize(ctx)
}

func (w *SecureTransactionalWorld) Close(ctx context.Context) error {
	return w.inner.Close(ctx)
}
