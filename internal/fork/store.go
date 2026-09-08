package fork

import (
	"context"

	"github.com/Methamorphe/go-agent/internal/id"
)

type Store interface {
	PutCheckpoint(context.Context, Checkpoint) error
	Checkpoint(context.Context, id.CheckpointID) (Checkpoint, error)
	CreateGroup(context.Context, Group) error
	Group(context.Context, id.ForkGroupID) (Group, error)
	UpdateGroup(context.Context, Group) error
	CreateBranch(context.Context, Branch) error
	Branch(context.Context, id.ForkID) (Branch, error)
	UpdateBranch(context.Context, Branch) error
	Branches(context.Context, id.ForkGroupID) ([]Branch, error)
	DeleteBranch(context.Context, id.ForkID) error
	DeleteGroup(context.Context, id.ForkGroupID) error
	DeleteCheckpoint(context.Context, id.CheckpointID) error
}
