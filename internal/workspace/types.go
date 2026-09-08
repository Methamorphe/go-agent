package workspace

import (
	"encoding/json"
	"time"

	"github.com/Methamorphe/go-agent/internal/id"
	agentprocess "github.com/Methamorphe/go-agent/internal/process"
)

const (
	ProtocolVersion uint32 = 1
	DefaultPageSize        = 100
	MaxPageSize            = 500
	DefaultTreeLimit       = 256
	MaxTreeLimit           = 4096
)

type Mode string

const (
	ModeAsk     Mode = "ASK"
	ModePlan    Mode = "PLAN"
	ModeAct     Mode = "ACT"
	ModeReview  Mode = "REVIEW"
	ModeObserve Mode = "OBSERVE"
)

func (m Mode) Valid() bool {
	switch m {
	case ModeAsk, ModePlan, ModeAct, ModeReview, ModeObserve:
		return true
	default:
		return false
	}
}

type BlockKind string

const (
	BlockUserMessage      BlockKind = "UserMessage"
	BlockAssistantMessage BlockKind = "AssistantMessage"
	BlockProgress         BlockKind = "ProgressUpdate"
	BlockPlan             BlockKind = "Plan"
	BlockToolCall         BlockKind = "ToolCall"
	BlockToolResult       BlockKind = "ToolResult"
	BlockDiff             BlockKind = "Diff"
	BlockTestResult       BlockKind = "TestResult"
	BlockArtifact         BlockKind = "Artifact"
	BlockApproval         BlockKind = "Approval"
	BlockChildAgent       BlockKind = "ChildAgent"
	BlockTransaction      BlockKind = "Transaction"
	BlockForkComparison   BlockKind = "ForkComparison"
	BlockContextFault     BlockKind = "ContextFault"
	BlockSecurityDecision BlockKind = "SecurityDecision"
	BlockCheckpoint       BlockKind = "Checkpoint"
	BlockError            BlockKind = "Error"
	BlockSystem           BlockKind = "SystemNotice"
)

type Block struct {
	ID             string          `json:"id"`
	AgentID        id.AgentID      `json:"agent_id"`
	Sequence       uint64          `json:"sequence"`
	ProcessVersion uint64          `json:"process_version"`
	Kind           BlockKind       `json:"kind"`
	Title          string          `json:"title,omitempty"`
	Preview        string          `json:"preview,omitempty"`
	ObjectRef      string          `json:"object_ref,omitempty"`
	EventType      string          `json:"event_type,omitempty"`
	Payload        json.RawMessage `json:"payload,omitempty"`
	OccurredAt     time.Time       `json:"occurred_at"`
	Critical       bool            `json:"critical,omitempty"`
}

type ConversationViewport struct {
	AgentID     id.AgentID `json:"agent_id"`
	Blocks      []Block    `json:"blocks"`
	Before      uint64     `json:"before,omitempty"`
	Oldest      uint64     `json:"oldest,omitempty"`
	Newest      uint64     `json:"newest,omitempty"`
	HasPrevious bool       `json:"has_previous"`
}

type ProcessSummary struct {
	AgentID       id.AgentID          `json:"agent_id"`
	RootAgentID   id.AgentID          `json:"root_agent_id"`
	ParentAgentID *id.AgentID         `json:"parent_agent_id,omitempty"`
	Depth         uint32              `json:"depth"`
	Status        agentprocess.Status `json:"status"`
	Version       uint64              `json:"version"`
	Goal          string              `json:"goal,omitempty"`
	WaitingReason string              `json:"waiting_reason,omitempty"`
	WaitingRef    string              `json:"waiting_ref,omitempty"`
	Failure       string              `json:"failure,omitempty"`
	UpdatedAt     time.Time           `json:"updated_at"`
	CreatedAt     time.Time           `json:"created_at"`
	Children      int                 `json:"children"`
}

type Snapshot struct {
	ProtocolVersion uint32               `json:"protocol_version"`
	RootAgentID     id.AgentID           `json:"root_agent_id"`
	FocusedAgentID  id.AgentID           `json:"focused_agent_id"`
	Mode            Mode                 `json:"mode"`
	Cursor          uint64               `json:"cursor"`
	Tree            []ProcessSummary     `json:"tree"`
	Viewport        ConversationViewport `json:"viewport"`
	Inspector       Inspector            `json:"inspector"`
	GeneratedAt     time.Time            `json:"generated_at"`
}

type AttachRequest struct {
	AgentID      id.AgentID `json:"agent_id,omitempty"`
	Mode         Mode       `json:"mode,omitempty"`
	HistoryLimit int        `json:"history_limit,omitempty"`
	TreeLimit    int        `json:"tree_limit,omitempty"`
}

type RefreshRequest struct {
	RootAgentID    id.AgentID `json:"root_agent_id"`
	FocusedAgentID id.AgentID `json:"focused_agent_id"`
	AfterSequence  uint64     `json:"after_sequence"`
	Limit          int        `json:"limit,omitempty"`
	Inspector      bool       `json:"inspector,omitempty"`
}

type Refresh struct {
	Cursor     uint64           `json:"cursor"`
	TreePatch  []ProcessSummary `json:"tree_patch"`
	Blocks     []Block          `json:"blocks"`
	Inspector  *Inspector       `json:"inspector,omitempty"`
	Behind     bool             `json:"behind"`
	GeneratedAt time.Time       `json:"generated_at"`
}

type HistoryRequest struct {
	AgentID id.AgentID `json:"agent_id"`
	Before  uint64     `json:"before,omitempty"`
	Limit   int        `json:"limit,omitempty"`
}

type SearchRequest struct {
	RootAgentID id.AgentID `json:"root_agent_id"`
	Query       string     `json:"query"`
	Limit       int        `json:"limit,omitempty"`
}

type SearchResult struct {
	Blocks []Block `json:"blocks"`
}
