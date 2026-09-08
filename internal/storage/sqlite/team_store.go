package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/team"
)

func (s *Store) PutTeam(ctx context.Context, value team.Team) error {
	if value.Proposal.ID == "" { return errs.New(errs.CodeInvalidArgument, "sqlite.team.put", "team id is required") }
	body, err := json.Marshal(value)
	if err != nil { return errs.Wrap(errs.CodeInvalidArgument, "sqlite.team.put", "encode team", err) }
	_, err = s.db.ExecContext(ctx, `
INSERT INTO adaptive_teams (team_id, state, team_json, created_at, updated_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(team_id) DO UPDATE SET
    state = excluded.state,
    team_json = excluded.team_json,
    updated_at = excluded.updated_at`,
		value.Proposal.ID.String(), string(value.State), string(body), formatTime(value.CreatedAt), formatTime(value.UpdatedAt))
	if err != nil { return errs.Wrap(errs.CodeUnavailable, "sqlite.team.put", "persist team", err) }
	return nil
}

func (s *Store) Team(ctx context.Context, teamID id.TeamID) (team.Team, error) {
	var body string
	err := s.db.QueryRowContext(ctx, `SELECT team_json FROM adaptive_teams WHERE team_id = ?`, teamID.String()).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) { return team.Team{}, errs.New(errs.CodeNotFound, "sqlite.team.get", "team not found") }
	if err != nil { return team.Team{}, errs.Wrap(errs.CodeUnavailable, "sqlite.team.get", "query team", err) }
	var value team.Team
	if err := json.Unmarshal([]byte(body), &value); err != nil { return team.Team{}, errs.Wrap(errs.CodeCorruption, "sqlite.team.get", "decode team", err) }
	return value, nil
}

