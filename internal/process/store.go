package process

import (
	"context"
	"time"

	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/ledger"
)

type Store interface {
	Current(
		context.Context,
		id.AgentID,
	) (State, error)

	Append(
		context.Context,
		uint64,
		[]ledger.Event,
		State,
		Receipt,
	) error

	Events(
		context.Context,
		id.AgentID,
		uint64,
		int,
	) ([]ledger.Event, error)

	Receipt(
		context.Context,
		id.RequestID,
	) (Receipt, bool, error)

	SaveSnapshot(
		context.Context,
		Snapshot,
	) error

	LatestSnapshot(
		context.Context,
		id.AgentID,
	) (Snapshot, bool, error)

	LedgerSequenceAtVersion(
		context.Context,
		id.AgentID,
		uint64,
	) (uint64, error)

	ListByStatus(
		context.Context,
		Status,
		int,
	) ([]State, error)

	DueSleeps(
		context.Context,
		time.Time,
		int,
	) ([]SleepDue, error)
}
