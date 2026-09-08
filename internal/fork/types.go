package fork

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/scheduler"
	"github.com/Methamorphe/go-agent/internal/world"
)

var (
	ErrNotFound          = errors.New("cognitive fork object not found")
	ErrConflict          = errors.New("cognitive fork conflict")
	ErrInvalidState      = errors.New("invalid cognitive fork state")
	ErrBudgetExhausted   = errors.New("cognitive fork budget exhausted")
	ErrNoEligibleWinner  = errors.New("no eligible cognitive fork winner")
	ErrPromotionRequired = errors.New("cognitive fork promotion requires evaluated winner")
	ErrUnsafeCheckpoint  = errors.New("checkpoint is not safe for mutation-capable fork")
)

type CheckpointClass string

const (
	CheckpointForkable   CheckpointClass = "ForkableCheckpoint"
	CheckpointCommittable CheckpointClass = "CommittableCheckpoint"
)

type ResultDependency struct {
	Ref        string `json:"ref"`
	RequiredBy string `json:"required_by,omitempty"`
}

type ChildDependency struct {
	AgentID   id.AgentID `json:"agent_id"`
	ResultRef string     `json:"result_ref,omitempty"`
}

type ExecutionFrontier struct {
	AgentID                id.AgentID         `json:"agent_id"`
	ProcessVersion          uint64             `json:"process_version"`
	LedgerSequence          uint64             `json:"ledger_sequence"`
	InvocationBoundary      id.InvocationID    `json:"invocation_boundary,omitempty"`
	CompletedResultRefs     []string           `json:"completed_result_refs,omitempty"`
	InFlightInvocations     []id.InvocationID  `json:"in_flight_invocations,omitempty"`
	InFlightActions         []id.ActionID      `json:"in_flight_actions,omitempty"`
	UnknownOutcomeActions   []id.ActionID      `json:"unknown_outcome_actions,omitempty"`
	TransactionRefs         []id.TransactionID `json:"transaction_refs,omitempty"`
	RequiredResults         []ResultDependency `json:"required_results,omitempty"`
	ChildDependencies       []ChildDependency  `json:"child_dependencies,omitempty"`
	ContextRefs             []id.ContextPageID `json:"context_refs,omitempty"`
	WorldSnapshotRef        world.BranchRef    `json:"world_snapshot_ref"`
}

type AuthoritySnapshot struct {
	Intent       world.Intent       `json:"intent"`
	Capabilities []world.Capability `json:"capabilities,omitempty"`
	CapturedAt   time.Time          `json:"captured_at"`
}

type Checkpoint struct {
	ID               id.CheckpointID  `json:"id"`
	Class             CheckpointClass  `json:"class"`
	SourceAgentID    id.AgentID       `json:"source_agent_id"`
	RootAgentID      id.AgentID       `json:"root_agent_id"`
	Frontier         ExecutionFrontier `json:"frontier"`
	Authority        AuthoritySnapshot `json:"authority"`
	LastEventID      id.EventID       `json:"last_event_id"`
	ProcessStateJSON json.RawMessage  `json:"process_state_json"`
	StateHash        string           `json:"state_hash"`
	CreatedAt        time.Time        `json:"created_at"`
	RetainUntil      *time.Time       `json:"retain_until,omitempty"`
}

type GroupState string

const (
	GroupOpen      GroupState = "OPEN"
	GroupEvaluated GroupState = "EVALUATED"
	GroupPromoting GroupState = "PROMOTING"
	GroupPromoted  GroupState = "PROMOTED"
	GroupDiscarded GroupState = "DISCARDED"
)

type Group struct {
	ID              id.ForkGroupID  `json:"id"`
	CheckpointID    id.CheckpointID `json:"checkpoint_id"`
	SourceAgentID   id.AgentID      `json:"source_agent_id"`
	State           GroupState      `json:"state"`
	WinnerForkID    id.ForkID       `json:"winner_fork_id,omitempty"`
	SelectionReason string          `json:"selection_reason,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

type BranchState string

const (
	BranchOpen      BranchState = "OPEN"
	BranchEvaluated BranchState = "EVALUATED"
	BranchSelected  BranchState = "SELECTED"
	BranchPromoted  BranchState = "PROMOTED"
	BranchDiscarded BranchState = "DISCARDED"
)

type Branch struct {
	ID                 id.ForkID             `json:"id"`
	GroupID            id.ForkGroupID        `json:"group_id"`
	AgentID            id.AgentID            `json:"agent_id"`
	WorldID            id.WorldID            `json:"world_id"`
	WorldRef           world.BranchRef        `json:"world_ref"`
	Authority          AuthoritySnapshot      `json:"authority"`
	BudgetLimit        scheduler.Resources    `json:"budget_limit"`
	BudgetSpent        scheduler.Resources    `json:"budget_spent"`
	BudgetReservation  scheduler.ReservationID `json:"budget_reservation"`
	BudgetSettled      bool                  `json:"budget_settled"`
	State              BranchState           `json:"state"`
	Evaluation         *Evaluation           `json:"evaluation,omitempty"`
	CognitiveOverlay   []OverlayEntry        `json:"cognitive_overlay,omitempty"`
	CreatedAt          time.Time             `json:"created_at"`
	UpdatedAt          time.Time             `json:"updated_at"`
}

type OverlayKind string

const (
	OverlayContext  OverlayKind = "context"
	OverlayMemory   OverlayKind = "memory"
	OverlayDecision OverlayKind = "decision"
)

type OverlayEntry struct {
	Kind          OverlayKind     `json:"kind"`
	Key           string          `json:"key"`
	Value         json.RawMessage `json:"value"`
	SourceRef     string          `json:"source_ref,omitempty"`
	ProvenanceRef string          `json:"provenance_ref,omitempty"`
	Promotable    bool            `json:"promotable"`
	Promoted      bool            `json:"promoted,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
}

