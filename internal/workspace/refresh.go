package workspace

import (
	"context"

	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/ledger"
	agentprocess "github.com/Methamorphe/go-agent/internal/process"
)

type incrementalProjectionStore interface {
	WorkspaceCursor(context.Context, id.AgentID) (uint64, error)
	ProcessTreeAfter(context.Context, id.AgentID, uint64, int) ([]agentprocess.State, error)
	WorkspaceEventsAfter(context.Context, id.AgentID, uint64, int) ([]ledger.Event, error)
}

func (s *Service) Refresh(ctx context.Context, request RefreshRequest) (Refresh, error) {
	if request.RootAgentID == "" || request.FocusedAgentID == "" {
		return Refresh{}, errs.New(errs.CodeInvalidArgument, "workspace.refresh", "root and focused agent ids are required")
	}
	store, ok := s.store.(incrementalProjectionStore)
	if !ok {
		return Refresh{}, errs.New(errs.CodeUnsupported, "workspace.refresh", "incremental workspace projections are unavailable")
	}
	limit, err := pageLimit(request.Limit)
	if err != nil {
		return Refresh{}, err
	}
	cursor, err := store.WorkspaceCursor(ctx, request.RootAgentID)
	if err != nil {
		return Refresh{}, err
	}
	states, err := store.ProcessTreeAfter(ctx, request.RootAgentID, request.AfterSequence, limit+1)
	if err != nil {
		return Refresh{}, err
	}
	events, err := store.WorkspaceEventsAfter(ctx, request.FocusedAgentID, request.AfterSequence, limit+1)
	if err != nil {
		return Refresh{}, err
	}
	behind := len(states) > limit || len(events) > limit
	if len(states) > limit {
		states = states[:limit]
	}
	if len(events) > limit {
		events = events[:limit]
	}
	blocks := make([]Block, 0, len(events))
	for _, event := range events {
		blocks = append(blocks, blockFromEvent(event))
	}
	result := Refresh{
		Cursor: cursor,
		TreePatch: summarizeTree(states),
		Blocks: blocks,
		Behind: behind,
		GeneratedAt: s.now().UTC(),
	}
	if request.Inspector {
		inspector, err := s.store.WorkspaceInspector(ctx, request.RootAgentID)
		if err != nil {
			return Refresh{}, err
		}
		result.Inspector = &inspector
	}
	return result, nil
}

func (s *Service) cursor(ctx context.Context, rootID id.AgentID) (uint64, error) {
	store, ok := s.store.(incrementalProjectionStore)
	if !ok {
		return 0, nil
	}
	return store.WorkspaceCursor(ctx, rootID)
}
