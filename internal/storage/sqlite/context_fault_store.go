package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/mmu"
)

func (s *Store) PutContextFault(ctx context.Context, record mmu.FaultRecord) error {
	if record.Request.ID == "" || record.Request.AgentID == "" || record.Request.InvocationID == "" {
		return errs.New(errs.CodeInvalidArgument, "sqlite.context_fault.put", "fault, agent and source invocation ids are required")
	}
	body, err := json.Marshal(record)
	if err != nil {
		return errs.Wrap(errs.CodeInvalidArgument, "sqlite.context_fault.put", "encode fault record", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return errs.Wrap(errs.CodeUnavailable, "sqlite.context_fault.put", "begin transaction", err)
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
INSERT INTO context_faults (fault_id, agent_id, source_invocation_id, state, record_json, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(fault_id) DO UPDATE SET
    state = excluded.state,
    record_json = excluded.record_json,
    updated_at = excluded.updated_at`,
		record.Request.ID.String(), record.Request.AgentID.String(), record.Request.InvocationID.String(), string(record.State), string(body), formatTime(record.CreatedAt), formatTime(record.UpdatedAt))
	if err != nil {
		return errs.Wrap(errs.CodeUnavailable, "sqlite.context_fault.put", "persist fault record", err)
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO context_fault_events (fault_id, state, record_json, recorded_at)
VALUES (?, ?, ?, ?)`, record.Request.ID.String(), string(record.State), string(body), formatTime(record.UpdatedAt))
	if err != nil {
		return errs.Wrap(errs.CodeUnavailable, "sqlite.context_fault.put", "append fault lifecycle event", err)
	}
	if err := tx.Commit(); err != nil {
		return errs.Wrap(errs.CodeUnavailable, "sqlite.context_fault.put", "commit transaction", err)
	}
	return nil
}

func (s *Store) PendingResolvedContextFaults(ctx context.Context, agentID id.AgentID) ([]mmu.FaultRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT record_json FROM context_faults
WHERE agent_id = ? AND state = ? AND manifested_invocation_id IS NULL
ORDER BY updated_at, fault_id`, agentID.String(), string(mmu.FaultResolved))
	if err != nil {
		return nil, errs.Wrap(errs.CodeUnavailable, "sqlite.context_fault.pending", "query pending faults", err)
	}
	defer rows.Close()
	var result []mmu.FaultRecord
	for rows.Next() {
		var body string
		if err := rows.Scan(&body); err != nil {
			return nil, errs.Wrap(errs.CodeUnavailable, "sqlite.context_fault.pending", "scan fault", err)
		}
		var record mmu.FaultRecord
		if err := json.Unmarshal([]byte(body), &record); err != nil {
			return nil, errs.Wrap(errs.CodeCorruption, "sqlite.context_fault.pending", "decode fault record", err)
		}
		result = append(result, record)
	}
	if err := rows.Err(); err != nil {
		return nil, errs.Wrap(errs.CodeUnavailable, "sqlite.context_fault.pending", "iterate faults", err)
	}
	return result, nil
}

func (s *Store) MarkContextFaultsManifested(ctx context.Context, faultIDs []id.ContextFaultID, invocationID id.InvocationID) error {
	if len(faultIDs) == 0 {
		return nil
	}
	if invocationID == "" {
		return errs.New(errs.CodeInvalidArgument, "sqlite.context_fault.manifest", "invocation id is required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return errs.Wrap(errs.CodeUnavailable, "sqlite.context_fault.manifest", "begin transaction", err)
	}
	defer tx.Rollback()
	for _, faultID := range faultIDs {
		result, execErr := tx.ExecContext(ctx, `UPDATE context_faults SET manifested_invocation_id = ? WHERE fault_id = ? AND manifested_invocation_id IS NULL`, invocationID.String(), faultID.String())
		if execErr != nil {
			return errs.Wrap(errs.CodeUnavailable, "sqlite.context_fault.manifest", "mark fault manifested", execErr)
		}
		if affected, rowsErr := result.RowsAffected(); rowsErr == nil && affected == 0 {
			var existing sql.NullString
			queryErr := tx.QueryRowContext(ctx, `SELECT manifested_invocation_id FROM context_faults WHERE fault_id = ?`, faultID.String()).Scan(&existing)
			if errors.Is(queryErr, sql.ErrNoRows) {
				return errs.New(errs.CodeNotFound, "sqlite.context_fault.manifest", "context fault not found")
			}
			if queryErr != nil {
				return errs.Wrap(errs.CodeUnavailable, "sqlite.context_fault.manifest", "inspect manifested fault", queryErr)
			}
			if existing.Valid && existing.String != invocationID.String() {
				return errs.New(errs.CodeConflict, "sqlite.context_fault.manifest", "fault already belongs to another manifest")
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return errs.Wrap(errs.CodeUnavailable, "sqlite.context_fault.manifest", "commit transaction", err)
	}
	return nil
}

var _ mmu.FaultJournal = (*Store)(nil)
