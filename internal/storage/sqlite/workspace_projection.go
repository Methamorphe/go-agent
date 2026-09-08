package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/ledger"
	agentprocess "github.com/Methamorphe/go-agent/internal/process"
)

func (s *Store) LatestRoot(ctx context.Context) (agentprocess.State, error) {
	var schema uint32
	var body []byte
	err := s.db.QueryRowContext(ctx, `
SELECT state_schema_version, state_json
FROM agent_processes
WHERE agent_id = root_agent_id
ORDER BY updated_at DESC, agent_id DESC
LIMIT 1`).Scan(&schema, &body)
	if errors.Is(err, sql.ErrNoRows) {
		return agentprocess.State{}, errs.Wrap(errs.CodeNotFound, "sqlite.workspace.latest_root", "no root agent process exists", err)
	}
	if err != nil {
		return agentprocess.State{}, errs.Wrap(errs.CodeUnavailable, "sqlite.workspace.latest_root", "query latest root process", err)
	}
	return decodeState(schema, body)
}

func (s *Store) ProcessTree(ctx context.Context, rootID id.AgentID, limit int) ([]agentprocess.State, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT state_schema_version, state_json
FROM agent_processes
WHERE root_agent_id = ?
ORDER BY lineage_depth ASC, created_at ASC, agent_id ASC
LIMIT ?`, rootID, limit)
	if err != nil {
		return nil, errs.Wrap(errs.CodeUnavailable, "sqlite.workspace.tree", "query process tree", err)
	}
	defer rows.Close()

	states := make([]agentprocess.State, 0, min(limit, 128))
	for rows.Next() {
		var schema uint32
		var body []byte
		if err := rows.Scan(&schema, &body); err != nil {
			return nil, errs.Wrap(errs.CodeUnavailable, "sqlite.workspace.tree", "scan process tree", err)
		}
		state, err := decodeState(schema, body)
		if err != nil {
			return nil, err
		}
		states = append(states, state)
	}
	if err := rows.Err(); err != nil {
		return nil, errs.Wrap(errs.CodeUnavailable, "sqlite.workspace.tree", "iterate process tree", err)
	}
	return states, nil
}

func (s *Store) WorkspaceEventsBefore(ctx context.Context, agentID id.AgentID, before uint64, limit int) ([]ledger.Event, error) {
	query := `
SELECT ledger_sequence, event_id, agent_id, root_agent_id, process_version,
       event_type, schema_version, occurred_at, causation_id, correlation_id,
       actor_kind, actor_id, payload_json
FROM ledger_events
WHERE agent_id = ?`
	args := []any{agentID}
	if before > 0 {
		query += " AND process_version < ?"
		args = append(args, before)
	}
	query += " ORDER BY process_version DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, errs.Wrap(errs.CodeUnavailable, "sqlite.workspace.history", "query workspace history", err)
	}
	defer rows.Close()
	events, err := scanWorkspaceEvents(rows, limit)
	if err != nil {
		return nil, err
	}
	reverseEvents(events)
	return events, nil
}

func (s *Store) SearchWorkspaceEvents(ctx context.Context, rootID id.AgentID, query string, limit int) ([]ledger.Event, error) {
	pattern := "%" + escapeLike(strings.TrimSpace(query)) + "%"
	rows, err := s.db.QueryContext(ctx, `
SELECT ledger_sequence, event_id, agent_id, root_agent_id, process_version,
       event_type, schema_version, occurred_at, causation_id, correlation_id,
       actor_kind, actor_id, payload_json
FROM ledger_events
WHERE root_agent_id = ?
  AND (event_type LIKE ? ESCAPE '\' OR CAST(payload_json AS TEXT) LIKE ? ESCAPE '\')
ORDER BY ledger_sequence DESC
LIMIT ?`, rootID, pattern, pattern, limit)
	if err != nil {
		return nil, errs.Wrap(errs.CodeUnavailable, "sqlite.workspace.search", "search workspace history", err)
	}
	defer rows.Close()
	events, err := scanWorkspaceEvents(rows, limit)
	if err != nil {
		return nil, err
	}
	reverseEvents(events)
	return events, nil
}

func scanWorkspaceEvents(rows *sql.Rows, limit int) ([]ledger.Event, error) {
	events := make([]ledger.Event, 0, min(limit, 128))
	for rows.Next() {
		var event ledger.Event
		var occurredAt string
		var causation sql.NullString
		var actorKind string
		var payload []byte
		if err := rows.Scan(
			&event.LedgerSequence, &event.ID, &event.AgentID, &event.RootAgentID,
			&event.ProcessVersion, &event.Type, &event.SchemaVersion, &occurredAt,
			&causation, &event.CorrelationID, &actorKind, &event.Actor.ID, &payload,
		); err != nil {
			return nil, errs.Wrap(errs.CodeUnavailable, "sqlite.workspace.event", "scan workspace event", err)
		}
		event.Actor.Kind = ledger.ActorKind(actorKind)
		event.Payload = json.RawMessage(append([]byte(nil), payload...))
		parsed, err := time.Parse(time.RFC3339Nano, occurredAt)
		if err != nil {
			return nil, errs.Wrap(errs.CodeCorruption, "sqlite.workspace.event", "invalid workspace event timestamp", err)
		}
		event.Timestamp = parsed
		if causation.Valid {
			value := id.EventID(causation.String)
			event.CausationID = &value
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, errs.Wrap(errs.CodeUnavailable, "sqlite.workspace.event", "iterate workspace events", err)
	}
	return events, nil
}

func reverseEvents(events []ledger.Event) {
	for left, right := 0, len(events)-1; left < right; left, right = left+1, right-1 {
		events[left], events[right] = events[right], events[left]
	}
}

func escapeLike(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "%", "\\%")
	return strings.ReplaceAll(value, "_", "\\_")
}
