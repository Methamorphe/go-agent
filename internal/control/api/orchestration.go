package api

import (
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/orchestration"
)

const (
	TypeOrchestrationBootstrap = "orchestration.bootstrap"
	TypeOrchestrationSpawn = "orchestration.spawn"
	TypeOrchestrationWait = "orchestration.wait"
	TypeOrchestrationSend = "orchestration.send"
	TypeOrchestrationMailbox = "orchestration.mailbox"
	TypeOrchestrationConsume = "orchestration.consume"
	TypeOrchestrationSettle = "orchestration.settle"
	TypeOrchestrationChildTerminal = "orchestration.child_terminal"
	TypeOrchestrationCancelTree = "orchestration.cancel_tree"
	TypeOrchestrationAdmit = "orchestration.admit"
)

type OrchestrationBootstrapRequest struct {
	AgentID id.AgentID `json:"agent_id"`
	Policy orchestration.RootPolicy `json:"policy"`
}

type OrchestrationSpawnRequest struct { Spec orchestration.SpawnSpec `json:"spec"` }
type OrchestrationSpawnResponse struct { Outcome orchestration.SpawnOutcome `json:"outcome"` }

type OrchestrationWaitRequest struct {
	ParentID id.AgentID `json:"parent_id"`
	Children []id.AgentID `json:"children"`
	Mode orchestration.WaitMode `json:"mode"`
	Quorum int `json:"quorum,omitempty"`
	Failure orchestration.FailurePolicy `json:"failure"`
}
type OrchestrationWaitResponse struct { Wait orchestration.WaitSpec `json:"wait"` }

type OrchestrationSendRequest struct { Message orchestration.AgentMessage `json:"message"` }
type OrchestrationSendResponse struct { Message orchestration.AgentMessage `json:"message"`; Inserted bool `json:"inserted"` }

type OrchestrationMailboxRequest struct { AgentID id.AgentID `json:"agent_id"`; Limit int `json:"limit,omitempty"` }
type OrchestrationMailboxResponse struct { Messages []orchestration.AgentMessage `json:"messages"` }

type OrchestrationConsumeRequest struct { AgentID id.AgentID `json:"agent_id"`; MessageID id.MessageID `json:"message_id"` }
type OrchestrationConsumeResponse struct { Consumed bool `json:"consumed"` }

type OrchestrationSettleRequest struct {
	SpawnID id.SpawnID `json:"spawn_id"`
	Actual orchestration.Budget `json:"actual"`
	Result orchestration.ChildResult `json:"result"`
}

type OrchestrationChildTerminalRequest struct { ChildID id.AgentID `json:"child_id"` }
type OrchestrationChildTerminalResponse struct { Woken int `json:"woken"` }

type OrchestrationCancelTreeRequest struct { AgentID id.AgentID `json:"agent_id"`; Reason string `json:"reason,omitempty"` }

type OrchestrationAdmitRequest struct { GlobalSlots int `json:"global_slots"` }
type OrchestrationAdmitResponse struct { Admissions []orchestration.Admission `json:"admissions"` }

type OrchestrationOKResponse struct { OK bool `json:"ok"` }
