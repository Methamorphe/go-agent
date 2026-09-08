package sqlite

import (
	"context"

	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/ledger"
	agentprocess "github.com/Methamorphe/go-agent/internal/process"
)

func (s *Store) WorkspaceCursor(ctx context.Context, rootID id.AgentID) (uint64, error) {
	var cursor uint64
	if err := s.db.QueryRowContext(ctx, `
SELECT COALESCE(MAX(ledger_sequence), 0)
FROM ledger_events
WHERE root_agent_id=?`, rootID).Scan(&cursor); err != nil {
		return 0, errs.Wrap(errs.CodeUnavailable, "sqlite.workspace.cursor", "query workspace cursor", err)
	}
	return cursor, nil
}

func (s *Store) ProcessTreeAfter(ctx context.Context, rootID id.AgentID, after uint64, limit int) ([]agentprocess.State, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT p.state_schema_version, p.state_json
FROM agent_processes p
JOIN ledger_events e ON e.agent_id=p.agent_id AND e.process_version=p.version
WHERE p.root_agent_id=? AND e.ledger_sequence>?
ORDER BY e.ledger_sequence ASC
LIMIT ?`, rootID, after, limit)
	if err != nil {
		return nil, errs.Wrap(errs.CodeUnavailable, "sqlite.workspace.tree_after", "query changed process projections", err)
	}
	defer rows.Close()
	states := make([]agentprocess.State, 0, min(limit, 64))
	for rows.Next() {
		var schema uint32
		var body []byte
		if err := rows.Scan(&schema, &body); err != nil {
			return nil, errs.Wrap(errs.CodeUnavailable, "sqlite.workspace.tree_after", "scan changed process projection", err)
		}
		state, err := decodeState(schema, body)
		if err != nil {
			return nil, err
		}
		states = append(states, state)
	}
	if err := rows.Err(); err != nil {
		return nil, errs.Wrap(errs.CodeUnavailable, "sqlite.workspace.tree_after", "iterate changed process projections", err)
	}
	return states, nil
}

func (s *Store) WorkspaceEventsAfter(ctx context.Context, agentID id.AgentID, after uint64, limit int) ([]ledger.Event, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT ledger_sequence, event_id, agent_id, root_agent_id, process_version,
       event_type, schema_version, occurred_at, causation_id, correlation_id,
       actor_kind, actor_id, payload_json
FROM ledger_events
WHERE agent_id=? AND ledger_sequence>?
ORDER BY ledger_sequence ASC
LIMIT ?`, agentID, after, limit)
	if err != nil {
		return nil, errs.Wrap(errs.CodeUnavailable, "sqlite.workspace.events_after", "query incremental workspace events", err)
	}
	defer rows.Close()
	return scanWorkspaceEvents(rows, limit)
}
