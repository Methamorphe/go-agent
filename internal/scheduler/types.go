package scheduler

import (
	"errors"
	"fmt"
	"github.com/Methamorphe/go-agent/internal/id"
	"strings"
	"time"
)

const (
	DefaultMaxProfiles          = 256
	DefaultPolicyVersion uint32 = 1
)

var (
	ErrNoEligibleModel    = errors.New("no eligible model")
	ErrBudgetExhausted    = errors.New("scheduler budget exhausted")
	ErrReservationSettled = errors.New("budget reservation already settled")
	ErrReservationUnknown = errors.New("budget reservation not found")
	ErrSlotsExhausted     = errors.New("scheduler concurrency slots exhausted")
)

type Clock interface{ Now() time.Time }
type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

type ModelID string
type ProviderID string
type TaskID string
type DecisionID string
type ReservationID string

type Capability string

const (
	CapabilityToolCalling      Capability = "tool-calling"
	CapabilityStructuredOutput Capability = "structured-output"
	CapabilityVision           Capability = "vision"
	CapabilityReasoning        Capability = "reasoning-controls"
	CapabilityStreaming        Capability = "streaming"
	CapabilityLargeContext     Capability = "large-context"
)

type TaskKind string

const (
	TaskClassification        TaskKind = "classification"
	TaskExtraction            TaskKind = "extraction"
	TaskSummarization         TaskKind = "summarization"
	TaskCodeGeneration        TaskKind = "code-generation"
	TaskCodeReview            TaskKind = "code-review"
	TaskArchitectureReasoning TaskKind = "architecture-reasoning"
	TaskPlanning              TaskKind = "planning"
	TaskRetrievalQuery        TaskKind = "retrieval-query-generation"
	TaskMemoryConsolidation   TaskKind = "memory-consolidation"
	TaskConflictResolution    TaskKind = "conflict-resolution"
	TaskEvaluation            TaskKind = "evaluation"
	TaskConversation          TaskKind = "conversation-response"
	TaskVision                TaskKind = "vision"
)

type Objective string

const (
	ObjectiveQualityFirst Objective = "quality-first"
	ObjectiveBalanced     Objective = "balanced"
	ObjectiveCostFirst    Objective = "cost-first"
	ObjectiveLatencyFirst Objective = "latency-first"
	ObjectiveLocalOnly    Objective = "local-only"
)

type Locality string

const (
	LocalityCloud Locality = "cloud"
	LocalityLocal Locality = "local"
)

type PrivacyPolicy string

const (
	PrivacyStandard  PrivacyPolicy = "standard"
	PrivacySensitive PrivacyPolicy = "sensitive"
	PrivacyLocalOnly PrivacyPolicy = "local-only"
)

type RiskClass string

const (
	RiskLow    RiskClass = "low"
	RiskNormal RiskClass = "normal"
	RiskHigh   RiskClass = "high"
)

type HealthState string

const (
	HealthHealthy     HealthState = "healthy"
	HealthDegraded    HealthState = "degraded"
	HealthRateLimited HealthState = "rate_limited"
	HealthUnavailable HealthState = "unavailable"
	HealthRecovering  HealthState = "recovering"
)

type DataPolicy struct {
	AllowsSensitive bool `json:"allows_sensitive"`
	TrainingOptOut  bool `json:"training_opt_out"`
}

type ModelProfile struct {
	ModelID                    ModelID              `json:"model_id"`
	ProviderID                 ProviderID           `json:"provider_id"`
	ContextWindow              int                  `json:"context_window"`
	MaxOutputTokens            int                  `json:"max_output_tokens"`
	Capabilities               []Capability         `json:"capabilities,omitempty"`
	Locality                   Locality             `json:"locality"`
	DataPolicy                 DataPolicy           `json:"data_policy"`
	InputCostMicrosPerMillion  int64                `json:"input_cost_micros_per_million"`
	OutputCostMicrosPerMillion int64                `json:"output_cost_micros_per_million"`
	LatencyP95                 time.Duration        `json:"latency_p95"`
	Reliability                float64              `json:"reliability"`
	Quality                    map[TaskKind]float64 `json:"quality,omitempty"`
	MaxConcurrency             int                  `json:"max_concurrency"`
	Experimental               bool                 `json:"experimental,omitempty"`
	ProfileVersion             uint32               `json:"profile_version"`
}

func (p ModelProfile) Ref() ModelRef {
	return ModelRef{ProviderID: p.ProviderID, ModelID: p.ModelID, Version: p.ProfileVersion}
}
func (p ModelProfile) Key() string { return string(p.ProviderID) + "/" + string(p.ModelID) }
func (p ModelProfile) Has(cap Capability) bool {
	for _, item := range p.Capabilities {
		if item == cap {
			return true
		}
	}
	return false
}
func (p ModelProfile) QualityFor(kind TaskKind) float64 {
	if p.Quality == nil {
		return 0
	}
	return p.Quality[kind]
}
func (p ModelProfile) Validate() error {
	if strings.TrimSpace(string(p.ProviderID)) == "" || strings.TrimSpace(string(p.ModelID)) == "" {
		return errors.New("provider_id and model_id are required")
	}
	if p.ContextWindow <= 0 || p.MaxOutputTokens <= 0 {
		return errors.New("context_window and max_output_tokens must be positive")
	}
	if p.InputCostMicrosPerMillion < -1 || p.OutputCostMicrosPerMillion < -1 {
		return errors.New("cost must be >= 0 or -1 for unknown")
	}
	if p.Reliability < 0 || p.Reliability > 1 {
		return errors.New("reliability must be within [0,1]")
	}
	for _, q := range p.Quality {
		if q < 0 || q > 1 {
			return errors.New("quality must be within [0,1]")
		}
	}
	if p.MaxConcurrency <= 0 {
		return errors.New("max_concurrency must be positive")
	}
	if p.ProfileVersion == 0 {
		return errors.New("profile_version must be positive")
	}
	if p.Locality != LocalityCloud && p.Locality != LocalityLocal {
		return errors.New("invalid locality")
	}
	return nil
}

