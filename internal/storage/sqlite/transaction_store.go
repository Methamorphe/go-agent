package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Methamorphe/go-agent/internal/id"
	agenttx "github.com/Methamorphe/go-agent/internal/transaction"
	"github.com/Methamorphe/go-agent/internal/world"
)

var _ agenttx.Store = (*Store)(nil)

func (s *Store) Create(ctx context.Context, transaction agenttx.Transaction, eventType string, eventPayload json.RawMessage) error {
	branchJSON, err := json.Marshal(transaction.IsolatedWorldRef)
	if err != nil {
		return fmt.Errorf("encode transaction branch reference: %w", err)
	}
	planJSON, err := marshalPromotionPlan(transaction.PreparedPlan)
	if err != nil {
		return err
	}
	dbtx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return fmt.Errorf("begin agent transaction create: %w", err)
	}
	defer dbtx.Rollback()
	_, err = dbtx.ExecContext(ctx, `
INSERT INTO agent_transactions(transaction_id, agent_id, base_checkpoint_id, world_id, branch_ref_json, state, version, commit_policy, prepared_plan_json, reconcile_reason, created_at, updated_at)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		transaction.ID.String(), transaction.AgentID.String(), transaction.BaseCheckpoint.String(), transaction.WorldID.String(), branchJSON,
		string(transaction.State), transaction.Version, string(transaction.CommitPolicy), planJSON, transaction.ReconcileReason,
		formatTransactionTime(transaction.CreatedAt), formatTransactionTime(transaction.UpdatedAt))
	if err != nil {
		return fmt.Errorf("insert agent transaction: %w", err)
	}
	if err := appendAgentTransactionEvent(ctx, dbtx, transaction.ID, transaction.Version, eventType, eventPayload, transaction.UpdatedAt); err != nil {
		return err
	}
	if err := dbtx.Commit(); err != nil {
		return fmt.Errorf("commit agent transaction create: %w", err)
	}
	return nil
}

func (s *Store) Get(ctx context.Context, transactionID id.TransactionID) (agenttx.Transaction, error) {
	return loadAgentTransaction(s.db.QueryRowContext(ctx, `
SELECT transaction_id, agent_id, base_checkpoint_id, world_id, branch_ref_json, state, version, commit_policy, prepared_plan_json, reconcile_reason, created_at, updated_at
FROM agent_transactions WHERE transaction_id = ?`, transactionID.String()))
}

func (s *Store) Transition(ctx context.Context, transactionID id.TransactionID, expectedVersion uint64, expectedStates []agenttx.State, newState agenttx.State, plan *world.PromotionPlan, reconcileReason, eventType string, eventPayload json.RawMessage) (agenttx.Transaction, error) {
	dbtx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return agenttx.Transaction{}, fmt.Errorf("begin agent transaction transition: %w", err)
	}
	defer dbtx.Rollback()
	current, err := loadAgentTransaction(dbtx.QueryRowContext(ctx, `
SELECT transaction_id, agent_id, base_checkpoint_id, world_id, branch_ref_json, state, version, commit_policy, prepared_plan_json, reconcile_reason, created_at, updated_at
FROM agent_transactions WHERE transaction_id = ?`, transactionID.String()))
	if err != nil {
		return agenttx.Transaction{}, err
	}
	if current.Version != expectedVersion || !stateAllowed(expectedStates, current.State) {
		return agenttx.Transaction{}, agenttx.ErrConflict
	}
	planJSON, err := marshalPromotionPlan(plan)
	if err != nil {
		return agenttx.Transaction{}, err
	}
	updatedAt := time.Now().UTC()
	newVersion := current.Version + 1
	result, err := dbtx.ExecContext(ctx, `
UPDATE agent_transactions SET state = ?, version = ?, prepared_plan_json = ?, reconcile_reason = ?, updated_at = ?
WHERE transaction_id = ? AND version = ? AND state = ?`, string(newState), newVersion, planJSON, reconcileReason, formatTransactionTime(updatedAt), transactionID.String(), current.Version, string(current.State))
	if err != nil {
		return agenttx.Transaction{}, fmt.Errorf("update agent transaction: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return agenttx.Transaction{}, fmt.Errorf("read transaction transition result: %w", err)
	}
	if rows != 1 {
		return agenttx.Transaction{}, agenttx.ErrConflict
	}
	if err := appendAgentTransactionEvent(ctx, dbtx, transactionID, newVersion, eventType, eventPayload, updatedAt); err != nil {
		return agenttx.Transaction{}, err
	}
	if err := dbtx.Commit(); err != nil {
		return agenttx.Transaction{}, fmt.Errorf("commit transaction transition: %w", err)
	}
	current.State, current.Version, current.PreparedPlan, current.ReconcileReason, current.UpdatedAt = newState, newVersion, clonePromotionPlan(plan), reconcileReason, updatedAt
	return current, nil
}

func (s *Store) CreateEffect(ctx context.Context, effect agenttx.EffectRecord, eventType string, eventPayload json.RawMessage) error {
	dbtx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return fmt.Errorf("begin effect create: %w", err)
	}
	defer dbtx.Rollback()
	_, err = dbtx.ExecContext(ctx, `
INSERT INTO agent_transaction_effects(effect_id, transaction_id, action_id, kind, effect_class, idempotent, retryable, state, outcome_certainty, error, created_at, updated_at)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, effect.ID.String(), effect.TransactionID.String(), effect.ActionID.String(), effect.Kind, string(effect.Effect.Class), transactionBoolInt(effect.Effect.Idempotent), transactionBoolInt(effect.Effect.Retryable), string(effect.State), string(effect.OutcomeCertainty), effect.Error, formatTransactionTime(effect.CreatedAt), formatTransactionTime(effect.UpdatedAt))
	if err != nil {
		return fmt.Errorf("insert transaction effect: %w", err)
	}
	version, err := transactionVersion(ctx, dbtx, effect.TransactionID)
	if err != nil {
		return err
	}
	if err := appendAgentTransactionEvent(ctx, dbtx, effect.TransactionID, version, eventType, eventPayload, effect.UpdatedAt); err != nil {
		return err
	}
	if err := dbtx.Commit(); err != nil {
		return fmt.Errorf("commit transaction effect: %w", err)
	}
	return nil
}

