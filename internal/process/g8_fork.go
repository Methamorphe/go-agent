package process

import (
	"context"

	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
)

// ForkTimeline materializes a new Agent Process lineage from the current source
// state without changing the source ledger.
func (s *Service) ForkTimeline(ctx context.Context, sourceAgentID id.AgentID, expectedSourceVersion *uint64, meta CommandMeta) (State, error) {
	source, err := s.store.Current(ctx, sourceAgentID)
	if err != nil { return State{}, err }
	if expectedSourceVersion != nil && *expectedSourceVersion != source.Version {
		return State{}, versionConflict("process.fork_timeline", *expectedSourceVersion, source.Version)
	}
	return s.ForkTimelineFromState(ctx, source, meta)
}

// ForkTimelineFromState creates a fresh process from an immutable checkpoint
// projection. The source projection may be historical; no current source read
// is performed, so restore-as-new-timeline remains valid after the source has
// advanced. History is linked causally and never rewritten.
func (s *Service) ForkTimelineFromState(ctx context.Context, source State, meta CommandMeta) (State, error) {
	meta, err := s.normalizeMeta(meta)
	if err != nil { return State{}, err }
	if state, ok, err := s.receiptState(ctx, meta.RequestID, "process.fork_timeline", ""); err != nil || ok { return state, err }
	if source.AgentID == "" || source.RootAgentID == "" || source.Version == 0 {
		return State{}, errs.New(errs.CodeInvalidArgument, "process.fork_timeline", "source checkpoint state is incomplete")
	}
	if source.Status.Terminal() {
		return State{}, errs.New(errs.CodeConflict, "process.fork_timeline", "cannot fork a terminal process")
	}
	if source.RootIntent == nil {
		return State{}, errs.New(errs.CodeCorruption, "process.fork_timeline", "source process has no root intent")
	}
	if meta.CausationID == nil && source.LastEventID != "" { meta.CausationID = eventPtr(source.LastEventID) }
	agentID, err := s.ids.Agent()
	if err != nil { return State{}, wrapID("process.fork_timeline", err) }
	intent := *source.RootIntent
	return s.create(ctx, agentID, source.RootAgentID, &source.AgentID, source.LineageDepth+1, intent, meta, "process.fork_timeline")
}
