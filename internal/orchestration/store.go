package orchestration

import (
	"context"
	"time"

	"github.com/Methamorphe/go-agent/internal/id"
)

type Store interface {
	BootstrapRoot(context.Context, id.AgentID, RootPolicy, time.Time) error
	Account(context.Context, id.AgentID) (Account, error)
	ReserveSpawn(context.Context, SpawnRecord) error
	BindSpawnChild(context.Context, id.SpawnID, id.AgentID, time.Time) error
	RejectSpawn(context.Context, id.SpawnID, string, time.Time) error
	SettleSpawn(context.Context, id.SpawnID, Budget, time.Time) error
	Spawn(context.Context, id.SpawnID) (SpawnRecord, error)
	FindReusableSpawn(context.Context, id.AgentID, string) (SpawnRecord, bool, error)
	CountChildren(context.Context, id.AgentID) (int, error)
	CountDescendants(context.Context, id.AgentID) (int, error)
	Descendants(context.Context, id.AgentID) ([]id.AgentID, error)

	PutMessage(context.Context, AgentMessage, int, int64) (bool, error)
	Mailbox(context.Context, id.AgentID, int) ([]AgentMessage, error)
	ConsumeMessage(context.Context, id.AgentID, id.MessageID, time.Time) (bool, error)

	PutWait(context.Context, WaitSpec) error
	DeleteWait(context.Context, id.WaitID) error
	WaitEdges(context.Context, id.AgentID) ([]id.AgentID, error)
	WaitsForChild(context.Context, id.AgentID) ([]WaitSpec, error)

	PutResult(context.Context, ChildResult) error
	Result(context.Context, id.AgentID) (ChildResult, bool, error)

	Enqueue(context.Context, Admission) error
	DequeueFair(context.Context, int, int) ([]Admission, error)
	MarkAdmitted(context.Context, id.AgentID, bool, time.Time) error
	ActiveByRoot(context.Context, id.AgentID) (int, error)
}
