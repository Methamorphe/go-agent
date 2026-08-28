package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/orchestration"
)

func orchestrationJSON(v any) (string, error) {
	body, err := json.Marshal(v)
	return string(body), err
}

func (s *Store) BootstrapRoot(ctx context.Context, agentID id.AgentID, policy orchestration.RootPolicy, now time.Time) error {
	authority, err := orchestrationJSON(policy.Authority)
	if err != nil { return err }
	available, err := orchestrationJSON(policy.Budget)
	if err != nil { return err }
	reserved, _ := orchestrationJSON(orchestration.Budget{})
	policyJSON, err := orchestrationJSON(policy)
	if err != nil { return err }
	_, err = s.db.ExecContext(ctx, `
INSERT INTO orchestration_accounts
(agent_id, root_id, authority_json, available_json, reserved_json, policy_json, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(agent_id) DO NOTHING`,
		agentID.String(), agentID.String(), authority, available, reserved, policyJSON, formatTime(now), formatTime(now),
	)
	if err != nil { return errs.Wrap(errs.CodeUnavailable, "sqlite.orchestration.bootstrap", "insert root account", err) }
	return nil
}

func scanOrchestrationAccount(scanner rowScanner) (orchestration.Account, error) {
	var account orchestration.Account
	var agentID, rootID, authorityJSON, availableJSON, reservedJSON, policyJSON string
	if err := scanner.Scan(&agentID, &rootID, &authorityJSON, &availableJSON, &reservedJSON, &policyJSON); err != nil { return account, err }
	account.AgentID = id.AgentID(agentID)
	account.RootID = id.AgentID(rootID)
	if err := json.Unmarshal([]byte(authorityJSON), &account.Authority); err != nil { return account, err }
	if err := json.Unmarshal([]byte(availableJSON), &account.Available); err != nil { return account, err }
	if err := json.Unmarshal([]byte(reservedJSON), &account.Reserved); err != nil { return account, err }
	if err := json.Unmarshal([]byte(policyJSON), &account.Policy); err != nil { return account, err }
	return account, nil
}

func (s *Store) Account(ctx context.Context, agentID id.AgentID) (orchestration.Account, error) {
	account, err := scanOrchestrationAccount(s.db.QueryRowContext(ctx, `
SELECT agent_id, root_id, authority_json, available_json, reserved_json, policy_json
FROM orchestration_accounts WHERE agent_id = ?`, agentID.String()))
	if errors.Is(err, sql.ErrNoRows) { return account, errs.Wrap(errs.CodeNotFound, "sqlite.orchestration.account", "account not found", err) }
	if err != nil { return account, errs.Wrap(errs.CodeUnavailable, "sqlite.orchestration.account", "query account", err) }
	return account, nil
}

func (s *Store) ReserveSpawn(ctx context.Context, record orchestration.SpawnRecord) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil { return errs.Wrap(errs.CodeUnavailable, "sqlite.orchestration.reserve", "begin transaction", err) }
	defer tx.Rollback()

	account, err := scanOrchestrationAccount(tx.QueryRowContext(ctx, `
SELECT agent_id, root_id, authority_json, available_json, reserved_json, policy_json
FROM orchestration_accounts WHERE agent_id = ?`, record.ParentAgentID.String()))
	if err != nil { return errs.Wrap(errs.CodeUnavailable, "sqlite.orchestration.reserve", "load parent account", err) }
	if !record.Reserved.Fits(account.Available) { return errs.New(errs.CodeResourceExhausted, "sqlite.orchestration.reserve", "budget no longer available") }

	account.Available = account.Available.Sub(record.Reserved)
	account.Reserved = account.Reserved.Add(record.Reserved)
	available, _ := orchestrationJSON(account.Available)
	reserved, _ := orchestrationJSON(account.Reserved)
	authority, _ := orchestrationJSON(record.Authority)
	spawnBudget, _ := orchestrationJSON(record.Reserved)

	if _, err = tx.ExecContext(ctx, `
UPDATE orchestration_accounts SET available_json = ?, reserved_json = ?, updated_at = ? WHERE agent_id = ?`,
		available, reserved, formatTime(record.UpdatedAt), record.ParentAgentID.String()); err != nil {
		return errs.Wrap(errs.CodeUnavailable, "sqlite.orchestration.reserve", "debit parent", err)
	}
	if _, err = tx.ExecContext(ctx, `
INSERT INTO orchestration_spawns
(spawn_id, parent_agent_id, root_agent_id, depth, task_intent, task_key, authority_json, reservation_id, reserved_json, status, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.ID.String(), record.ParentAgentID.String(), record.RootAgentID.String(), record.Depth, record.TaskIntent, record.TaskKey,
		authority, record.ReservationID.String(), spawnBudget, string(record.Status), formatTime(record.CreatedAt), formatTime(record.UpdatedAt)); err != nil {
		return errs.Wrap(errs.CodeUnavailable, "sqlite.orchestration.reserve", "insert spawn", err)
	}
	if err := tx.Commit(); err != nil { return errs.Wrap(errs.CodeUnavailable, "sqlite.orchestration.reserve", "commit", err) }
	return nil
}

func (s *Store) BindSpawnChild(ctx context.Context, spawnID id.SpawnID, childID id.AgentID, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil { return err }
	defer tx.Rollback()
	record, err := scanOrchestrationSpawn(tx.QueryRowContext(ctx, orchestrationSpawnSelect+` WHERE spawn_id = ?`, spawnID.String()))
	if err != nil { return err }
	parent, err := scanOrchestrationAccount(tx.QueryRowContext(ctx, `
SELECT agent_id, root_id, authority_json, available_json, reserved_json, policy_json
FROM orchestration_accounts WHERE agent_id = ?`, record.ParentAgentID.String()))
	if err != nil { return err }
	authority, _ := orchestrationJSON(record.Authority)
	available, _ := orchestrationJSON(record.Reserved)
	zero, _ := orchestrationJSON(orchestration.Budget{})
	policy, _ := orchestrationJSON(parent.Policy)
	if _, err = tx.ExecContext(ctx, `
INSERT INTO orchestration_accounts
(agent_id, root_id, authority_json, available_json, reserved_json, policy_json, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, childID.String(), record.RootAgentID.String(), authority, available, zero, policy, formatTime(now), formatTime(now)); err != nil {
		return errs.Wrap(errs.CodeUnavailable, "sqlite.orchestration.bind", "create child account", err)
	}
	result, err := tx.ExecContext(ctx, `
UPDATE orchestration_spawns SET child_agent_id = ?, status = ?, updated_at = ? WHERE spawn_id = ? AND status = ?`,
		childID.String(), string(orchestration.SpawnCreated), formatTime(now), spawnID.String(), string(orchestration.SpawnReserved))
	if err != nil { return err }
	rows, _ := result.RowsAffected()
	if rows != 1 { return errs.New(errs.CodeConflict, "sqlite.orchestration.bind", "spawn is no longer reserved") }
	return tx.Commit()
}

