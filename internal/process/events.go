package process

import (
	"encoding/json"
	"time"

	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/ledger"
)

const EventSchemaVersion uint32 = 1

const (
	EventAgentCreated          ledger.EventType = "AgentCreated"
	EventRootIntentBound       ledger.EventType = "RootIntentBound"
	EventAgentReadied          ledger.EventType = "AgentReadied"
	EventAgentActivated        ledger.EventType = "AgentActivated"
	EventAgentYielded          ledger.EventType = "AgentYielded"
	EventAgentWaitStarted      ledger.EventType = "AgentWaitStarted"
	EventAgentWaitSatisfied    ledger.EventType = "AgentWaitSatisfied"
	EventAgentSleepStarted     ledger.EventType = "AgentSleepStarted"
	EventAgentWoken            ledger.EventType = "AgentWoken"
	EventCancelRequested       ledger.EventType = "CancelRequested"
	EventAgentCancelled        ledger.EventType = "AgentCancelled"
	EventAgentSuspended        ledger.EventType = "AgentSuspended"
	EventAgentResumed          ledger.EventType = "AgentResumed"
	EventAgentCompleted        ledger.EventType = "AgentCompleted"
	EventAgentFailed           ledger.EventType = "AgentFailed"
	EventCheckpointCreated     ledger.EventType = "CheckpointCreated"
	EventRecoveryActionApplied ledger.EventType = "RecoveryActionApplied"
)

type AgentCreatedPayload struct {
	ParentAgentID *id.AgentID `json:"parent_agent_id,omitempty"`
	LineageDepth  uint32      `json:"lineage_depth"`
}

// Exemple volontaire de compatibilité historique.
// Imaginons que le schema 0 n'avait pas encore LineageDepth.
type agentCreatedPayloadV0 struct {
	ParentAgentID *id.AgentID `json:"parent_agent_id,omitempty"`
}

type RootIntentBoundPayload struct {
	Intent Intent `json:"intent"`
}

type AgentReadiedPayload struct {
	Reason string `json:"reason,omitempty"`
}

type AgentActivatedPayload struct {
	LeaseID           id.LeaseID           `json:"lease_id"`
	RuntimeInstanceID id.RuntimeInstanceID `json:"runtime_instance_id"`
}

type AgentYieldedPayload struct {
	Reason string `json:"reason,omitempty"`
}

type AgentWaitStartedPayload struct {
	Reason    WaitingReason `json:"reason"`
	Reference string        `json:"reference,omitempty"`
}

type AgentWaitSatisfiedPayload struct {
	Reference string `json:"reference,omitempty"`
}

type AgentSleepStartedPayload struct {
	SleepID    id.SleepID `json:"sleep_id"`
	WakeAt     time.Time  `json:"wake_at"`
	Generation uint64     `json:"generation"`
}

type AgentWokenPayload struct {
	SleepID    id.SleepID `json:"sleep_id"`
	Generation uint64     `json:"generation"`
}

type CancelRequestedPayload struct {
	Scope string `json:"scope,omitempty"`
}

type AgentCancelledPayload struct {
	Reason string `json:"reason,omitempty"`
}

type AgentSuspendedPayload struct {
	Reason string `json:"reason,omitempty"`
}

type AgentResumedPayload struct {
	Reason string `json:"reason,omitempty"`
}

type AgentCompletedPayload struct {
	SummaryRef string `json:"summary_ref,omitempty"`
}

type AgentFailedPayload struct {
	Failure Failure `json:"failure"`
}

type CheckpointCreatedPayload struct {
	CheckpointID id.CheckpointID `json:"checkpoint_id"`
}

type RecoveryActionAppliedPayload struct {
	Action         string `json:"action"`
	PreviousStatus Status `json:"previous_status"`
}

func encodePayload(value any) (json.RawMessage, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, errs.Wrap(
			errs.CodeInvalidArgument,
			"process.event.encode",
			"marshal event payload",
			err,
		)
	}

	if len(body) > ledger.MaxInlinePayloadBytes {
		return nil, errs.New(
			errs.CodeResourceExhausted,
			"process.event.encode",
			"event payload exceeds inline limit",
		)
	}

	return body, nil
}

func decodePayload[T any](
	event ledger.Event,
) (T, error) {
	var zero T

	if event.SchemaVersion != EventSchemaVersion {
		return zero, errs.New(
			errs.CodeUnsupported,
			"process.event.decode",
			"unsupported event schema version",
		)
	}

	var payload T

	if err := json.Unmarshal(
		event.Payload,
		&payload,
	); err != nil {
		return zero, errs.Wrap(
			errs.CodeCorruption,
			"process.event.decode",
			"decode event payload",
			err,
		)
	}

	return payload, nil
}

func decodeAgentCreated(
	event ledger.Event,
) (AgentCreatedPayload, error) {
	switch event.SchemaVersion {
	case 0:
		var old agentCreatedPayloadV0

		if err := json.Unmarshal(
			event.Payload,
			&old,
		); err != nil {
			return AgentCreatedPayload{},
				errs.Wrap(
					errs.CodeCorruption,
					"process.event.decode_agent_created",
					"decode v0 payload",
					err,
				)
		}

		var depth uint32

		if old.ParentAgentID != nil {
			depth = 1
		}

		return AgentCreatedPayload{
			ParentAgentID: old.ParentAgentID,
			LineageDepth:  depth,
		}, nil

	case EventSchemaVersion:
		return decodePayload[AgentCreatedPayload](
			event,
		)

	default:
		return AgentCreatedPayload{},
			errs.New(
				errs.CodeUnsupported,
				"process.event.decode_agent_created",
				"unsupported AgentCreated schema version",
			)
	}
}
