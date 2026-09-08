package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/memory"
)

func (s *Store) CreatePropagationJob(ctx context.Context, job memory.PropagationJob, items []memory.PropagationItem) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return errs.Wrap(errs.CodeUnavailable, "sqlite.memory.propagation.create", "begin propagation transaction", err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
INSERT OR IGNORE INTO memory_propagation_jobs (
    job_id, cause, state, max_depth, max_work, processed_count, created_at, updated_at, last_error
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		job.ID.String(), job.Cause, string(job.State), job.MaxDepth, job.MaxWork, job.ProcessedCount,
		formatTime(job.CreatedAt), formatTime(job.UpdatedAt), job.LastError,
	)
	if err != nil {
		return errs.Wrap(errs.CodeUnavailable, "sqlite.memory.propagation.create", "insert propagation job", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return errs.Wrap(errs.CodeInternal, "sqlite.memory.propagation.create", "read propagation insert result", err)
	}
	if changed != 1 {
		return memory.ErrConflict
	}
	for _, item := range items {
		if _, err := tx.ExecContext(ctx, `
INSERT OR IGNORE INTO memory_propagation_queue (
    job_id, node_kind, node_id, depth, processed, enqueued_at, processed_at
) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			job.ID.String(), string(item.Kind), item.NodeID, item.Depth, boolInt(item.Processed), formatTime(item.EnqueuedAt), formatOptionalTime(item.ProcessedAt),
		); err != nil {
			return errs.Wrap(errs.CodeUnavailable, "sqlite.memory.propagation.create", "insert propagation root", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return errs.Wrap(errs.CodeUnavailable, "sqlite.memory.propagation.create", "commit propagation job", err)
	}
	return nil
}

func (s *Store) PropagationJob(ctx context.Context, jobID id.PropagationID) (memory.PropagationJob, error) {
	var job memory.PropagationJob
	var rawID, state, created, updated string
	err := s.db.QueryRowContext(ctx, `
SELECT job_id, cause, state, max_depth, max_work, processed_count, created_at, updated_at, last_error
FROM memory_propagation_jobs WHERE job_id = ?`, jobID.String()).Scan(
		&rawID, &job.Cause, &state, &job.MaxDepth, &job.MaxWork, &job.ProcessedCount, &created, &updated, &job.LastError,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return memory.PropagationJob{}, memory.ErrNotFound
	}
	if err != nil {
		return memory.PropagationJob{}, errs.Wrap(errs.CodeUnavailable, "sqlite.memory.propagation.get", "query propagation job", err)
	}
	job.ID = id.PropagationID(rawID)
	job.State = memory.PropagationState(state)
	job.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return memory.PropagationJob{}, errs.Wrap(errs.CodeCorruption, "sqlite.memory.propagation.get", "parse created timestamp", err)
	}
	job.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated)
	if err != nil {
		return memory.PropagationJob{}, errs.Wrap(errs.CodeCorruption, "sqlite.memory.propagation.get", "parse updated timestamp", err)
	}
	return job, nil
}

func (s *Store) PendingPropagationItems(ctx context.Context, jobID id.PropagationID, limit int) ([]memory.PropagationItem, error) {
	if limit <= 0 {
		limit = 128
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT job_id, node_kind, node_id, depth, processed, enqueued_at, processed_at
FROM memory_propagation_queue
WHERE job_id = ? AND processed = 0
ORDER BY depth, enqueued_at, node_kind, node_id
LIMIT ?`, jobID.String(), limit)
	if err != nil {
		return nil, errs.Wrap(errs.CodeUnavailable, "sqlite.memory.propagation.pending", "query pending propagation items", err)
	}
	defer rows.Close()
	out := make([]memory.PropagationItem, 0)
	for rows.Next() {
		value, scanErr := scanPropagationItem(rows)
		if scanErr != nil {
			return nil, errs.Wrap(errs.CodeCorruption, "sqlite.memory.propagation.pending", "scan propagation item", scanErr)
		}
		out = append(out, value)
	}
	return out, rows.Err()
}

func (s *Store) EnqueuePropagationItems(ctx context.Context, items []memory.PropagationItem) error {
	if len(items) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return errs.Wrap(errs.CodeUnavailable, "sqlite.memory.propagation.enqueue", "begin enqueue transaction", err)
	}
	defer tx.Rollback()
	for _, item := range items {
		if _, err := tx.ExecContext(ctx, `
INSERT OR IGNORE INTO memory_propagation_queue (
    job_id, node_kind, node_id, depth, processed, enqueued_at, processed_at
) VALUES (?, ?, ?, ?, 0, ?, NULL)`,
			item.JobID.String(), string(item.Kind), item.NodeID, item.Depth, formatTime(item.EnqueuedAt),
		); err != nil {
			return errs.Wrap(errs.CodeUnavailable, "sqlite.memory.propagation.enqueue", "insert propagation item", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return errs.Wrap(errs.CodeUnavailable, "sqlite.memory.propagation.enqueue", "commit propagation items", err)
	}
	return nil
}

func (s *Store) MarkPropagationItemProcessed(ctx context.Context, jobID id.PropagationID, kind memory.PropagationNodeKind, nodeID string, at time.Time) error {
	result, err := s.db.ExecContext(ctx, `
UPDATE memory_propagation_queue
SET processed = 1, processed_at = ?
WHERE job_id = ? AND node_kind = ? AND node_id = ? AND processed = 0`,
		formatTime(at), jobID.String(), string(kind), nodeID,
	)
	if err != nil {
		return errs.Wrap(errs.CodeUnavailable, "sqlite.memory.propagation.processed", "mark propagation item processed", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return errs.Wrap(errs.CodeInternal, "sqlite.memory.propagation.processed", "read processed result", err)
	}
	if changed == 0 {
		var processed int
		checkErr := s.db.QueryRowContext(ctx, `
SELECT processed FROM memory_propagation_queue
WHERE job_id = ? AND node_kind = ? AND node_id = ?`, jobID.String(), string(kind), nodeID).Scan(&processed)
		if errors.Is(checkErr, sql.ErrNoRows) {
			return memory.ErrNotFound
		}
		if checkErr != nil {
			return errs.Wrap(errs.CodeUnavailable, "sqlite.memory.propagation.processed", "check propagation item", checkErr)
		}
	}
	return nil
}

func (s *Store) UpdatePropagationJob(ctx context.Context, job memory.PropagationJob) error {
	result, err := s.db.ExecContext(ctx, `
UPDATE memory_propagation_jobs
SET cause = ?, state = ?, max_depth = ?, max_work = ?, processed_count = ?, updated_at = ?, last_error = ?
WHERE job_id = ?`,
		job.Cause, string(job.State), job.MaxDepth, job.MaxWork, job.ProcessedCount,
		formatTime(job.UpdatedAt), job.LastError, job.ID.String(),
	)
	if err != nil {
		return errs.Wrap(errs.CodeUnavailable, "sqlite.memory.propagation.update", "update propagation job", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return errs.Wrap(errs.CodeInternal, "sqlite.memory.propagation.update", "read propagation update result", err)
	}
	if changed != 1 {
		return memory.ErrNotFound
	}
	return nil
}

func scanPropagationItem(scanner rowScanner) (memory.PropagationItem, error) {
	var value memory.PropagationItem
	var jobID, kind, enqueued string
	var processed int
	var processedAt sql.NullString
	if err := scanner.Scan(&jobID, &kind, &value.NodeID, &value.Depth, &processed, &enqueued, &processedAt); err != nil {
		return memory.PropagationItem{}, err
	}
	value.JobID = id.PropagationID(jobID)
	value.Kind = memory.PropagationNodeKind(kind)
	value.Processed = processed != 0
	var err error
	value.EnqueuedAt, err = time.Parse(time.RFC3339Nano, enqueued)
	if err != nil {
		return memory.PropagationItem{}, err
	}
	if processedAt.Valid {
		t, parseErr := time.Parse(time.RFC3339Nano, processedAt.String)
		if parseErr != nil {
			return memory.PropagationItem{}, parseErr
		}
		value.ProcessedAt = &t
	}
	return value, nil
}

var _ memory.Repository = (*Store)(nil)
