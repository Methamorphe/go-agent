package mmu

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/objectstore"
)

const (
	DefaultModelMaxContext      = 32_768
	DefaultReservedOutputTokens = 4_096
	DefaultProviderOverhead     = 1_024
	DefaultSafetyMargin         = 512
)

var (
	ErrContextBudgetImpossible = errors.New("context budget impossible")
	ErrRecallLoopDetected      = errors.New("recall loop detected")
)

type PageType string

const (
	PageUserMessage       PageType = "user_message"
	PageAssistantSummary  PageType = "assistant_summary"
	PageSourceExcerpt     PageType = "source_excerpt"
	PageFileSummary       PageType = "file_summary"
	PageToolResultSummary PageType = "tool_result_summary"
	PageDecision          PageType = "decision"
	PagePlan              PageType = "plan"
	PageChildReport       PageType = "child_report"
	PageErrorTrace        PageType = "error_trace"
	PageDocumentation     PageType = "documentation"
	PageRecallResult      PageType = "recall_result"
)

type Scope string

const (
	ScopeProcess   Scope = "process"
	ScopeRootTask  Scope = "root_task"
	ScopeProject   Scope = "project"
	ScopeWorkspace Scope = "workspace"
	ScopeWorld     Scope = "world"
)

type PageMeta struct {
	ID             id.ContextPageID   `json:"id"`
	AgentID        id.AgentID         `json:"agent_id"`
	Type           PageType           `json:"type"`
	Scope          Scope              `json:"scope"`
	SourceRef      string             `json:"source_ref,omitempty"`
	ObjectRef      objectstore.Ref    `json:"object_ref"`
	TokenEstimate  int                `json:"token_estimate"`
	Importance     float64            `json:"importance"`
	Confidence     float64            `json:"confidence"`
	CreatedAt      time.Time          `json:"created_at"`
	LastAccessedAt time.Time          `json:"last_accessed_at"`
	PinnedUntil    *time.Time         `json:"pinned_until,omitempty"`
	SupersededBy   *id.ContextPageID  `json:"superseded_by,omitempty"`
	CompactedBy    *id.ContextPageID  `json:"compacted_by,omitempty"`
	SummaryOf      []id.ContextPageID `json:"summary_of,omitempty"`
}

type PageInput struct {
	AgentID     id.AgentID
	Type        PageType
	Scope       Scope
	SourceRef   string
	Content     string
	Importance  float64
	Confidence  float64
	PinnedUntil *time.Time
	SummaryOf   []id.ContextPageID
}

type CandidateQuery struct {
	AgentID id.AgentID
	Text    string
	Scopes  []Scope
	Types   []PageType
	Limit   int
}

type Candidate struct {
	Meta         PageMeta
	LexicalScore float64
}

type ContextLease struct {
	ID              id.LeaseID       `json:"id"`
	PageID          id.ContextPageID `json:"page_id"`
	AgentID         id.AgentID       `json:"agent_id"`
	Reason          string           `json:"reason"`
	RemainingBuilds int              `json:"remaining_builds"`
	ExpiresAt       *time.Time       `json:"expires_at,omitempty"`
	HardPin         bool             `json:"hard_pin"`
	CreatedAt       time.Time        `json:"created_at"`
}

type RankingComponents struct {
	Explicit   float64 `json:"explicit"`
	Task       float64 `json:"task"`
	Recency    float64 `json:"recency"`
	Importance float64 `json:"importance"`
	Confidence float64 `json:"confidence"`
	Type       float64 `json:"type"`
}

type RecallQuery struct {
	Text        string             `json:"text,omitempty"`
	Scopes      []Scope            `json:"scopes,omitempty"`
	Types       []PageType         `json:"types,omitempty"`
	Limit       int                `json:"limit,omitempty"`
	MaxTokens   int                `json:"max_tokens,omitempty"`
	ExplicitIDs []id.ContextPageID `json:"explicit_ids,omitempty"`
}

type RecallRequest struct {
	AgentID       id.AgentID
	AllowedScopes []Scope
	Query         RecallQuery
}

type RecallSelection struct {
	PageID     id.ContextPageID  `json:"page_id"`
	Score      float64           `json:"score"`
	Components RankingComponents `json:"components"`
	Reason     string            `json:"reason"`
	Tokens     int               `json:"tokens"`
}

type RecallResult struct {
	Selected   []RecallSelection  `json:"selected"`
	Unresolved []id.ContextPageID `json:"unresolved,omitempty"`
	BudgetUsed int                `json:"budget_used"`
}

