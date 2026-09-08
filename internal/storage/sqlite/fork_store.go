package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	cogfork "github.com/Methamorphe/go-agent/internal/fork"
	"github.com/Methamorphe/go-agent/internal/id"
)

var _ cogfork.Store = (*Store)(nil)

func (s *Store) PutCheckpoint(ctx context.Context, cp cogfork.Checkpoint) error {
	worldJSON, err := json.Marshal(cp.WorldRef)
	if err != nil { return err }
	frontierJSON, err := json.Marshal(cp.ContextFrontier)
	if err != nil { return err }
	var retain any
	if cp.RetainUntil != nil { retain = formatForkTime(*cp.RetainUntil) }
	_, err = s.db.ExecContext(ctx, `INSERT INTO cognitive_fork_checkpoints(
checkpoint_id, source_agent_id, root_agent_id, process_version, ledger_sequence, last_event_id,
world_ref_json, context_frontier_json, process_state_json, state_hash, created_at, retain_until)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, cp.ID.String(), cp.SourceAgentID.String(), cp.RootAgentID.String(), cp.ProcessVersion, cp.LedgerSequence, cp.LastEventID.String(), worldJSON, frontierJSON, []byte(cp.ProcessStateJSON), cp.StateHash, formatForkTime(cp.CreatedAt), retain)
	if err != nil { return fmt.Errorf("insert cognitive fork checkpoint: %w", err) }
	return nil
}

func (s *Store) Checkpoint(ctx context.Context, checkpointID id.CheckpointID) (cogfork.Checkpoint, error) {
	var cp cogfork.Checkpoint
	var checkpoint, source, root, lastEvent, created string
	var worldJSON, frontierJSON, processJSON []byte
	var retain sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT checkpoint_id, source_agent_id, root_agent_id, process_version, ledger_sequence, last_event_id,
world_ref_json, context_frontier_json, process_state_json, state_hash, created_at, retain_until
FROM cognitive_fork_checkpoints WHERE checkpoint_id = ?`, checkpointID.String()).Scan(&checkpoint, &source, &root, &cp.ProcessVersion, &cp.LedgerSequence, &lastEvent, &worldJSON, &frontierJSON, &processJSON, &cp.StateHash, &created, &retain)
	if errors.Is(err, sql.ErrNoRows) { return cogfork.Checkpoint{}, cogfork.ErrNotFound }
	if err != nil { return cogfork.Checkpoint{}, fmt.Errorf("load cognitive fork checkpoint: %w", err) }
	cp.ID, cp.SourceAgentID, cp.RootAgentID, cp.LastEventID = id.CheckpointID(checkpoint), id.AgentID(source), id.AgentID(root), id.EventID(lastEvent)
	if err := json.Unmarshal(worldJSON, &cp.WorldRef); err != nil { return cogfork.Checkpoint{}, err }
	if err := json.Unmarshal(frontierJSON, &cp.ContextFrontier); err != nil { return cogfork.Checkpoint{}, err }
	cp.ProcessStateJSON = append(json.RawMessage(nil), processJSON...)
	cp.CreatedAt, err = parseForkTime(created)
	if err != nil { return cogfork.Checkpoint{}, err }
	if retain.Valid {
		t, err := parseForkTime(retain.String)
		if err != nil { return cogfork.Checkpoint{}, err }
		cp.RetainUntil = &t
	}
	return cp, nil
}

