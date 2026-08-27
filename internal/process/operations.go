package process

import (
	"context"

	"github.com/Methamorphe/go-agent/internal/id"
)

func (s *Service) ModelInvocationStarted(
	ctx context.Context,
	agentID id.AgentID,
	expected *uint64,
	payload ModelInvocationStartedPayload,
	meta CommandMeta,
) (State, error) {
	return s.transition(ctx, "process.model_invocation_started", agentID, expected, EventModelInvocationStarted, payload, meta)
}

func (s *Service) ModelInvocationCompleted(
	ctx context.Context,
	agentID id.AgentID,
	expected *uint64,
	payload ModelInvocationCompletedPayload,
	meta CommandMeta,
) (State, error) {
	return s.transition(ctx, "process.model_invocation_completed", agentID, expected, EventModelInvocationCompleted, payload, meta)
}

func (s *Service) ModelInvocationFailed(
	ctx context.Context,
	agentID id.AgentID,
	expected *uint64,
	payload ModelInvocationFailedPayload,
	meta CommandMeta,
) (State, error) {
	return s.transition(ctx, "process.model_invocation_failed", agentID, expected, EventModelInvocationFailed, payload, meta)
}

func (s *Service) SyscallRequested(
	ctx context.Context,
	agentID id.AgentID,
	expected *uint64,
	payload SyscallRequestedPayload,
	meta CommandMeta,
) (State, error) {
	return s.transition(ctx, "process.syscall_requested", agentID, expected, EventSyscallRequested, payload, meta)
}

func (s *Service) SyscallCompleted(
	ctx context.Context,
	agentID id.AgentID,
	expected *uint64,
	payload SyscallCompletedPayload,
	meta CommandMeta,
) (State, error) {
	return s.transition(ctx, "process.syscall_completed", agentID, expected, EventSyscallCompleted, payload, meta)
}

func (s *Service) SyscallFailed(
	ctx context.Context,
	agentID id.AgentID,
	expected *uint64,
	payload SyscallFailedPayload,
	meta CommandMeta,
) (State, error) {
	return s.transition(ctx, "process.syscall_failed", agentID, expected, EventSyscallFailed, payload, meta)
}
