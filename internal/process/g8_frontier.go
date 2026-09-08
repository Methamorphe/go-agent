package process

import (
	"context"
	"sort"

	"github.com/Methamorphe/go-agent/internal/id"
)

// ExecutionFrontier is the process-owned portion of a forkable checkpoint.
// It is derived from the durable event stream rather than in-memory runner
// state so restart does not change edit-safety decisions.
type ExecutionFrontier struct {
	AgentID               id.AgentID       `json:"agent_id"`
	ProcessVersion         uint64           `json:"process_version"`
	LedgerSequence         uint64           `json:"ledger_sequence"`
	LastInvocationID       id.InvocationID  `json:"last_invocation_id,omitempty"`
	InFlightInvocations    []id.InvocationID `json:"in_flight_invocations,omitempty"`
	InFlightActions        []id.ActionID     `json:"in_flight_actions,omitempty"`
	CompletedResultRefs    []string          `json:"completed_result_refs,omitempty"`
}

func (s *Service) ExecutionFrontier(ctx context.Context, agentID id.AgentID) (ExecutionFrontier, error) {
	state, err := s.store.Current(ctx, agentID)
	if err != nil { return ExecutionFrontier{}, err }
	sequence, err := s.store.LedgerSequenceAtVersion(ctx, agentID, state.Version)
	if err != nil { return ExecutionFrontier{}, err }

	inflightInv := make(map[id.InvocationID]struct{})
	inflightActions := make(map[id.ActionID]struct{})
	resultSet := make(map[string]struct{})
	var lastInvocation id.InvocationID
	var after uint64
	for {
		events, err := s.store.Events(ctx, agentID, after, 256)
		if err != nil { return ExecutionFrontier{}, err }
		if len(events) == 0 { break }
		for _, event := range events {
			switch event.Type {
			case EventModelInvocationStarted:
				payload, err := decodePayload[ModelInvocationStartedPayload](event)
				if err != nil { return ExecutionFrontier{}, err }
				inflightInv[payload.InvocationID] = struct{}{}
				lastInvocation = payload.InvocationID
			case EventModelInvocationCompleted:
				payload, err := decodePayload[ModelInvocationCompletedPayload](event)
				if err != nil { return ExecutionFrontier{}, err }
				delete(inflightInv, payload.InvocationID)
				lastInvocation = payload.InvocationID
				if payload.ResponseRef != "" { resultSet[payload.ResponseRef] = struct{}{} }
			case EventModelInvocationFailed:
				payload, err := decodePayload[ModelInvocationFailedPayload](event)
				if err != nil { return ExecutionFrontier{}, err }
				delete(inflightInv, payload.InvocationID)
				lastInvocation = payload.InvocationID
				if payload.PartialResponseRef != "" { resultSet[payload.PartialResponseRef] = struct{}{} }
			case EventSyscallRequested:
				payload, err := decodePayload[SyscallRequestedPayload](event)
				if err != nil { return ExecutionFrontier{}, err }
				inflightActions[payload.ActionID] = struct{}{}
			case EventSyscallCompleted:
				payload, err := decodePayload[SyscallCompletedPayload](event)
				if err != nil { return ExecutionFrontier{}, err }
				delete(inflightActions, payload.ActionID)
				if payload.ResultRef != "" { resultSet[payload.ResultRef] = struct{}{} }
			case EventSyscallFailed:
				payload, err := decodePayload[SyscallFailedPayload](event)
				if err != nil { return ExecutionFrontier{}, err }
				delete(inflightActions, payload.ActionID)
			}
			after = event.ProcessVersion
		}
		if len(events) < 256 { break }
	}

	frontier := ExecutionFrontier{AgentID: agentID, ProcessVersion: state.Version, LedgerSequence: sequence, LastInvocationID: lastInvocation}
	for invocationID := range inflightInv { frontier.InFlightInvocations = append(frontier.InFlightInvocations, invocationID) }
	for actionID := range inflightActions { frontier.InFlightActions = append(frontier.InFlightActions, actionID) }
	for ref := range resultSet { frontier.CompletedResultRefs = append(frontier.CompletedResultRefs, ref) }
	sort.Slice(frontier.InFlightInvocations, func(i, j int) bool { return frontier.InFlightInvocations[i].String() < frontier.InFlightInvocations[j].String() })
	sort.Slice(frontier.InFlightActions, func(i, j int) bool { return frontier.InFlightActions[i].String() < frontier.InFlightActions[j].String() })
	sort.Strings(frontier.CompletedResultRefs)
	return frontier, nil
}
