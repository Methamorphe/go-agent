package world

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/Methamorphe/go-agent/internal/id"
)

var (
	ErrTransactionUnsupported = errors.New("world does not provide transactional isolation")
	ErrPromotionConflict       = errors.New("world promotion conflict")
	ErrPromotionUncertain      = errors.New("world promotion outcome uncertain")
)

type BranchRef struct {
	WorldID      id.WorldID      `json:"world_id"`
	Type         WorldType       `json:"type"`
	BaseIdentity string          `json:"base_identity"`
	Ref          string          `json:"ref"`
	Metadata     json.RawMessage `json:"metadata,omitempty"`
}

type PromotionPlan struct {
	OperationID   id.OperationID `json:"operation_id"`
	BaseIdentity  string         `json:"base_identity"`
	TargetBefore  string         `json:"target_before"`
	Source        string         `json:"source"`
	Merged        string         `json:"merged"`
	TargetChanged bool           `json:"target_changed"`
	Metadata      json.RawMessage `json:"metadata,omitempty"`
}

type PromotionStatus string

const (
	PromotionApplied    PromotionStatus = "applied"
	PromotionNotApplied PromotionStatus = "not_applied"
	PromotionUnknown    PromotionStatus = "unknown"
)

// TransactionalWorld is optional. Worlds must implement it only when they can
// honestly isolate mutations and reconcile promotion after a crash.
type TransactionalWorld interface {
	Executor
	BranchRef() BranchRef
	PreparePromotion(context.Context, id.OperationID) (PromotionPlan, error)
	ApplyPromotion(context.Context, PromotionPlan) error
	VerifyPromotion(context.Context, PromotionPlan) error
	ReconcilePromotion(context.Context, PromotionPlan) (PromotionStatus, error)
	Rollback(context.Context) error
	Close(context.Context) error
}