func (s *Store) PutNegotiation(ctx context.Context, value team.Negotiation) error {
	if value.ID == "" || value.TeamID == "" { return errs.New(errs.CodeInvalidArgument, "sqlite.team.negotiation.put", "negotiation and team ids are required") }
	body, err := json.Marshal(value)
	if err != nil { return errs.Wrap(errs.CodeInvalidArgument, "sqlite.team.negotiation.put", "encode negotiation", err) }
	_, err = s.db.ExecContext(ctx, `
INSERT INTO agent_negotiations (negotiation_id, team_id, state, version, round, negotiation_json, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, value.ID.String(), value.TeamID.String(), string(value.State), value.Version, value.Round, string(body), formatTime(value.CreatedAt), formatTime(value.UpdatedAt))
	if err != nil { return errs.Wrap(errs.CodeUnavailable, "sqlite.team.negotiation.put", "persist negotiation", err) }
	return nil
}

func (s *Store) Negotiation(ctx context.Context, negotiationID id.NegotiationID) (team.Negotiation, error) {
	var body string
	err := s.db.QueryRowContext(ctx, `SELECT negotiation_json FROM agent_negotiations WHERE negotiation_id = ?`, negotiationID.String()).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) { return team.Negotiation{}, errs.New(errs.CodeNotFound, "sqlite.team.negotiation.get", "negotiation not found") }
	if err != nil { return team.Negotiation{}, errs.Wrap(errs.CodeUnavailable, "sqlite.team.negotiation.get", "query negotiation", err) }
	var value team.Negotiation
	if err := json.Unmarshal([]byte(body), &value); err != nil { return team.Negotiation{}, errs.Wrap(errs.CodeCorruption, "sqlite.team.negotiation.get", "decode negotiation", err) }
	return value, nil
}

func (s *Store) Turns(ctx context.Context, negotiationID id.NegotiationID, limit int) ([]team.Turn, error) {
	if limit <= 0 { limit = 128 }
	if limit > 128 { limit = 128 }
	rows, err := s.db.QueryContext(ctx, `
SELECT turn_json FROM agent_negotiation_turns
WHERE negotiation_id = ?
ORDER BY sequence ASC
LIMIT ?`, negotiationID.String(), limit)
	if err != nil { return nil, errs.Wrap(errs.CodeUnavailable, "sqlite.team.negotiation.turns", "query turns", err) }
	defer rows.Close()
	result := make([]team.Turn, 0, limit)
	for rows.Next() {
		var body string
		if err := rows.Scan(&body); err != nil { return nil, errs.Wrap(errs.CodeUnavailable, "sqlite.team.negotiation.turns", "scan turn", err) }
		var value team.Turn
		if err := json.Unmarshal([]byte(body), &value); err != nil { return nil, errs.Wrap(errs.CodeCorruption, "sqlite.team.negotiation.turns", "decode turn", err) }
		result = append(result, value)
	}
	if err := rows.Err(); err != nil { return nil, errs.Wrap(errs.CodeUnavailable, "sqlite.team.negotiation.turns", "iterate turns", err) }
	return result, nil
}

func (s *Store) AppendTurn(ctx context.Context, negotiationID id.NegotiationID, expectedVersion uint64, turn team.Turn, state team.NegotiationState, round int, resolution string, escalatedTo id.AgentID, now time.Time) (team.Negotiation, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil { return team.Negotiation{}, errs.Wrap(errs.CodeUnavailable, "sqlite.team.negotiation.append", "begin transaction", err) }
	defer tx.Rollback()
	var body string
	var version uint64
	err = tx.QueryRowContext(ctx, `SELECT negotiation_json, version FROM agent_negotiations WHERE negotiation_id = ?`, negotiationID.String()).Scan(&body, &version)
	if errors.Is(err, sql.ErrNoRows) { return team.Negotiation{}, errs.New(errs.CodeNotFound, "sqlite.team.negotiation.append", "negotiation not found") }
	if err != nil { return team.Negotiation{}, errs.Wrap(errs.CodeUnavailable, "sqlite.team.negotiation.append", "query negotiation", err) }
	if version != expectedVersion { return team.Negotiation{}, errs.New(errs.CodeConflict, "sqlite.team.negotiation.append", "negotiation version changed") }
	var current team.Negotiation
	if err := json.Unmarshal([]byte(body), &current); err != nil { return team.Negotiation{}, errs.Wrap(errs.CodeCorruption, "sqlite.team.negotiation.append", "decode negotiation", err) }
	turnBody, err := json.Marshal(turn)
	if err != nil { return team.Negotiation{}, errs.Wrap(errs.CodeInvalidArgument, "sqlite.team.negotiation.append", "encode turn", err) }
	_, err = tx.ExecContext(ctx, `
INSERT INTO agent_negotiation_turns (negotiation_id, sequence, round, actor_agent_id, kind, turn_json, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`, negotiationID.String(), turn.Sequence, turn.Round, turn.ActorAgentID.String(), string(turn.Kind), string(turnBody), formatTime(turn.CreatedAt))
	if err != nil { return team.Negotiation{}, errs.Wrap(errs.CodeConflict, "sqlite.team.negotiation.append", "append negotiation turn", err) }
	current.State = state
	current.Round = round
	current.Resolution = resolution
	current.EscalatedTo = escalatedTo
	current.Version++
	current.UpdatedAt = now.UTC()
	nextBody, err := json.Marshal(current)
	if err != nil { return team.Negotiation{}, errs.Wrap(errs.CodeInternal, "sqlite.team.negotiation.append", "encode updated negotiation", err) }
	result, err := tx.ExecContext(ctx, `
UPDATE agent_negotiations
SET state = ?, version = ?, round = ?, negotiation_json = ?, updated_at = ?
WHERE negotiation_id = ? AND version = ?`, string(current.State), current.Version, current.Round, string(nextBody), formatTime(current.UpdatedAt), negotiationID.String(), expectedVersion)
	if err != nil { return team.Negotiation{}, errs.Wrap(errs.CodeUnavailable, "sqlite.team.negotiation.append", "update negotiation", err) }
	affected, err := result.RowsAffected()
	if err != nil { return team.Negotiation{}, errs.Wrap(errs.CodeUnavailable, "sqlite.team.negotiation.append", "inspect update", err) }
	if affected != 1 { return team.Negotiation{}, errs.New(errs.CodeConflict, "sqlite.team.negotiation.append", "negotiation version changed") }
	if err := tx.Commit(); err != nil { return team.Negotiation{}, errs.Wrap(errs.CodeUnavailable, "sqlite.team.negotiation.append", "commit transaction", err) }
	return current, nil
}

var _ team.Store = (*Store)(nil)
