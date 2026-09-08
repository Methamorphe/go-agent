package sqlite

import (
	"context"
	"fmt"

	"github.com/Methamorphe/go-agent/internal/id"
	agenttx "github.com/Methamorphe/go-agent/internal/transaction"
	"github.com/Methamorphe/go-agent/internal/world"
)

func (s *Store) ListEffects(ctx context.Context, transactionID id.TransactionID) ([]agenttx.EffectRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT effect_id, transaction_id, action_id, kind, effect_class, idempotent, retryable,
       state, outcome_certainty, error, created_at, updated_at
FROM agent_transaction_effects
WHERE transaction_id = ?
ORDER BY created_at, effect_id`, transactionID.String())
	if err != nil {
		return nil, fmt.Errorf("query transaction effects: %w", err)
	}
	defer rows.Close()

	result := make([]agenttx.EffectRecord, 0)
	for rows.Next() {
		var effect agenttx.EffectRecord
		var effectID, txID, actionID, effectClass, state, certainty, createdAt, updatedAt string
		var idempotent, retryable int
		if err := rows.Scan(
			&effectID,
			&txID,
			&actionID,
			&effect.Kind,
			&effectClass,
			&idempotent,
			&retryable,
			&state,
			&certainty,
			&effect.Error,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan transaction effect: %w", err)
		}
		effect.ID = id.EffectRecordID(effectID)
		effect.TransactionID = id.TransactionID(txID)
		effect.ActionID = id.ActionID(actionID)
		effect.Effect = world.Effect{
			Class:      world.EffectClass(effectClass),
			Idempotent: idempotent != 0,
			Retryable:  retryable != 0,
		}
		effect.State = agenttx.EffectState(state)
		effect.OutcomeCertainty = agenttx.OutcomeCertainty(certainty)
		effect.CreatedAt, err = parseTransactionTime(createdAt)
		if err != nil {
			return nil, err
		}
		effect.UpdatedAt, err = parseTransactionTime(updatedAt)
		if err != nil {
			return nil, err
		}
		result = append(result, effect)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate transaction effects: %w", err)
	}
	return result, nil
}
