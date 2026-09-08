package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/memory"
)

func (s *Store) PromotedForkBelief(ctx context.Context, forkID id.ForkID, fingerprint string, scope memory.Scope) (memory.Belief, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT `+memoryBeliefColumns+`
FROM memory_beliefs
WHERE origin_fork_id = ? AND fingerprint = ? AND scope_kind = ? AND scope_ref = ?
LIMIT 1`, forkID.String(), fingerprint, string(scope.Kind), scope.Ref)
	value, err := scanMemoryBelief(row)
	if errors.Is(err, sql.ErrNoRows) {
		return memory.Belief{}, memory.ErrNotFound
	}
	if err != nil {
		return memory.Belief{}, errs.Wrap(errs.CodeCorruption, "sqlite.memory.fork_lookup", "scan promoted fork belief", err)
	}
	return value, nil
}
