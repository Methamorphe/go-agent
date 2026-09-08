package api

import (
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/transaction"
	"github.com/Methamorphe/go-agent/internal/world"
)

const TypeWorkspaceTransactionOperate = "workspace.transaction.operate"
const MessageWorkspaceTransactionOperate = TypeWorkspaceTransactionOperate

type WorkspaceTransactionOperation string

const (
	WorkspaceTransactionVerify    WorkspaceTransactionOperation = "verify"
	WorkspaceTransactionPrepare   WorkspaceTransactionOperation = "prepare"
	WorkspaceTransactionCommit    WorkspaceTransactionOperation = "commit"
	WorkspaceTransactionRollback  WorkspaceTransactionOperation = "rollback"
	WorkspaceTransactionReconcile WorkspaceTransactionOperation = "reconcile"
	WorkspaceTransactionResolve   WorkspaceTransactionOperation = "resolve_effect"
)

// WorkspaceTransactionOperateRequest is an explicit runtime operation. The TUI
// cannot manufacture transaction authority: all mutation and promotion remains
// inside the daemon's G7 manager and commit guard.
type WorkspaceTransactionOperateRequest struct {
	TransactionID id.TransactionID              `json:"transaction_id"`
	Operation     WorkspaceTransactionOperation `json:"operation"`
	Command       []string                      `json:"command,omitempty"`
	TimeoutMS     int                           `json:"timeout_ms,omitempty"`
	EffectID      id.EffectRecordID             `json:"effect_id,omitempty"`
	Certainty     transaction.OutcomeCertainty  `json:"certainty,omitempty"`
	Evidence      string                        `json:"evidence,omitempty"`
}

type WorkspaceTransactionOperateResponse struct {
	Transaction  transaction.Transaction `json:"transaction"`
	Verification *transaction.Verification `json:"verification,omitempty"`
	Promotion    *world.PromotionPlan       `json:"promotion,omitempty"`
}
