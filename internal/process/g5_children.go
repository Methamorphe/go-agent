package process

import (
	"context"
	"strings"

	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
)

// CreateDelegatedChild creates a durable child with its own immutable Task Intent.
// Resource/authority validation is performed by the G5 orchestration service before this call.
func (s *Service) CreateDelegatedChild(ctx context.Context, parentID id.AgentID, taskGoal string, meta CommandMeta) (State, error) {
	meta, err := s.normalizeMeta(meta)
	if err != nil { return State{}, err }
	if state, ok, err := s.receiptState(ctx, meta.RequestID, "process.create_delegated_child", ""); err != nil || ok { return state, err }

	parent, err := s.store.Current(ctx, parentID)
	if err != nil { return State{}, err }
	if parent.Status.Terminal() { return State{}, errs.New(errs.CodeConflict, "process.create_delegated_child", "cannot create child from terminal parent") }
	if parent.RootIntent == nil { return State{}, errs.New(errs.CodeCorruption, "process.create_delegated_child", "parent has no bound root intent") }

	taskGoal = strings.TrimSpace(taskGoal)
	if taskGoal == "" { return State{}, errs.New(errs.CodeInvalidArgument, "process.create_delegated_child", "task intent goal must not be empty") }
	if meta.CausationID == nil && parent.LastEventID != "" { meta.CausationID = eventPtr(parent.LastEventID) }

	agentID, err := s.ids.Agent()
	if err != nil { return State{}, wrapID("process.create_delegated_child", err) }
	intentID, err := s.ids.Intent()
	if err != nil { return State{}, wrapID("process.create_delegated_child", err) }
	intent := Intent{ID: intentID, SchemaVersion: IntentSchemaVersion, Goal: taskGoal, CreatedAt: s.clock.Now().UTC()}

	return s.create(ctx, agentID, parent.RootAgentID, &parent.AgentID, parent.LineageDepth+1, intent, meta, "process.create_delegated_child")
}
