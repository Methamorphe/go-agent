package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/ledger"
	agentprocess "github.com/Methamorphe/go-agent/internal/process"
)

func (s *Store) Current(
	ctx context.Context,
	agentID id.AgentID,
) (agentprocess.State, error) {
	const query = `
SELECT
    state_schema_version,
    state_json,
    version,
    status
FROM agent_processes
WHERE agent_id = ?`

	var schema uint32
	var body []byte
	var version uint64
	var status string

	err := s.db.QueryRowContext(
		ctx,
		query,
		agentID,
	).Scan(
		&schema,
		&body,
		&version,
		&status,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return agentprocess.State{},
			errs.Wrap(
				errs.CodeNotFound,
				"sqlite.process.current",
				"agent process not found",
				err,
			)
	}

	if err != nil {
		return agentprocess.State{},
			errs.Wrap(
				errs.CodeUnavailable,
				"sqlite.process.current",
				"query current process",
				err,
			)
	}

	state, err := decodeState(
		schema,
		body,
	)
	if err != nil {
		return agentprocess.State{}, err
	}

	if state.AgentID != agentID ||
		state.Version != version ||
		string(state.Status) != status {
		return agentprocess.State{},
			errs.New(
				errs.CodeCorruption,
				"sqlite.process.current",
				"projection columns do not match serialized state",
			)
	}

	return state, nil
}

func (s *Store) Append(
	ctx context.Context,
	expectedVersion uint64,
	events []ledger.Event,
	next agentprocess.State,
	receipt agentprocess.Receipt,
) error {
	if len(events) == 0 {
		return errs.New(
			errs.CodeInvalidArgument,
			"sqlite.process.append",
			"at least one event is required",
		)
	}

	if next.Version !=
		expectedVersion+
			uint64(len(events)) {
		return errs.New(
			errs.CodeInvalidArgument,
			"sqlite.process.append",
			"next state version does not match event count",
		)
	}

	for index, event := range events {
		if err := event.Validate(); err != nil {
			return err
		}

		expectedEventVersion :=
			expectedVersion +
				uint64(index) + 1

		if event.AgentID != next.AgentID ||
			event.RootAgentID !=
				next.RootAgentID ||
			event.ProcessVersion !=
				expectedEventVersion {
			return errs.New(
				errs.CodeInvalidArgument,
				"sqlite.process.append",
				"event stream identity/version is inconsistent",
			)
		}
	}

	stateJSON, err := json.Marshal(next)
	if err != nil {
		return errs.Wrap(
			errs.CodeInternal,
			"sqlite.process.append",
			"marshal process projection",
			err,
		)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return errs.Wrap(
			errs.CodeUnavailable,
			"sqlite.process.append",
			"begin transaction",
			err,
		)
	}

	rollback := func() {
		_ = tx.Rollback()
	}

	if expectedVersion == 0 {
		var exists int

		err := tx.QueryRowContext(
			ctx,
			`
SELECT 1
FROM agent_processes
WHERE agent_id = ?`,
			next.AgentID,
		).Scan(&exists)

		if err == nil {
			rollback()

			return errs.New(
				errs.CodeConflict,
				"sqlite.process.append",
				"agent process already exists",
			)
		}

		if !errors.Is(
			err,
			sql.ErrNoRows,
		) {
			rollback()

			return errs.Wrap(
				errs.CodeUnavailable,
				"sqlite.process.append",
				"check process existence",
				err,
			)
		}

		for _, event := range events {
			if err := insertEvent(
				ctx,
				tx,
				event,
			); err != nil {
				rollback()
				return err
			}
		}

		if err := insertProjection(
			ctx,
			tx,
			next,
			stateJSON,
		); err != nil {
			rollback()
			return err
		}
	} else {
		result, err := tx.ExecContext(
			ctx,
			`
UPDATE agent_processes
SET
    version = ?,
    status = ?,
    state_schema_version = ?,
    state_json = ?,
    updated_at = ?
WHERE
    agent_id = ?
    AND version = ?`,
			next.Version,
			next.Status,
			agentprocess.StateSchemaVersion,
			stateJSON,
			next.UpdatedAt.UTC().
				Format(time.RFC3339Nano),
			next.AgentID,
			expectedVersion,
		)

		if err != nil {
			rollback()

			return errs.Wrap(
				errs.CodeUnavailable,
				"sqlite.process.append",
				"compare-and-swap process projection",
				err,
			)
		}

		rows, err := result.RowsAffected()
		if err != nil {
			rollback()

			return errs.Wrap(
				errs.CodeInternal,
				"sqlite.process.append",
				"read projection affected rows",
				err,
			)
		}

		if rows != 1 {
			rollback()

			return errs.New(
				errs.CodeConflict,
				"sqlite.process.append",
				"process version changed concurrently",
			)
		}

		for _, event := range events {
			if err := insertEvent(
				ctx,
				tx,
				event,
			); err != nil {
				rollback()
				return err
			}
		}
	}

	if err := syncWakeCondition(
		ctx,
		tx,
		next,
	); err != nil {
		rollback()
		return err
	}

	if err := insertReceipt(
		ctx,
		tx,
		receipt,
	); err != nil {
		rollback()
		return err
	}

	if err := tx.Commit(); err != nil {
		return errs.Wrap(
			errs.CodeUnavailable,
			"sqlite.process.append",
			"commit canonical transition",
			err,
		)
	}

	return nil
}

