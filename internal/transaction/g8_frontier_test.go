package transaction

import (
	"context"
	"errors"
	"testing"

	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/world"
)

func TestG8ForkableCheckpointBlockedByOpenTransaction(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	manager := NewManager(store, nil, nil)
	branch := newFakeTransactionalWorld()
	tx := beginTestTransaction(t, manager, store, branch)

	frontier, err := manager.ExecutionFrontier(ctx, tx.AgentID)
	if err != nil { t.Fatal(err) }
	if len(frontier.TransactionRefs) != 1 || frontier.TransactionRefs[0] != tx.ID { t.Fatalf("transaction refs=%v", frontier.TransactionRefs) }
	if err := manager.RequireExecutionEditQuiescence(ctx, tx.AgentID); !errors.Is(err, ErrInvalidState) { t.Fatalf("quiescence err=%v want invalid state", err) }

	if _, err := manager.Rollback(ctx, tx.ID, branch); err != nil { t.Fatal(err) }
	if err := manager.RequireExecutionEditQuiescence(ctx, tx.AgentID); err != nil { t.Fatalf("terminal transaction still blocked checkpoint: %v", err) }
}

func TestG8UnknownMutationOutcomeBlocksExecutionEdit(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	manager := NewManager(store, nil, nil)
	branch := newFakeTransactionalWorld()
	branch.executeErr = errors.New("transport lost after dispatch")
	tx := beginTestTransaction(t, manager, store, branch)
	action := world.Action{ID: id.ActionID("act_g8_unknown"), AgentID: tx.AgentID, Kind: "fs.write_file", Purpose: "speculative edit", Resource: "x.txt", Effect: world.CanonicalEffect("fs.write_file")}
	if _, err := manager.Execute(ctx, tx.ID, branch, action); !errors.Is(err, ErrReconciliationRequired) { t.Fatalf("execute err=%v", err) }

	frontier, err := manager.ExecutionFrontier(ctx, tx.AgentID)
	if err != nil { t.Fatal(err) }
	if len(frontier.UnknownOutcomeActions) != 1 || frontier.UnknownOutcomeActions[0] != action.ID { t.Fatalf("unknown actions=%v", frontier.UnknownOutcomeActions) }
	if err := manager.RequireExecutionEditQuiescence(ctx, tx.AgentID); !errors.Is(err, ErrReconciliationRequired) { t.Fatalf("quiescence err=%v want reconciliation", err) }
}