func (s *Store) UpdateEffect(ctx context.Context, effectID id.EffectRecordID, state agenttx.EffectState, certainty agenttx.OutcomeCertainty, effectError, eventType string, eventPayload json.RawMessage) error {
	dbtx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return fmt.Errorf("begin effect update: %w", err)
	}
	defer dbtx.Rollback()
	var transactionID string
	err = dbtx.QueryRowContext(ctx, `SELECT transaction_id FROM agent_transaction_effects WHERE effect_id = ?`, effectID.String()).Scan(&transactionID)
	if errors.Is(err, sql.ErrNoRows) {
		return agenttx.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("load transaction effect: %w", err)
	}
	updatedAt := time.Now().UTC()
	result, err := dbtx.ExecContext(ctx, `UPDATE agent_transaction_effects SET state = ?, outcome_certainty = ?, error = ?, updated_at = ? WHERE effect_id = ?`, string(state), string(certainty), effectError, formatTransactionTime(updatedAt), effectID.String())
	if err != nil {
		return fmt.Errorf("update transaction effect: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read effect update result: %w", err)
	}
	if rows != 1 {
		return agenttx.ErrNotFound
	}
	txID := id.TransactionID(transactionID)
	version, err := transactionVersion(ctx, dbtx, txID)
	if err != nil {
		return err
	}
	if err := appendAgentTransactionEvent(ctx, dbtx, txID, version, eventType, eventPayload, updatedAt); err != nil {
		return err
	}
	if err := dbtx.Commit(); err != nil {
		return fmt.Errorf("commit transaction effect update: %w", err)
	}
	return nil
}

func (s *Store) CreateVerification(ctx context.Context, verification agenttx.Verification, eventType string, eventPayload json.RawMessage) error {
	body, err := json.Marshal(verification)
	if err != nil {
		return fmt.Errorf("encode transaction verification: %w", err)
	}
	dbtx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return fmt.Errorf("begin verification create: %w", err)
	}
	defer dbtx.Rollback()
	var completed any
	if verification.CompletedAt != nil {
		completed = formatTransactionTime(*verification.CompletedAt)
	}
	_, err = dbtx.ExecContext(ctx, `INSERT INTO agent_transaction_verifications(verification_id, transaction_id, status, verification_json, started_at, completed_at) VALUES(?, ?, ?, ?, ?, ?)`, verification.ID.String(), verification.TransactionID.String(), string(verification.Status), body, formatTransactionTime(verification.StartedAt), completed)
	if err != nil {
		return fmt.Errorf("insert transaction verification: %w", err)
	}
	version, err := transactionVersion(ctx, dbtx, verification.TransactionID)
	if err != nil {
		return err
	}
	at := verification.StartedAt
	if verification.CompletedAt != nil {
		at = *verification.CompletedAt
	}
	if err := appendAgentTransactionEvent(ctx, dbtx, verification.TransactionID, version, eventType, eventPayload, at); err != nil {
		return err
	}
	if err := dbtx.Commit(); err != nil {
		return fmt.Errorf("commit transaction verification: %w", err)
	}
	return nil
}