func (s *Store) CreateGroup(ctx context.Context, g cogfork.Group) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO cognitive_fork_groups(group_id, checkpoint_id, source_agent_id, state, winner_fork_id, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?, ?)`, g.ID.String(), g.CheckpointID.String(), g.SourceAgentID.String(), string(g.State), nullableFork(g.WinnerForkID.String()), formatForkTime(g.CreatedAt), formatForkTime(g.UpdatedAt))
	if err != nil { return fmt.Errorf("insert cognitive fork group: %w", err) }
	return nil
}

func (s *Store) Group(ctx context.Context, groupID id.ForkGroupID) (cogfork.Group, error) {
	var g cogfork.Group
	var group, checkpoint, source, state, created, updated string
	var winner sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT group_id, checkpoint_id, source_agent_id, state, winner_fork_id, created_at, updated_at FROM cognitive_fork_groups WHERE group_id = ?`, groupID.String()).Scan(&group, &checkpoint, &source, &state, &winner, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) { return cogfork.Group{}, cogfork.ErrNotFound }
	if err != nil { return cogfork.Group{}, err }
	g.ID, g.CheckpointID, g.SourceAgentID, g.State = id.ForkGroupID(group), id.CheckpointID(checkpoint), id.AgentID(source), cogfork.GroupState(state)
	if winner.Valid { g.WinnerForkID = id.ForkID(winner.String) }
	g.CreatedAt, err = parseForkTime(created)
	if err != nil { return cogfork.Group{}, err }
	g.UpdatedAt, err = parseForkTime(updated)
	if err != nil { return cogfork.Group{}, err }
	return g, nil
}

func (s *Store) UpdateGroup(ctx context.Context, g cogfork.Group) error {
	res, err := s.db.ExecContext(ctx, `UPDATE cognitive_fork_groups SET state = ?, winner_fork_id = ?, updated_at = ? WHERE group_id = ?`, string(g.State), nullableFork(g.WinnerForkID.String()), formatForkTime(g.UpdatedAt), g.ID.String())
	if err != nil { return err }
	n, err := res.RowsAffected()
	if err != nil { return err }
	if n != 1 { return cogfork.ErrNotFound }
	return nil
}

func (s *Store) CreateBranch(ctx context.Context, b cogfork.Branch) error {
	worldJSON, err := json.Marshal(b.WorldRef)
	if err != nil { return err }
	overlayJSON, err := json.Marshal(b.CognitiveOverlay)
	if err != nil { return err }
	evalJSON, err := marshalForkEvaluation(b.Evaluation)
	if err != nil { return err }
	_, err = s.db.ExecContext(ctx, `INSERT INTO cognitive_fork_branches(
fork_id, group_id, agent_id, world_id, world_ref_json,
budget_limit_money_micros, budget_limit_tokens, budget_spent_money_micros, budget_spent_tokens,
state, evaluation_json, cognitive_overlay_json, created_at, updated_at)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, b.ID.String(), b.GroupID.String(), b.AgentID.String(), b.WorldID.String(), worldJSON, b.BudgetLimit.MoneyMicros, b.BudgetLimit.Tokens, b.BudgetSpent.MoneyMicros, b.BudgetSpent.Tokens, string(b.State), evalJSON, overlayJSON, formatForkTime(b.CreatedAt), formatForkTime(b.UpdatedAt))
	if err != nil { return fmt.Errorf("insert cognitive fork branch: %w", err) }
	return nil
}

func (s *Store) Branch(ctx context.Context, forkID id.ForkID) (cogfork.Branch, error) {
	return loadForkBranch(s.db.QueryRowContext(ctx, forkBranchSelect+` WHERE fork_id = ?`, forkID.String()))
}

func (s *Store) UpdateBranch(ctx context.Context, b cogfork.Branch) error {
	evalJSON, err := marshalForkEvaluation(b.Evaluation)
	if err != nil { return err }
	overlayJSON, err := json.Marshal(b.CognitiveOverlay)
	if err != nil { return err }
	res, err := s.db.ExecContext(ctx, `UPDATE cognitive_fork_branches SET budget_spent_money_micros = ?, budget_spent_tokens = ?, state = ?, evaluation_json = ?, cognitive_overlay_json = ?, updated_at = ? WHERE fork_id = ?`, b.BudgetSpent.MoneyMicros, b.BudgetSpent.Tokens, string(b.State), evalJSON, overlayJSON, formatForkTime(b.UpdatedAt), b.ID.String())
	if err != nil { return err }
	n, err := res.RowsAffected()
	if err != nil { return err }
	if n != 1 { return cogfork.ErrNotFound }
	return nil
}

func (s *Store) Branches(ctx context.Context, groupID id.ForkGroupID) ([]cogfork.Branch, error) {
	if _, err := s.Group(ctx, groupID); err != nil { return nil, err }
	rows, err := s.db.QueryContext(ctx, forkBranchSelect+` WHERE group_id = ? ORDER BY fork_id`, groupID.String())
	if err != nil { return nil, err }
	defer rows.Close()
	out := make([]cogfork.Branch, 0)
	for rows.Next() {
		b, err := scanForkBranch(rows)
		if err != nil { return nil, err }
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *Store) DeleteBranch(ctx context.Context, forkID id.ForkID) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM cognitive_fork_branches WHERE fork_id = ?`, forkID.String())
	return err
}
func (s *Store) DeleteGroup(ctx context.Context, groupID id.ForkGroupID) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM cognitive_fork_groups WHERE group_id = ?`, groupID.String())
	return err
}
func (s *Store) DeleteCheckpoint(ctx context.Context, checkpointID id.CheckpointID) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM cognitive_fork_checkpoints WHERE checkpoint_id = ?`, checkpointID.String())
	return err
}

