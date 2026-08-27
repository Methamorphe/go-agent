package ledger

import (
	"encoding/json"
	"time"

	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
)

const MaxInlinePayloadBytes = 64 << 10

type EventType string

type ActorKind string

const (
	ActorUser       ActorKind = "user"
	ActorAgent      ActorKind = "agent"
	ActorSupervisor ActorKind = "supervisor"
	ActorScheduler  ActorKind = "scheduler"
	ActorPolicy     ActorKind = "policy"
	ActorSystem     ActorKind = "system"
	ActorExternal   ActorKind = "external"
)

type ActorRef struct {
	Kind ActorKind `json:"kind"`
	ID   string    `json:"id,omitempty"`
}

type Event struct {
	ID             id.EventID       `json:"id"`
	LedgerSequence uint64           `json:"ledger_sequence"`
	AgentID        id.AgentID       `json:"agent_id"`
	RootAgentID    id.AgentID       `json:"root_agent_id"`
	ProcessVersion uint64           `json:"process_version"`
	Type           EventType        `json:"type"`
	SchemaVersion  uint32           `json:"schema_version"`
	Timestamp      time.Time        `json:"timestamp"`
	CausationID    *id.EventID      `json:"causation_id,omitempty"`
	CorrelationID  id.CorrelationID `json:"correlation_id"`
	Actor          ActorRef         `json:"actor"`
	Payload        json.RawMessage  `json:"payload"`
}

func (e Event) Validate() error {
	if e.ID == "" ||
		e.AgentID == "" ||
		e.RootAgentID == "" ||
		e.CorrelationID == "" {
		return errs.New(
			errs.CodeInvalidArgument,
			"ledger.event.validate",
			"event identity fields are required",
		)
	}

	if e.ProcessVersion == 0 {
		return errs.New(
			errs.CodeInvalidArgument,
			"ledger.event.validate",
			"process_version must be greater than zero",
		)
	}

	if e.Type == "" {
		return errs.New(
			errs.CodeInvalidArgument,
			"ledger.event.validate",
			"event type is required",
		)
	}

	if e.Timestamp.IsZero() {
		return errs.New(
			errs.CodeInvalidArgument,
			"ledger.event.validate",
			"timestamp is required",
		)
	}

	if e.Actor.Kind == "" {
		return errs.New(
			errs.CodeInvalidArgument,
			"ledger.event.validate",
			"actor kind is required",
		)
	}

	if len(e.Payload) > MaxInlinePayloadBytes {
		return errs.New(
			errs.CodeResourceExhausted,
			"ledger.event.validate",
			"event payload exceeds inline limit",
		)
	}

	return nil
}