func (s *Store) RejectSpawn(ctx context.Context, spawnID id.SpawnID, reason string, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil { return err }
	defer tx.Rollback()
	record, err := scanOrchestrationSpawn(tx.QueryRowContext(ctx, orchestrationSpawnSelect+` WHERE spawn_id = ?`, spawnID.String()))
	if err != nil { return err }
	if record.Status != orchestration.SpawnReserved { return nil }
	account, err := scanOrchestrationAccount(tx.QueryRowContext(ctx, `
SELECT agent_id, root_id, authority_json, available_json, reserved_json, policy_json
FROM orchestration_accounts WHERE agent_id = ?`, record.ParentAgentID.String()))
	if err != nil { return err }
	account.Available = account.Available.Add(record.Reserved)
	account.Reserved = account.Reserved.Sub(record.Reserved)
	available, _ := orchestrationJSON(account.Available)
	reserved, _ := orchestrationJSON(account.Reserved)
	if _, err = tx.ExecContext(ctx, `UPDATE orchestration_accounts SET available_json = ?, reserved_json = ?, updated_at = ? WHERE agent_id = ?`, available, reserved, formatTime(now), account.AgentID.String()); err != nil { return err }
	if _, err = tx.ExecContext(ctx, `UPDATE orchestration_spawns SET status = ?, reject_reason = ?, updated_at = ? WHERE spawn_id = ?`, string(orchestration.SpawnRejected), reason, formatTime(now), spawnID.String()); err != nil { return err }
	return tx.Commit()
}

func (s *Store) SettleSpawn(ctx context.Context, spawnID id.SpawnID, actual orchestration.Budget, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil { return err }
	defer tx.Rollback()
	record, err := scanOrchestrationSpawn(tx.QueryRowContext(ctx, orchestrationSpawnSelect+` WHERE spawn_id = ?`, spawnID.String()))
	if err != nil { return err }
	if record.Status == orchestration.SpawnSettled { return nil }
	if record.Status != orchestration.SpawnCreated { return errs.New(errs.CodeConflict, "sqlite.orchestration.settle", "spawn is not created") }
	if !actual.Fits(record.Reserved) { return errs.New(errs.CodeResourceExhausted, "sqlite.orchestration.settle", "actual exceeds reservation") }
	account, err := scanOrchestrationAccount(tx.QueryRowContext(ctx, `
SELECT agent_id, root_id, authority_json, available_json, reserved_json, policy_json
FROM orchestration_accounts WHERE agent_id = ?`, record.ParentAgentID.String()))
	if err != nil { return err }
	unused := record.Reserved.Sub(actual)
	account.Reserved = account.Reserved.Sub(record.Reserved)
	account.Available = account.Available.Add(unused)
	available, _ := orchestrationJSON(account.Available)
	reserved, _ := orchestrationJSON(account.Reserved)
	actualJSON, _ := orchestrationJSON(actual)
	if _, err = tx.ExecContext(ctx, `UPDATE orchestration_accounts SET available_json = ?, reserved_json = ?, updated_at = ? WHERE agent_id = ?`, available, reserved, formatTime(now), account.AgentID.String()); err != nil { return err }
	if _, err = tx.ExecContext(ctx, `UPDATE orchestration_spawns SET status = ?, actual_json = ?, updated_at = ? WHERE spawn_id = ?`, string(orchestration.SpawnSettled), actualJSON, formatTime(now), spawnID.String()); err != nil { return err }
	return tx.Commit()
}

