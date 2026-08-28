package orchestration

import (
	"context"

	"github.com/Methamorphe/go-agent/internal/id"
)

// ChildTerminal reconciles every durable wait that depends on childID.
// It is idempotent: already-satisfied waits are absent from the durable wait table.
func (s *Service) ChildTerminal(ctx context.Context, childID id.AgentID) (int, error) {
	waits, err := s.store.WaitsForChild(ctx, childID)
	if err != nil {
		return 0, err
	}
	woken := 0
	for _, wait := range waits {
		done, err := s.EvaluateWait(ctx, wait)
		if err != nil {
			return woken, err
		}
		if done {
			woken++
		}
	}
	return woken, nil
}

// SettleAndNotify persists a structured child result, settles its reservation,
// then reconciles durable parents waiting on that child.
func (s *Service) SettleAndNotify(ctx context.Context, spawnID id.SpawnID, actual Budget, result ChildResult) (int, error) {
	if err := s.Settle(ctx, spawnID, actual, result); err != nil {
		return 0, err
	}
	record, err := s.store.Spawn(ctx, spawnID)
	if err != nil {
		return 0, err
	}
	return s.ChildTerminal(ctx, record.ChildAgentID)
}