func (s *Store) TransactionEvents(ctx context.Context, transactionID id.TransactionID) ([]agenttx.Event, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT sequence, transaction_id, transaction_version, event_type, payload_json, created_at FROM agent_transaction_events WHERE transaction_id = ? ORDER BY sequence`, transactionID.String())
	if err != nil {
		return nil, fmt.Errorf("query transaction events: %w", err)
	}
	defer rows.Close()
	var events []agenttx.Event
	for rows.Next() {
		var event agenttx.Event
		var transaction, created string
		var body []byte
		if err := rows.Scan(&event.Sequence, &transaction, &event.TransactionVersion, &event.Type, &body, &created); err != nil {
			return nil, fmt.Errorf("scan transaction event: %w", err)
		}
		event.TransactionID, event.Payload = id.TransactionID(transaction), append(json.RawMessage(nil), body...)
		event.CreatedAt, err = parseTransactionTime(created)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (s *Store) ListByStates(ctx context.Context, states ...agenttx.State) ([]agenttx.Transaction, error) {
	if len(states) == 0 {
		return nil, nil
	}
	marks, args := make([]string, len(states)), make([]any, len(states))
	for i, state := range states {
		marks[i], args[i] = "?", string(state)
	}
	query := `SELECT transaction_id, agent_id, base_checkpoint_id, world_id, branch_ref_json, state, version, commit_policy, prepared_plan_json, reconcile_reason, created_at, updated_at FROM agent_transactions WHERE state IN (` + strings.Join(marks, ",") + `) ORDER BY created_at, transaction_id`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query transactions by state: %w", err)
	}
	defer rows.Close()
	var result []agenttx.Transaction
	for rows.Next() {
		transaction, err := scanAgentTransaction(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, transaction)
	}
	return result, rows.Err()
}

type transactionRowScanner interface {
	Scan(...any) error
}

func loadAgentTransaction(row transactionRowScanner) (agenttx.Transaction, error) {
	transaction, err := scanAgentTransaction(row)
	if errors.Is(err, sql.ErrNoRows) {
		return agenttx.Transaction{}, agenttx.ErrNotFound
	}
	return transaction, err
}

func scanAgentTransaction(row transactionRowScanner) (agenttx.Transaction, error) {
	var transaction agenttx.Transaction
	var transactionID, agentID, checkpointID, worldID, state, commitPolicy, createdAt, updatedAt string
	var branchJSON, planJSON []byte
	if err := row.Scan(&transactionID, &agentID, &checkpointID, &worldID, &branchJSON, &state, &transaction.Version, &commitPolicy, &planJSON, &transaction.ReconcileReason, &createdAt, &updatedAt); err != nil {
		return agenttx.Transaction{}, err
	}
	transaction.ID, transaction.AgentID, transaction.BaseCheckpoint, transaction.WorldID = id.TransactionID(transactionID), id.AgentID(agentID), id.CheckpointID(checkpointID), id.WorldID(worldID)
	transaction.State, transaction.CommitPolicy = agenttx.State(state), agenttx.CommitPolicy(commitPolicy)
	if err := json.Unmarshal(branchJSON, &transaction.IsolatedWorldRef); err != nil {
		return agenttx.Transaction{}, fmt.Errorf("decode transaction branch reference: %w", err)
	}
	if len(planJSON) > 0 {
		var plan world.PromotionPlan
		if err := json.Unmarshal(planJSON, &plan); err != nil {
			return agenttx.Transaction{}, fmt.Errorf("decode transaction promotion plan: %w", err)
		}
		transaction.PreparedPlan = &plan
	}
	var err error
	transaction.CreatedAt, err = parseTransactionTime(createdAt)
	if err != nil {
		return agenttx.Transaction{}, err
	}
	transaction.UpdatedAt, err = parseTransactionTime(updatedAt)
	if err != nil {
		return agenttx.Transaction{}, err
	}
	return transaction, nil
}

func transactionVersion(ctx context.Context, tx *sql.Tx, transactionID id.TransactionID) (uint64, error) {
	var version uint64
	err := tx.QueryRowContext(ctx, `SELECT version FROM agent_transactions WHERE transaction_id = ?`, transactionID.String()).Scan(&version)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, agenttx.ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("load transaction version: %w", err)
	}
	return version, nil
}

func appendAgentTransactionEvent(ctx context.Context, tx *sql.Tx, transactionID id.TransactionID, version uint64, eventType string, eventPayload json.RawMessage, at time.Time) error {
	if eventType == "" {
		return fmt.Errorf("transaction event type is required")
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO agent_transaction_events(transaction_id, transaction_version, event_type, payload_json, created_at) VALUES(?, ?, ?, ?, ?)`, transactionID.String(), version, eventType, []byte(eventPayload), formatTransactionTime(at))
	if err != nil {
		return fmt.Errorf("append transaction event: %w", err)
	}
	return nil
}

func marshalPromotionPlan(plan *world.PromotionPlan) ([]byte, error) {
	if plan == nil {
		return nil, nil
	}
	body, err := json.Marshal(plan)
	if err != nil {
		return nil, fmt.Errorf("encode transaction promotion plan: %w", err)
	}
	return body, nil
}

func clonePromotionPlan(plan *world.PromotionPlan) *world.PromotionPlan {
	if plan == nil {
		return nil
	}
	copy := *plan
	copy.Metadata = append(json.RawMessage(nil), plan.Metadata...)
	return &copy
}

func stateAllowed(states []agenttx.State, state agenttx.State) bool {
	for _, candidate := range states {
		if candidate == state {
			return true
		}
	}
	return false
}

func transactionBoolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func formatTransactionTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func parseTransactionTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse transaction time: %w", err)
	}
	return parsed.UTC(), nil
}