type ModelRef struct {
	ProviderID ProviderID `json:"provider_id"`
	ModelID    ModelID    `json:"model_id"`
	Version    uint32     `json:"version"`
}

func (r ModelRef) Key() string { return string(r.ProviderID) + "/" + string(r.ModelID) }

type Requirements struct {
	MinContext           int          `json:"min_context,omitempty"`
	RequiredCapabilities []Capability `json:"required_capabilities,omitempty"`
	AllowedProviders     []ProviderID `json:"allowed_providers,omitempty"`
	DeniedProviders      []ProviderID `json:"denied_providers,omitempty"`
	AllowedModels        []ModelID    `json:"allowed_models,omitempty"`
	DeniedModels         []ModelID    `json:"denied_models,omitempty"`
	MinQuality           float64      `json:"min_quality,omitempty"`
	MinReliability       float64      `json:"min_reliability,omitempty"`
}

type Resources struct {
	MoneyMicros int64 `json:"money_micros"`
	Tokens      int64 `json:"tokens"`
}

func (r Resources) Valid() bool { return r.MoneyMicros >= 0 && r.Tokens >= 0 }
func (r Resources) Fits(available Resources) bool {
	return r.MoneyMicros <= available.MoneyMicros && r.Tokens <= available.Tokens
}
func (r Resources) Add(o Resources) Resources {
	return Resources{MoneyMicros: r.MoneyMicros + o.MoneyMicros, Tokens: r.Tokens + o.Tokens}
}
func (r Resources) Sub(o Resources) Resources {
	return Resources{MoneyMicros: r.MoneyMicros - o.MoneyMicros, Tokens: r.Tokens - o.Tokens}
}

type CognitiveTask struct {
	ID                   TaskID        `json:"id"`
	AgentID              id.AgentID    `json:"agent_id"`
	RootAgentID          id.AgentID    `json:"root_agent_id"`
	Kind                 TaskKind      `json:"kind"`
	EstimatedInputTokens int           `json:"estimated_input_tokens"`
	ReservedOutputTokens int           `json:"reserved_output_tokens"`
	Requirements         Requirements  `json:"requirements"`
	Objective            Objective     `json:"objective"`
	Budget               Resources     `json:"budget"`
	Deadline             *time.Time    `json:"deadline,omitempty"`
	Privacy              PrivacyPolicy `json:"privacy"`
	Risk                 RiskClass     `json:"risk"`
	AllowExperimental    bool          `json:"allow_experimental,omitempty"`
}

func (t CognitiveTask) Validate() error {
	if t.ID == "" || t.AgentID == "" || t.RootAgentID == "" {
		return errors.New("task id, agent id and root agent id are required")
	}
	if t.EstimatedInputTokens < 0 || t.ReservedOutputTokens <= 0 {
		return errors.New("invalid token estimate")
	}
	if !t.Budget.Valid() {
		return errors.New("invalid task budget")
	}
	if t.Requirements.MinQuality < 0 || t.Requirements.MinQuality > 1 || t.Requirements.MinReliability < 0 || t.Requirements.MinReliability > 1 {
		return errors.New("invalid requirement floor")
	}
	return nil
}

type RejectionCode string

const (
	RejectContext      RejectionCode = "context"
	RejectCapability   RejectionCode = "capability"
	RejectPrivacy      RejectionCode = "privacy"
	RejectPolicy       RejectionCode = "policy"
	RejectHealth       RejectionCode = "health"
	RejectBudget       RejectionCode = "budget"
	RejectQuality      RejectionCode = "quality"
	RejectReliability  RejectionCode = "reliability"
	RejectDeadline     RejectionCode = "deadline"
	RejectExperimental RejectionCode = "experimental"
	RejectExcluded     RejectionCode = "excluded"
)

type CandidateRejection struct {
	Model  ModelRef      `json:"model"`
	Code   RejectionCode `json:"code"`
	Detail string        `json:"detail"`
}
type ScoreBreakdown struct {
	Quality     float64 `json:"quality"`
	Cost        float64 `json:"cost"`
	Latency     float64 `json:"latency"`
	FailureRisk float64 `json:"failure_risk"`
	Load        float64 `json:"load"`
	Total       float64 `json:"total"`
}
type Candidate struct {
	Model            ModelRef       `json:"model"`
	EstimatedCost    Resources      `json:"estimated_cost"`
	EstimatedLatency time.Duration  `json:"estimated_latency"`
	Score            ScoreBreakdown `json:"score"`
}
type RoutingDecision struct {
	ID               DecisionID           `json:"id"`
	TaskID           TaskID               `json:"task_id"`
	Candidates       []Candidate          `json:"candidates"`
	Rejected         []CandidateRejection `json:"rejected,omitempty"`
	Selected         ModelRef             `json:"selected"`
	EstimatedCost    Resources            `json:"estimated_cost"`
	EstimatedLatency time.Duration        `json:"estimated_latency"`
	Score            ScoreBreakdown       `json:"score"`
	PolicyVersion    uint32               `json:"policy_version"`
	CreatedAt        time.Time            `json:"created_at"`
}

type NoEligibleError struct{ Rejected []CandidateRejection }

func (e *NoEligibleError) Error() string {
	return fmt.Sprintf("%v: %d candidates rejected", ErrNoEligibleModel, len(e.Rejected))
}
func (e *NoEligibleError) Unwrap() error { return ErrNoEligibleModel }
