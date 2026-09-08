package process

import (
	"context"

	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
)

// ForkTimeline materializes a new Agent Process lineage from a source process
// without changing, truncating, or replaying over the source ledger. The new
// timeline inherits the immutable root intent and causal parentage, but starts
// READY with a new AgentID and its own future event stream.
func (s *Service) ForkTimeline(
	ctx context.Context,
	sourceAgentID id.AgentID,
	expectedSourceVersion *uint64,
	meta CommandMeta,
) (State, error) {
	meta, err := s.normalizeMeta(meta)
	if err != nil { return State{}, err }
	if state, ok, err := s.receiptState(ctx, meta.RequestID, "process.fork_timeline", ""); err != nil || ok {
		return state, err
	}
	source, err := s.store.Current(ctx, sourceAgentID)
	if err != nil { return State{}, err }
	if expectedSourceVersion != nil && *expectedSourceVersion != source.Version {
		return State{}, versionConflict("process.fork_timeline", *expectedSourceVersion, source.Version)
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
