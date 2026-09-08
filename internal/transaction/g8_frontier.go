package transaction

import (
	"context"
	"fmt"
	"sort"

	"github.com/Methamorphe/go-agent/internal/id"
)

// ExecutionFrontier exposes transaction/effect obligations relevant to safe
// execution editing. G8 deliberately treats any non-terminal transaction as a
// quiescence blocker in v0; this is stronger than attempting to transfer
// ownership of partially staged work to a fork.
type ExecutionFrontier struct {
	AgentID              id.AgentID        `json:"agent_id"`
	TransactionRefs      []id.TransactionID `json:"transaction_refs,omitempty"`
	UnknownOutcomeActions []id.ActionID      `json:"unknown_outcome_actions,omitempty"`
}

func (m *Manager) ExecutionFrontier(ctx context.Context, agentID id.AgentID) (ExecutionFrontier, error) {
	if m == nil || m.store == nil || agentID == "" { return ExecutionFrontier{}, fmt.Errorf("transaction manager and agent id are required") }
	states := []State{StateCreating, StateOpen, StateVerifying, StateReadyToCommit, StateCommitting, StateRollingBack, StateNeedsReconciliation}
	txs, err := m.store.ListByStates(ctx, states...)
	if err != nil { return ExecutionFrontier{}, err }
	frontier := ExecutionFrontier{AgentID: agentID}
	for _, tx := range txs {
		if tx.AgentID != agentID { continue }
		frontier.TransactionRefs = append(frontier.TransactionRefs, tx.ID)
		effects, err := m.store.ListEffects(ctx, tx.ID)
		if err != nil { return ExecutionFrontier{}, err }
		for _, effect := range effects {
			if effect.OutcomeCertainty == OutcomeUnknown || effect.State == EffectDispatched || effect.State == EffectOutcomeUnknown {
				frontier.UnknownOutcomeActions = append(frontier.UnknownOutcomeActions, effect.ActionID)
			}
		}
	}
	sort.Slice(frontier.TransactionRefs, func(i, j int) bool { return frontier.TransactionRefs[i].String() < frontier.TransactionRefs[j].String() })
	sort.Slice(frontier.UnknownOutcomeActions, func(i, j int) bool { return frontier.UnknownOutcomeActions[i].String() < frontier.UnknownOutcomeActions[j].String() })
	return frontier, nil
}

func (m *Manager) RequireExecutionEditQuiescence(ctx context.Context, agentID id.AgentID) error {
	frontier, err := m.ExecutionFrontier(ctx, agentID)
	if err != nil { return err }
	if len(frontier.UnknownOutcomeActions) > 0 {
		return fmt.Errorf("%w: %d unresolved transaction outcomes", ErrReconciliationRequired, len(frontier.UnknownOutcomeActions))
	}
	if len(frontier.TransactionRefs) > 0 {
		return fmt.Errorf("%w: %d non-terminal transactions block forkable checkpoint", ErrInvalidState, len(frontier.TransactionRefs))
	}
	return nil
}