func insertEvent(
	ctx context.Context,
	tx *sql.Tx,
	event ledger.Event,
) error {
	var causation any

	if event.CausationID != nil {
		causation =
			event.CausationID.String()
	}

	_, err := tx.ExecContext(
		ctx,
		`
INSERT INTO ledger_events
(
    event_id,
    agent_id,
    root_agent_id,
    process_version,
    event_type,
    schema_version,
    occurred_at,
    causation_id,
    correlation_id,
    actor_kind,
    actor_id,
    payload_json
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		event.ID,
		event.AgentID,
		event.RootAgentID,
		event.ProcessVersion,
		event.Type,
		event.SchemaVersion,
		event.Timestamp.UTC().
			Format(time.RFC3339Nano),
		causation,
		event.CorrelationID,
		event.Actor.Kind,
		event.Actor.ID,
		[]byte(event.Payload),
	)

	if err != nil {
		return errs.Wrap(
			errs.CodeUnavailable,
			"sqlite.ledger.insert",
			"insert ledger event",
			err,
		)
	}

	return nil
}

func insertProjection(
	ctx context.Context,
	tx *sql.Tx,
	state agentprocess.State,
	body []byte,
) error {
	var parent any

	if state.ParentAgentID != nil {
		parent =
			state.ParentAgentID.String()
	}

	_, err := tx.ExecContext(
		ctx,
		`
INSERT INTO agent_processes
(
    agent_id,
    root_agent_id,
    parent_agent_id,
    creation_event_id,
    created_at,
    lineage_depth,
    version,
    status,
    state_schema_version,
    state_json,
    updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		state.AgentID,
		state.RootAgentID,
		parent,
		state.CreationEventID,
		state.CreatedAt.UTC().
			Format(time.RFC3339Nano),
		state.LineageDepth,
		state.Version,
		state.Status,
		agentprocess.StateSchemaVersion,
		body,
		state.UpdatedAt.UTC().
			Format(time.RFC3339Nano),
	)

	if err != nil {
		return errs.Wrap(
			errs.CodeUnavailable,
			"sqlite.process.insert_projection",
			"insert process projection",
			err,
		)
	}

	return nil
}

func syncWakeCondition(
	ctx context.Context,
	tx *sql.Tx,
	state agentprocess.State,
) error {
	if state.Sleep == nil {
		if _, err := tx.ExecContext(
			ctx,
			`
DELETE FROM wake_conditions
WHERE agent_id = ?`,
			state.AgentID,
		); err != nil {
			return errs.Wrap(
				errs.CodeUnavailable,
				"sqlite.process.wake_condition",
				"delete wake condition",
				err,
			)
		}

		return nil
	}

	_, err := tx.ExecContext(
		ctx,
		`
INSERT INTO wake_conditions
(
    agent_id,
    sleep_id,
    wake_at,
    generation,
    process_version
)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(agent_id) DO UPDATE SET
    sleep_id = excluded.sleep_id,
    wake_at = excluded.wake_at,
    generation = excluded.generation,
    process_version = excluded.process_version`,
		state.AgentID,
		state.Sleep.ID,
		state.Sleep.WakeAt.UTC().
			Format(time.RFC3339Nano),
		state.Sleep.Generation,
		state.Version,
	)

	if err != nil {
		return errs.Wrap(
			errs.CodeUnavailable,
			"sqlite.process.wake_condition",
			"upsert wake condition",
			err,
		)
	}

	return nil
}

func insertReceipt(
	ctx context.Context,
	tx *sql.Tx,
	receipt agentprocess.Receipt,
) error {
	result, err := tx.ExecContext(
		ctx,
		`
INSERT OR IGNORE INTO command_receipts
(
    request_id,
    command_type,
    agent_id,
    result_version,
    result_json,
    last_event_id,
    created_at
)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		receipt.RequestID,
		receipt.CommandType,
		receipt.AgentID,
		receipt.ResultVersion,
		receipt.ResultJSON,
		receipt.LastEventID,
		receipt.CreatedAt.UTC().
			Format(time.RFC3339Nano),
	)

	if err != nil {
		return errs.Wrap(
			errs.CodeUnavailable,
			"sqlite.command_receipt.insert",
			"insert command receipt",
			err,
		)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return errs.Wrap(
			errs.CodeInternal,
			"sqlite.command_receipt.insert",
			"read receipt affected rows",
			err,
		)
	}

	if rows != 1 {
		return errs.New(
			errs.CodeConflict,
			"sqlite.command_receipt.insert",
			"request_id already accepted",
		)
	}

	return nil
}

func (s *Store) Receipt(
	ctx context.Context,
	requestID id.RequestID,
) (agentprocess.Receipt, bool, error) {
	const query = `
SELECT
    request_id,
    command_type,
    agent_id,
    result_version,
    result_json,
    last_event_id,
    created_at
FROM command_receipts
WHERE request_id = ?`

	var receipt agentprocess.Receipt
	var createdAt string

	err := s.db.QueryRowContext(
		ctx,
		query,
		requestID,
	).Scan(
		&receipt.RequestID,
		&receipt.CommandType,
		&receipt.AgentID,
		&receipt.ResultVersion,
		&receipt.ResultJSON,
		&receipt.LastEventID,
		&createdAt,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return agentprocess.Receipt{},
			false,
			nil
	}

	if err != nil {
		return agentprocess.Receipt{},
			false,
			errs.Wrap(
				errs.CodeUnavailable,
				"sqlite.command_receipt.get",
				"query command receipt",
				err,
			)
	}

	receipt.CreatedAt, err =
		time.Parse(
			time.RFC3339Nano,
			createdAt,
		)
	if err != nil {
		return agentprocess.Receipt{},
			false,
			errs.Wrap(
				errs.CodeCorruption,
				"sqlite.command_receipt.get",
				"invalid receipt timestamp",
				err,
			)
	}

	return receipt, true, nil
}

func (s *Store) Events(
	ctx context.Context,
	agentID id.AgentID,
	afterVersion uint64,
	limit int,
) ([]ledger.Event, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`
SELECT
    ledger_sequence,
    event_id,
    agent_id,
    root_agent_id,
    process_version,
    event_type,
    schema_version,
    occurred_at,
    causation_id,
    correlation_id,
    actor_kind,
    actor_id,
    payload_json
FROM ledger_events
WHERE
    agent_id = ?
    AND process_version > ?
ORDER BY process_version ASC
LIMIT ?`,
		agentID,
		afterVersion,
		limit,
	)

	if err != nil {
		return nil,
			errs.Wrap(
				errs.CodeUnavailable,
				"sqlite.ledger.events",
				"query ledger events",
				err,
			)
	}

	defer rows.Close()

	events := make(
		[]ledger.Event,
		0,
		min(limit, 128),
	)

	for rows.Next() {
		var event ledger.Event
		var occurredAt string
		var causation sql.NullString
		var actorKind string
		var payload []byte

		if err := rows.Scan(
			&event.LedgerSequence,
			&event.ID,
			&event.AgentID,
			&event.RootAgentID,
			&event.ProcessVersion,
			&event.Type,
			&event.SchemaVersion,
			&occurredAt,
			&causation,
			&event.CorrelationID,
			&actorKind,
			&event.Actor.ID,
			&payload,
		); err != nil {
			return nil,
				errs.Wrap(
					errs.CodeUnavailable,
					"sqlite.ledger.events",
					"scan ledger event",
					err,
				)
		}

		event.Actor.Kind =
			ledger.ActorKind(actorKind)

		event.Payload =
			json.RawMessage(payload)

		event.Timestamp, err =
			time.Parse(
				time.RFC3339Nano,
				occurredAt,
			)
		if err != nil {
			return nil,
				errs.Wrap(
					errs.CodeCorruption,
					"sqlite.ledger.events",
					"invalid event timestamp",
					err,
				)
		}

		if causation.Valid {
			value :=
				id.EventID(
					causation.String,
				)

			event.CausationID = &value
		}

		events = append(
			events,
			event,
		)
	}

	if err := rows.Err(); err != nil {
		return nil,
			errs.Wrap(
				errs.CodeUnavailable,
				"sqlite.ledger.events",
				"iterate ledger events",
				err,
			)
	}

	return events, nil
}

func (s *Store) LedgerSequenceAtVersion(
	ctx context.Context,
	agentID id.AgentID,
	version uint64,
) (uint64, error) {
	var sequence uint64

	err := s.db.QueryRowContext(
		ctx,
		`
SELECT ledger_sequence
FROM ledger_events
WHERE
    agent_id = ?
    AND process_version = ?`,
		agentID,
		version,
	).Scan(&sequence)

	if errors.Is(err, sql.ErrNoRows) {
		return 0,
			errs.Wrap(
				errs.CodeNotFound,
				"sqlite.ledger.sequence",
				"ledger event not found",
				err,
			)
	}

	if err != nil {
		return 0,
			errs.Wrap(
				errs.CodeUnavailable,
				"sqlite.ledger.sequence",
				"query ledger sequence",
				err,
			)
	}

	return sequence, nil
}

func (s *Store) SaveSnapshot(
	ctx context.Context,
	snapshot agentprocess.Snapshot,
) error {
	result, err := s.db.ExecContext(
		ctx,
		`
INSERT OR IGNORE INTO agent_snapshots
(
    snapshot_id,
    agent_id,
    through_process_version,
    through_ledger_sequence,
    schema_version,
    created_at,
    state_json,
    sha256
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		snapshot.ID,
		snapshot.AgentID,
		snapshot.ThroughProcessVersion,
		snapshot.ThroughLedgerSequence,
		snapshot.SchemaVersion,
		snapshot.CreatedAt.UTC().
			Format(time.RFC3339Nano),
		snapshot.StateJSON,
		snapshot.SHA256,
	)

	if err != nil {
		return errs.Wrap(
			errs.CodeUnavailable,
			"sqlite.snapshot.save",
			"insert snapshot",
			err,
		)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return errs.Wrap(
			errs.CodeInternal,
			"sqlite.snapshot.save",
			"read snapshot affected rows",
			err,
		)
	}

	if rows == 1 {
		return nil
	}

	var existingHash string

	err = s.db.QueryRowContext(
		ctx,
		`
SELECT sha256
FROM agent_snapshots
WHERE
    agent_id = ?
    AND through_process_version = ?`,
		snapshot.AgentID,
		snapshot.ThroughProcessVersion,
	).Scan(&existingHash)

	if err != nil {
		return errs.Wrap(
			errs.CodeUnavailable,
			"sqlite.snapshot.save",
			"read existing snapshot",
			err,
		)
	}

	if existingHash != snapshot.SHA256 {
		return errs.New(
			errs.CodeCorruption,
			"sqlite.snapshot.save",
			"snapshot version already exists with different content",
		)
	}

	return nil
}

func (s *Store) LatestSnapshot(
	ctx context.Context,
	agentID id.AgentID,
) (agentprocess.Snapshot, bool, error) {
	var snapshot agentprocess.Snapshot
	var createdAt string

	err := s.db.QueryRowContext(
		ctx,
		`
SELECT
    snapshot_id,
    agent_id,
    through_process_version,
    through_ledger_sequence,
    schema_version,
    created_at,
    state_json,
    sha256
FROM agent_snapshots
WHERE agent_id = ?
ORDER BY through_process_version DESC
LIMIT 1`,
		agentID,
	).Scan(
		&snapshot.ID,
		&snapshot.AgentID,
		&snapshot.ThroughProcessVersion,
		&snapshot.ThroughLedgerSequence,
		&snapshot.SchemaVersion,
		&createdAt,
		&snapshot.StateJSON,
		&snapshot.SHA256,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return agentprocess.Snapshot{},
			false,
			nil
	}

	if err != nil {
		return agentprocess.Snapshot{},
			false,
			errs.Wrap(
				errs.CodeUnavailable,
				"sqlite.snapshot.latest",
				"query latest snapshot",
				err,
			)
	}

	snapshot.CreatedAt, err =
		time.Parse(
			time.RFC3339Nano,
			createdAt,
		)
	if err != nil {
		return agentprocess.Snapshot{},
			false,
			errs.Wrap(
				errs.CodeCorruption,
				"sqlite.snapshot.latest",
				"invalid snapshot timestamp",
				err,
			)
	}

	return snapshot, true, nil
}

func (s *Store) ListByStatus(
	ctx context.Context,
	status agentprocess.Status,
	limit int,
) ([]agentprocess.State, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`
SELECT
    state_schema_version,
    state_json
FROM agent_processes
WHERE status = ?
ORDER BY updated_at, agent_id
LIMIT ?`,
		status,
		limit,
	)

	if err != nil {
		return nil,
			errs.Wrap(
				errs.CodeUnavailable,
				"sqlite.process.list_status",
				"query process projections",
				err,
			)
	}

	defer rows.Close()

	result := make(
		[]agentprocess.State,
		0,
		min(limit, 128),
	)

	for rows.Next() {
		var schema uint32
		var body []byte

		if err := rows.Scan(
			&schema,
			&body,
		); err != nil {
			return nil,
				errs.Wrap(
					errs.CodeUnavailable,
					"sqlite.process.list_status",
					"scan process projection",
					err,
				)
		}

		state, err :=
			decodeState(
				schema,
				body,
			)
		if err != nil {
			return nil, err
		}

		result = append(
			result,
			state,
		)
	}

	if err := rows.Err(); err != nil {
		return nil,
			errs.Wrap(
				errs.CodeUnavailable,
				"sqlite.process.list_status",
				"iterate process projections",
				err,
			)
	}

	return result, nil
}

func (s *Store) DueSleeps(
	ctx context.Context,
	now time.Time,
	limit int,
) ([]agentprocess.SleepDue, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`
SELECT
    agent_id,
    sleep_id,
    generation,
    wake_at,
    process_version
FROM wake_conditions
WHERE wake_at <= ?
ORDER BY wake_at, agent_id
LIMIT ?`,
		now.UTC().
			Format(time.RFC3339Nano),
		limit,
	)

	if err != nil {
		return nil,
			errs.Wrap(
				errs.CodeUnavailable,
				"sqlite.process.due_sleeps",
				"query due sleeps",
				err,
			)
	}

	defer rows.Close()

	result := make(
		[]agentprocess.SleepDue,
		0,
		min(limit, 128),
	)

	for rows.Next() {
		var due agentprocess.SleepDue
		var wakeAt string

		if err := rows.Scan(
			&due.AgentID,
			&due.SleepID,
			&due.Generation,
			&wakeAt,
			&due.Version,
		); err != nil {
			return nil,
				errs.Wrap(
					errs.CodeUnavailable,
					"sqlite.process.due_sleeps",
					"scan due sleep",
					err,
				)
		}

		due.WakeAt, err =
			time.Parse(
				time.RFC3339Nano,
				wakeAt,
			)
		if err != nil {
			return nil,
				errs.Wrap(
					errs.CodeCorruption,
					"sqlite.process.due_sleeps",
					"invalid wake timestamp",
					err,
				)
		}

		result = append(
			result,
			due,
		)
	}

	if err := rows.Err(); err != nil {
		return nil,
			errs.Wrap(
				errs.CodeUnavailable,
				"sqlite.process.due_sleeps",
				"iterate due sleeps",
				err,
			)
	}

	return result, nil
}

func decodeState(
	schema uint32,
	body []byte,
) (agentprocess.State, error) {
	if schema !=
		agentprocess.StateSchemaVersion {
		return agentprocess.State{},
			errs.New(
				errs.CodeUnsupported,
				"sqlite.process.decode_state",
				fmt.Sprintf(
					"unsupported process state schema version %d",
					schema,
				),
			)
	}

	var state agentprocess.State

	if err := json.Unmarshal(
		body,
		&state,
	); err != nil {
		return agentprocess.State{},
			errs.Wrap(
				errs.CodeCorruption,
				"sqlite.process.decode_state",
				"decode process state",
				err,
			)
	}

	return state, nil
}
