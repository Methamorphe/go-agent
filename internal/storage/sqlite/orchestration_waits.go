package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/orchestration"
)

func (s *Store) PutWait(ctx context.Context, wait orchestration.WaitSpec) error {
	children, err := orchestrationJSON(wait.Children)
	if err != nil { return err }
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil { return err }
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `
INSERT INTO orchestration_waits(wait_id, parent_agent_id, mode, quorum, failure_policy, children_json, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`, wait.ID.String(), wait.ParentID.String(), string(wait.Mode), wait.Quorum, string(wait.Failure), children, formatTime(wait.CreatedAt)); err != nil { return err }
	for _, childID := range wait.Children {
		if _, err = tx.ExecContext(ctx, `INSERT INTO orchestration_wait_edges(wait_id, from_agent_id, to_agent_id) VALUES (?, ?, ?)`, wait.ID.String(), wait.ParentID.String(), childID.String()); err != nil { return err }
	}
	return tx.Commit()
}

func (s *Store) DeleteWait(ctx context.Context, waitID id.WaitID) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM orchestration_waits WHERE wait_id = ?`, waitID.String())
	return err
}

func (s *Store) WaitEdges(ctx context.Context, from id.AgentID) ([]id.AgentID, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT to_agent_id FROM orchestration_wait_edges WHERE from_agent_id = ? ORDER BY to_agent_id`, from.String())
	if err != nil { return nil, err }
	defer rows.Close()
	var result []id.AgentID
	for rows.Next() { var value string; if err := rows.Scan(&value); err != nil { return nil, err }; result = append(result, id.AgentID(value)) }
	return result, rows.Err()
}

func (s *Store) WaitsForChild(ctx context.Context, child id.AgentID) ([]orchestration.WaitSpec, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT w.wait_id, w.parent_agent_id, w.mode, w.quorum, w.failure_policy, w.children_json, w.created_at
FROM orchestration_waits w JOIN orchestration_wait_edges e ON e.wait_id = w.wait_id
WHERE e.to_agent_id = ? ORDER BY w.created_at`, child.String())
	if err != nil { return nil, err }
	defer rows.Close()
	var result []orchestration.WaitSpec
	for rows.Next() {
		var wait orchestration.WaitSpec
		var waitID, parentID, mode, failure, childrenJSON, createdAt string
		if err := rows.Scan(&waitID, &parentID, &mode, &wait.Quorum, &failure, &childrenJSON, &createdAt); err != nil { return nil, err }
		wait.ID = id.WaitID(waitID)
		wait.ParentID = id.AgentID(parentID)
		wait.Mode = orchestration.WaitMode(mode)
		wait.Failure = orchestration.FailurePolicy(failure)
		if err := json.Unmarshal([]byte(childrenJSON), &wait.Children); err != nil { return nil, err }
		wait.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
		if err != nil { return nil, err }
		result = append(result, wait)
	}
	return result, rows.Err()
}

func (s *Store) PutResult(ctx context.Context, result orchestration.ChildResult) error {
	body, err := orchestrationJSON(result)
	if err != nil { return err }
	_, err = s.db.ExecContext(ctx, `
INSERT INTO orchestration_results(child_agent_id, result_json, completed_at)
VALUES (?, ?, ?) ON CONFLICT(child_agent_id) DO NOTHING`, result.ChildAgentID.String(), body, formatTime(result.CompletedAt))
	return err
}

func (s *Store) Result(ctx context.Context, child id.AgentID) (orchestration.ChildResult, bool, error) {
	var body string
	err := s.db.QueryRowContext(ctx, `SELECT result_json FROM orchestration_results WHERE child_agent_id = ?`, child.String()).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) { return orchestration.ChildResult{}, false, nil }
	if err != nil { return orchestration.ChildResult{}, false, err }
	var result orchestration.ChildResult
	if err := json.Unmarshal([]byte(body), &result); err != nil { return orchestration.ChildResult{}, false, err }
	return result, true, nil
}