type Budget struct {
	ModelMaxContext      int `json:"model_max_context"`
	ReservedOutputTokens int `json:"reserved_output_tokens"`
	ProviderOverhead     int `json:"provider_overhead"`
	SafetyMargin         int `json:"safety_margin"`
	SchemaTokens         int `json:"schema_tokens"`
	ActiveTokens         int `json:"active_tokens"`
}

func (b Budget) InputBudget() int {
	return b.ModelMaxContext - b.ReservedOutputTokens - b.ProviderOverhead - b.SafetyMargin
}

type BuildRequest struct {
	AgentID       id.AgentID
	Model         string
	Budget        Budget
	AllowedScopes []Scope
	MandatoryIDs  []id.ContextPageID
	ActiveIDs     []id.ContextPageID
	Query         string
}

type MaterializedPage struct {
	Meta    PageMeta `json:"meta"`
	Tier    int      `json:"tier"`
	Score   float64  `json:"score"`
	Reason  string   `json:"reason"`
	Content string   `json:"content"`
	Tokens  int      `json:"tokens"`
}

type ManifestPage struct {
	ID         id.ContextPageID  `json:"id"`
	Tier       int               `json:"tier"`
	Reason     string            `json:"reason"`
	Tokens     int               `json:"tokens"`
	Score      float64           `json:"score,omitempty"`
	Components RankingComponents `json:"components,omitempty"`
}

type ManifestExclusion struct {
	ID     id.ContextPageID `json:"id"`
	Reason string           `json:"reason"`
}

type ContextManifest struct {
	InvocationID   id.InvocationID     `json:"invocation_id,omitempty"`
	AgentID        id.AgentID          `json:"agent_id"`
	Model          string              `json:"model"`
	InputBudget    int                 `json:"input_budget"`
	EstimatedUsed  int                 `json:"estimated_used"`
	ReservedOutput int                 `json:"reserved_output"`
	SchemaTokens   int                 `json:"schema_tokens"`
	ActiveTokens   int                 `json:"active_tokens"`
	Pages          []ManifestPage      `json:"pages"`
	Excluded       []ManifestExclusion `json:"excluded,omitempty"`
}

type WorkingSet struct {
	Pages    []MaterializedPage
	Manifest ContextManifest
}

type CompactRequest struct {
	AgentID   id.AgentID
	SourceIDs []id.ContextPageID
	Summary   string
	Type      PageType
	Scope     Scope
	SourceRef string
}

type RankingWeights struct {
	Explicit   float64
	Task       float64
	Recency    float64
	Importance float64
	Confidence float64
	Type       float64
}

type Config struct {
	CandidateLimit         int
	DefaultRecallLimit     int
	DefaultRecallMaxTokens int
	MaxPageTokens          int
	MaxMaterializeBytes    int64
	MaxIndexTextBytes      int
	MaxTokenCacheEntries   int
	RecallLoopThreshold    int
	MaxRecallTrackers      int
	Weights                RankingWeights
}

func DefaultConfig() Config {
	return Config{
		CandidateLimit:         256,
		DefaultRecallLimit:     8,
		DefaultRecallMaxTokens: 8_000,
		MaxPageTokens:          4_000,
		MaxMaterializeBytes:    256 << 10,
		MaxIndexTextBytes:      16 << 10,
		MaxTokenCacheEntries:   4_096,
		RecallLoopThreshold:    3,
		MaxRecallTrackers:      1_024,
		Weights: RankingWeights{
			Explicit:   10,
			Task:       4,
			Recency:    1,
			Importance: 1.5,
			Confidence: 0.5,
			Type:       0.5,
		},
	}
}

type Repository interface {
	PutContextPage(context.Context, PageMeta, string) error
	ContextPage(context.Context, id.AgentID, id.ContextPageID) (PageMeta, error)
	SearchContextPages(context.Context, CandidateQuery) ([]Candidate, error)
	TouchContextPages(context.Context, id.AgentID, []id.ContextPageID, time.Time) error
	MarkContextPagesCompacted(context.Context, id.AgentID, []id.ContextPageID, id.ContextPageID) error
	PutContextLease(context.Context, ContextLease) error
	ActiveContextLeases(context.Context, id.AgentID, time.Time) ([]ContextLease, error)
	ConsumeContextLeases(context.Context, id.AgentID, []id.ContextPageID) error
	PutContextManifest(context.Context, id.InvocationID, id.AgentID, objectstore.Ref, time.Time) error
}

type ObjectStore interface {
	Put(context.Context, io.Reader) (objectstore.Meta, error)
	Open(objectstore.Ref) (io.ReadCloser, error)
}

type IDGenerator interface {
	ContextPage() (id.ContextPageID, error)
	Lease() (id.LeaseID, error)
}
