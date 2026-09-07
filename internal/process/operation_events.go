package process

import (
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/ledger"
)

const (
	EventCognitiveRoutingDecided ledger.EventType = "CognitiveRoutingDecided"
	EventModelInvocationStarted   ledger.EventType = "ModelInvocationStarted"
	EventModelInvocationCompleted ledger.EventType = "ModelInvocationCompleted"
	EventModelInvocationFailed    ledger.EventType = "ModelInvocationFailed"
	EventSyscallRequested         ledger.EventType = "SyscallRequested"
	EventSyscallCompleted         ledger.EventType = "SyscallCompleted"
	EventSyscallFailed            ledger.EventType = "SyscallFailed"
)

type CognitiveRoutingDecidedPayload struct {
	DecisionID           string `json:"decision_id"`
	TaskID               string `json:"task_id"`
	Provider             string `json:"provider"`
	Model                string `json:"model"`
	ProfileVersion       uint32 `json:"profile_version"`
	DecisionRef          string `json:"decision_ref"`
	ReservationID        string `json:"reservation_id"`
	EstimatedMoneyMicros int64  `json:"estimated_money_micros"`
	EstimatedTokens      int64  `json:"estimated_tokens"`
}

type ModelUsage struct {
	InputTokens  *int64 `json:"input_tokens,omitempty"`
	OutputTokens *int64 `json:"output_tokens,omitempty"`
	TotalTokens  *int64 `json:"total_tokens,omitempty"`
}

type ModelInvocationStartedPayload struct {
	InvocationID id.InvocationID `json:"invocation_id"`
	Provider     string          `json:"provider"`
	Model        string          `json:"model"`
	RequestRef   string          `json:"request_ref"`
}

type ModelInvocationCompletedPayload struct {
	InvocationID     id.InvocationID `json:"invocation_id"`
	ResponseRef      string          `json:"response_ref"`
	ProviderResultID string          `json:"provider_result_id,omitempty"`
	FinishReason     string          `json:"finish_reason"`
	Usage            ModelUsage      `json:"usage"`
	ToolCallCount    int             `json:"tool_call_count,omitempty"`
}

type ModelInvocationFailedPayload struct {
	InvocationID       id.InvocationID `json:"invocation_id"`
	PartialResponseRef string          `json:"partial_response_ref,omitempty"`
	Failure            string          `json:"failure"`
}

type SyscallRequestedPayload struct {
	ActionID     id.ActionID `json:"action_id"`
	Name         string      `json:"name"`
	ArgumentsRef string      `json:"arguments_ref"`
}

type SyscallCompletedPayload struct {
	ActionID  id.ActionID `json:"action_id"`
	Name      string      `json:"name"`
	Status    string      `json:"status"`
	ResultRef string      `json:"result_ref"`
}

type SyscallFailedPayload struct {
	ActionID id.ActionID `json:"action_id"`
	Name     string      `json:"name"`
	Failure  string      `json:"failure"`
}

func validateOperationEvent(state State, event ledger.Event) (bool, error) {
	if state.Status != StatusRunning {
		switch event.Type {
		case EventCognitiveRoutingDecided,
			EventModelInvocationStarted,
			EventModelInvocationCompleted,
			EventModelInvocationFailed,
			EventSyscallRequested,
			EventSyscallCompleted,
			EventSyscallFailed:
			return true, impossible(event, "operation event requires RUNNING")
		default:
			return false, nil
		}
	}

	switch event.Type {
	case EventCognitiveRoutingDecided:
		payload, err := decodePayload[CognitiveRoutingDecidedPayload](event)
		if err != nil {
			return true, err
		}
		if payload.DecisionID == "" || payload.TaskID == "" || payload.Provider == "" || payload.Model == "" || payload.ProfileVersion == 0 || payload.DecisionRef == "" || payload.ReservationID == "" {
			return true, impossible(event, "cognitive routing decision payload is incomplete")
		}
		if payload.EstimatedMoneyMicros < 0 || payload.EstimatedTokens < 0 {
			return true, impossible(event, "cognitive routing estimate must be non-negative")
		}
		return true, nil

	case EventModelInvocationStarted:
		payload, err := decodePayload[ModelInvocationStartedPayload](event)
		if err != nil {
			return true, err
		}
		if payload.InvocationID == "" || payload.Provider == "" || payload.Model == "" || payload.RequestRef == "" {
			return true, impossible(event, "model invocation start payload is incomplete")
		}
		return true, nil

	case EventModelInvocationCompleted:
		payload, err := decodePayload[ModelInvocationCompletedPayload](event)
		if err != nil {
			return true, err
		}
		if payload.InvocationID == "" || payload.ResponseRef == "" || payload.FinishReason == "" {
			return true, impossible(event, "model invocation completion payload is incomplete")
		}
		return true, nil

	case EventModelInvocationFailed:
		payload, err := decodePayload[ModelInvocationFailedPayload](event)
		if err != nil {
			return true, err
		}
		if payload.InvocationID == "" || payload.Failure == "" {
			return true, impossible(event, "model invocation failure payload is incomplete")
		}
		return true, nil

	case EventSyscallRequested:
		payload, err := decodePayload[SyscallRequestedPayload](event)
		if err != nil {
			return true, err
		}
		if payload.ActionID == "" || payload.Name == "" || payload.ArgumentsRef == "" {
			return true, impossible(event, "syscall request payload is incomplete")
		}
		return true, nil

	case EventSyscallCompleted:
		payload, err := decodePayload[SyscallCompletedPayload](event)
		if err != nil {
			return true, err
		}
		if payload.ActionID == "" || payload.Name == "" || payload.Status == "" || payload.ResultRef == "" {
			return true, impossible(event, "syscall completion payload is incomplete")
		}
		return true, nil

	case EventSyscallFailed:
		payload, err := decodePayload[SyscallFailedPayload](event)
		if err != nil {
			return true, err
		}
		if payload.ActionID == "" || payload.Name == "" || payload.Failure == "" {
			return true, impossible(event, "syscall failure payload is incomplete")
		}
		return true, nil

	default:
		return false, nil
	}
}