type Metrics struct {
	Correctness float64 `json:"correctness"`
	Quality     float64 `json:"quality"`
	LatencyMS   int64   `json:"latency_ms"`
	MoneyMicros int64   `json:"money_micros"`
	Tokens      int64   `json:"tokens"`
}

type Objective struct {
	CorrectnessWeight float64 `json:"correctness_weight"`
	QualityWeight     float64 `json:"quality_weight"`
	LatencyWeight     float64 `json:"latency_weight"`
	MoneyWeight       float64 `json:"money_weight"`
	TokenWeight       float64 `json:"token_weight"`
	MinCorrectness    float64 `json:"min_correctness"`
}

func (o Objective) normalized() Objective {
	if o == (Objective{}) { return Objective{CorrectnessWeight: 0.65, QualityWeight: 0.35, MinCorrectness: 1} }
	return o
}

func (o Objective) Validate() error {
	o = o.normalized()
	if o.MinCorrectness < 0 || o.MinCorrectness > 1 { return errors.New("min correctness must be within [0,1]") }
	for _, w := range []float64{o.CorrectnessWeight, o.QualityWeight, o.LatencyWeight, o.MoneyWeight, o.TokenWeight} {
		if w < 0 { return errors.New("objective weights must be non-negative") }
	}
	return nil
}

type Evaluation struct {
	Metrics     Metrics   `json:"metrics"`
	Score       float64   `json:"score"`
	Eligible    bool      `json:"eligible"`
	Reason      string    `json:"reason"`
	EvidenceRef string    `json:"evidence_ref,omitempty"`
	EvaluatedAt time.Time `json:"evaluated_at"`
}

func Evaluate(metrics Metrics, objective Objective, at time.Time) Evaluation {
	o := objective.normalized()
	eligible := metrics.Correctness >= o.MinCorrectness
	score := metrics.Correctness*o.CorrectnessWeight + metrics.Quality*o.QualityWeight -
		(float64(metrics.LatencyMS)/1000.0)*o.LatencyWeight -
		(float64(metrics.MoneyMicros)/1_000_000.0)*o.MoneyWeight -
		(float64(metrics.Tokens)/1000.0)*o.TokenWeight
	reason := fmt.Sprintf("score=%.6f correctness=%.4f quality=%.4f latency_ms=%d money_micros=%d tokens=%d", score, metrics.Correctness, metrics.Quality, metrics.LatencyMS, metrics.MoneyMicros, metrics.Tokens)
	if !eligible { reason += fmt.Sprintf("; rejected: correctness below floor %.4f", o.MinCorrectness) }
	return Evaluation{Metrics: metrics, Score: score, Eligible: eligible, Reason: reason, EvaluatedAt: at.UTC()}
}

func SelectWinner(branches []Branch) (id.ForkID, error) {
	eligible := make([]Branch, 0, len(branches))
	for _, b := range branches { if b.Evaluation != nil && b.Evaluation.Eligible { eligible = append(eligible, b) } }
	if len(eligible) == 0 { return "", ErrNoEligibleWinner }
	sort.Slice(eligible, func(i, j int) bool {
		if eligible[i].Evaluation.Score == eligible[j].Evaluation.Score { return strings.Compare(eligible[i].ID.String(), eligible[j].ID.String()) < 0 }
		return eligible[i].Evaluation.Score > eligible[j].Evaluation.Score
	})
	return eligible[0].ID, nil
}

func NamespaceAction(forkID id.ForkID, actionID id.ActionID) id.ActionID {
	if forkID == "" || actionID == "" { return actionID }
	return id.ActionID("fork:" + forkID.String() + ":" + actionID.String())
}