const orchestrationSpawnSelect = `
SELECT spawn_id, parent_agent_id, COALESCE(child_agent_id, ''), root_agent_id, depth, task_intent, task_key,
       authority_json, reservation_id, reserved_json, status, created_at, updated_at
FROM orchestration_spawns`

func scanOrchestrationSpawn(scanner rowScanner) (orchestration.SpawnRecord, error) {
	var record orchestration.SpawnRecord
	var spawnID, parentID, childID, rootID, authorityJSON, reservationID, reservedJSON, status, createdAt, updatedAt string
	if err := scanner.Scan(&spawnID, &parentID, &childID, &rootID, &record.Depth, &record.TaskIntent, &record.TaskKey,
		&authorityJSON, &reservationID, &reservedJSON, &status, &createdAt, &updatedAt); err != nil { return record, err }
	record.ID = id.SpawnID(spawnID)
	record.ParentAgentID = id.AgentID(parentID)
	record.ChildAgentID = id.AgentID(childID)
	record.RootAgentID = id.AgentID(rootID)
	record.ReservationID = id.BudgetReservationID(reservationID)
	record.Status = orchestration.SpawnStatus(status)
	if err := json.Unmarshal([]byte(authorityJSON), &record.Authority); err != nil { return record, err }
	if err := json.Unmarshal([]byte(reservedJSON), &record.Reserved); err != nil { return record, err }
	var err error
	record.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); if err != nil { return record, err }
	record.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt); if err != nil { return record, err }
	return record, nil
}

func (s *Store) Spawn(ctx context.Context, spawnID id.SpawnID) (orchestration.SpawnRecord, error) {
	record, err := scanOrchestrationSpawn(s.db.QueryRowContext(ctx, orchestrationSpawnSelect+` WHERE spawn_id = ?`, spawnID.String()))
	if errors.Is(err, sql.ErrNoRows) { return record, errs.Wrap(errs.CodeNotFound, "sqlite.orchestration.spawn", "spawn not found", err) }
	return record, err
}

func (s *Store) FindReusableSpawn(ctx context.Context, root id.AgentID, key string) (orchestration.SpawnRecord, bool, error) {
	record, err := scanOrchestrationSpawn(s.db.QueryRowContext(ctx, orchestrationSpawnSelect+` WHERE root_agent_id = ? AND task_key = ? AND status = ? ORDER BY updated_at DESC LIMIT 1`, root.String(), key, string(orchestration.SpawnSettled)))
	if errors.Is(err, sql.ErrNoRows) { return record, false, nil }
	return record, err == nil, err
}

func (s *Store) CountChildren(ctx context.Context, parent id.AgentID) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM orchestration_spawns WHERE parent_agent_id = ? AND status IN (?, ?)`, parent.String(), string(orchestration.SpawnCreated), string(orchestration.SpawnSettled)).Scan(&count)
	return count, err
}

func (s *Store) CountDescendants(ctx context.Context, root id.AgentID) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
WITH RECURSIVE descendants(agent_id) AS (
  SELECT child_agent_id FROM orchestration_spawns WHERE parent_agent_id = ? AND child_agent_id IS NOT NULL AND status IN (?, ?)
  UNION ALL
  SELECT s.child_agent_id FROM orchestration_spawns s JOIN descendants d ON s.parent_agent_id = d.agent_id
  WHERE s.child_agent_id IS NOT NULL AND s.status IN (?, ?)
)
SELECT COUNT(*) FROM descendants`, root.String(), string(orchestration.SpawnCreated), string(orchestration.SpawnSettled), string(orchestration.SpawnCreated), string(orchestration.SpawnSettled)).Scan(&count)
	return count, err
}

func (s *Store) Descendants(ctx context.Context, root id.AgentID) ([]id.AgentID, error) {
	rows, err := s.db.QueryContext(ctx, `
WITH RECURSIVE descendants(agent_id, depth) AS (
  SELECT child_agent_id, 1 FROM orchestration_spawns WHERE parent_agent_id = ? AND child_agent_id IS NOT NULL
  UNION ALL
  SELECT s.child_agent_id, d.depth + 1 FROM orchestration_spawns s JOIN descendants d ON s.parent_agent_id = d.agent_id WHERE s.child_agent_id IS NOT NULL
)
SELECT agent_id FROM descendants ORDER BY depth DESC, agent_id`, root.String())
	if err != nil { return nil, err }
	defer rows.Close()
	var result []id.AgentID
	for rows.Next() { var value string; if err := rows.Scan(&value); err != nil { return nil, err }; result = append(result, id.AgentID(value)) }
	return result, rows.Err()
}