const forkBranchSelect = `SELECT fork_id, group_id, agent_id, world_id, world_ref_json,
budget_limit_money_micros, budget_limit_tokens, budget_spent_money_micros, budget_spent_tokens,
state, evaluation_json, cognitive_overlay_json, created_at, updated_at FROM cognitive_fork_branches`

type forkRowScanner interface { Scan(...any) error }

func loadForkBranch(row forkRowScanner) (cogfork.Branch, error) {
	b, err := scanForkBranch(row)
	if errors.Is(err, sql.ErrNoRows) { return cogfork.Branch{}, cogfork.ErrNotFound }
	return b, err
}

func scanForkBranch(row forkRowScanner) (cogfork.Branch, error) {
	var b cogfork.Branch
	var forkID, groupID, agentID, worldID, state, created, updated string
	var worldJSON, evalJSON, overlayJSON []byte
	if err := row.Scan(&forkID, &groupID, &agentID, &worldID, &worldJSON, &b.BudgetLimit.MoneyMicros, &b.BudgetLimit.Tokens, &b.BudgetSpent.MoneyMicros, &b.BudgetSpent.Tokens, &state, &evalJSON, &overlayJSON, &created, &updated); err != nil { return cogfork.Branch{}, err }
	b.ID, b.GroupID, b.AgentID, b.WorldID, b.State = id.ForkID(forkID), id.ForkGroupID(groupID), id.AgentID(agentID), id.WorldID(worldID), cogfork.BranchState(state)
	if err := json.Unmarshal(worldJSON, &b.WorldRef); err != nil { return cogfork.Branch{}, err }
	if len(evalJSON) > 0 {
		var e cogfork.Evaluation
		if err := json.Unmarshal(evalJSON, &e); err != nil { return cogfork.Branch{}, err }
		b.Evaluation = &e
	}
	if err := json.Unmarshal(overlayJSON, &b.CognitiveOverlay); err != nil { return cogfork.Branch{}, err }
	var err error
	b.CreatedAt, err = parseForkTime(created)
	if err != nil { return cogfork.Branch{}, err }
	b.UpdatedAt, err = parseForkTime(updated)
	if err != nil { return cogfork.Branch{}, err }
	return b, nil
}

func marshalForkEvaluation(e *cogfork.Evaluation) ([]byte, error) {
	if e == nil { return nil, nil }
	return json.Marshal(e)
}
func formatForkTime(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }
func parseForkTime(v string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, v)
	if err != nil { return time.Time{}, fmt.Errorf("parse fork time: %w", err) }
	return t.UTC(), nil
}
func nullableFork(v string) any {
	if strings.TrimSpace(v) == "" { return nil }
	return v
}
