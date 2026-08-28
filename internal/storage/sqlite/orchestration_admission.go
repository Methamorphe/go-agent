package sqlite

import (
	"context"
	"sort"
	"time"

	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/orchestration"
)

func (s *Store) Enqueue(ctx context.Context, admission orchestration.Admission) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO orchestration_admission(agent_id, root_id, priority, enqueued_at, active)
VALUES (?, ?, ?, ?, 0)
ON CONFLICT(agent_id) DO NOTHING`, admission.AgentID.String(), admission.RootID.String(), admission.Priority, formatTime(admission.EnqueuedAt))
	return err
}

func (s *Store) DequeueFair(ctx context.Context, globalSlots int, perRootSlots int) ([]orchestration.Admission, error) {
	if globalSlots <= 0 || perRootSlots <= 0 { return nil, nil }
	rows, err := s.db.QueryContext(ctx, `
SELECT agent_id, root_id, priority, enqueued_at
FROM orchestration_admission
WHERE active = 0
ORDER BY priority DESC, enqueued_at, root_id, agent_id
LIMIT ?`, globalSlots*64)
	if err != nil { return nil, err }
	defer rows.Close()

	var candidates []orchestration.Admission
	for rows.Next() {
		var admission orchestration.Admission
		var agentID, rootID, enqueuedAt string
		if err := rows.Scan(&agentID, &rootID, &admission.Priority, &enqueuedAt); err != nil { return nil, err }
		admission.AgentID = id.AgentID(agentID)
		admission.RootID = id.AgentID(rootID)
		admission.EnqueuedAt, err = time.Parse(time.RFC3339Nano, enqueuedAt)
		if err != nil { return nil, err }
		candidates = append(candidates, admission)
	}
	if err := rows.Err(); err != nil { return nil, err }

	counts := map[id.AgentID]int{}
	activeRows, err := s.db.QueryContext(ctx, `SELECT root_id, COUNT(*) FROM orchestration_admission WHERE active = 1 GROUP BY root_id`)
	if err != nil { return nil, err }
	for activeRows.Next() {
		var rootID string
		var count int
		if err := activeRows.Scan(&rootID, &count); err != nil { activeRows.Close(); return nil, err }
		counts[id.AgentID(rootID)] = count
	}
	if err := activeRows.Close(); err != nil { return nil, err }
	if err := activeRows.Err(); err != nil { return nil, err }

	// Stable ordering inside each root, then round-robin across roots by earliest queued item.
	byRoot := map[id.AgentID][]orchestration.Admission{}
	var roots []id.AgentID
	for _, candidate := range candidates {
		if _, ok := byRoot[candidate.RootID]; !ok { roots = append(roots, candidate.RootID) }
		byRoot[candidate.RootID] = append(byRoot[candidate.RootID], candidate)
	}
	sort.Slice(roots, func(i, j int) bool {
		left := byRoot[roots[i]][0]
		right := byRoot[roots[j]][0]
		if left.Priority != right.Priority { return left.Priority > right.Priority }
		if !left.EnqueuedAt.Equal(right.EnqueuedAt) { return left.EnqueuedAt.Before(right.EnqueuedAt) }
		return roots[i] < roots[j]
	})

	picked := make([]orchestration.Admission, 0, globalSlots)
	indices := map[id.AgentID]int{}
	for len(picked) < globalSlots {
		progress := false
		for _, rootID := range roots {
			if counts[rootID] >= perRootSlots { continue }
			index := indices[rootID]
			if index >= len(byRoot[rootID]) { continue }
			picked = append(picked, byRoot[rootID][index])
			indices[rootID] = index + 1
			counts[rootID]++
			progress = true
			if len(picked) == globalSlots { break }
		}
		if !progress { break }
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil { return nil, err }
	defer tx.Rollback()
	for _, admission := range picked {
		result, err := tx.ExecContext(ctx, `UPDATE orchestration_admission SET active = 1, admitted_at = ? WHERE agent_id = ? AND active = 0`, formatTime(admission.EnqueuedAt), admission.AgentID.String())
		if err != nil { return nil, err }
		rows, _ := result.RowsAffected()
		if rows != 1 { return nil, context.Canceled }
	}
	if err := tx.Commit(); err != nil { return nil, err }
	return picked, nil
}

func (s *Store) MarkAdmitted(ctx context.Context, agentID id.AgentID, active bool, at time.Time) error {
	value := 0
	if active { value = 1 }
	_, err := s.db.ExecContext(ctx, `UPDATE orchestration_admission SET active = ?, admitted_at = ? WHERE agent_id = ?`, value, formatTime(at), agentID.String())
	return err
}

func (s *Store) ActiveByRoot(ctx context.Context, root id.AgentID) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM orchestration_admission WHERE root_id = ? AND active = 1`, root.String()).Scan(&count)
	return count, err
}

var _ orchestration.Store = (*Store)(nil)
