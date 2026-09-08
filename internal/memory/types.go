package memory

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"time"

	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/objectstore"
)

type ScopeKind string

const (
	ScopeProcess   ScopeKind = "process"
	ScopeRootTask  ScopeKind = "root_task"
	ScopeProject   ScopeKind = "project"
	ScopeWorkspace ScopeKind = "workspace"
	ScopeWorld     ScopeKind = "world"
	ScopeUser      ScopeKind = "user"
)

type Scope struct {
	Kind ScopeKind `json:"kind"`
	Ref  string    `json:"ref,omitempty"`
}

func (s Scope) Valid() bool {
	switch s.Kind {
	case ScopeProcess, ScopeRootTask, ScopeProject, ScopeWorkspace, ScopeWorld, ScopeUser:
		return true
	default:
		return false
	}
}

func (s Scope) Matches(other Scope) bool {
	if s.Kind != other.Kind {
		return false
	}
	return s.Ref == "" || other.Ref == "" || s.Ref == other.Ref
}

type EvidenceKind string

const (
	EvidenceUserStatement  EvidenceKind = "user_statement"
	EvidenceRepository     EvidenceKind = "repository_source"
	EvidenceCommandOutput  EvidenceKind = "command_output"
	EvidenceTestResult     EvidenceKind = "test_result"
	EvidenceAPIResponse    EvidenceKind = "api_response"
	EvidenceWebResult      EvidenceKind = "web_result"
	EvidenceAgentReport    EvidenceKind = "agent_report"
	EvidenceWorldObserve   EvidenceKind = "world_observation"
	EvidenceEpisode        EvidenceKind = "episode"
	EvidenceOther          EvidenceKind = "other"
)

type ProvenanceClass string

const (
	ProvenanceUserExplicit     ProvenanceClass = "UserExplicit"
	ProvenanceRuntimeVerified  ProvenanceClass = "RuntimeVerified"
	ProvenanceTrustedConnector ProvenanceClass = "TrustedConnector"
	ProvenanceRepositorySource ProvenanceClass = "RepositorySource"
	ProvenanceExternalSource   ProvenanceClass = "ExternalSource"
	ProvenanceAgentReport      ProvenanceClass = "AgentReport"
	ProvenanceModelInference   ProvenanceClass = "ModelInference"
	ProvenanceConsolidated     ProvenanceClass = "Consolidated"
)

func validEvidenceProvenance(value ProvenanceClass) bool {
	switch value {
	case ProvenanceUserExplicit, ProvenanceRuntimeVerified, ProvenanceTrustedConnector,
		ProvenanceRepositorySource, ProvenanceExternalSource, ProvenanceAgentReport:
		return true
	default:
		return false
	}
}

type TrustClass string

const (
	TrustUntrusted     TrustClass = "untrusted"
	TrustAuthenticated TrustClass = "authenticated"
	TrustDeterministic TrustClass = "deterministic"
	TrustUserAsserted  TrustClass = "user_asserted"
	TrustUnknown       TrustClass = "unknown"
)

type SensitivityClass string

const (
	SensitivityPublic     SensitivityClass = "public"
	SensitivityInternal   SensitivityClass = "internal"
	SensitivitySensitive  SensitivityClass = "sensitive"
	SensitivityRestricted SensitivityClass = "restricted"
)

type Evidence struct {
	ID            id.EvidenceID    `json:"id"`
	Kind          EvidenceKind     `json:"kind"`
	Scope         Scope            `json:"scope"`
	SourceRef     string           `json:"source_ref"`
	ObjectRef     objectstore.Ref  `json:"object_ref,omitempty"`
	ContentHash   string           `json:"content_hash"`
	ObservedAt    time.Time        `json:"observed_at"`
	SourceVersion string           `json:"source_version,omitempty"`
	Provenance    ProvenanceClass  `json:"provenance"`
	TrustClass    TrustClass       `json:"trust_class"`
	Sensitivity   SensitivityClass `json:"sensitivity"`
}

type EvidenceInvalidation struct {
	EvidenceID            id.EvidenceID `json:"evidence_id"`
	Reason                string        `json:"reason"`
	ReplacementEvidenceID id.EvidenceID `json:"replacement_evidence_id,omitempty"`
	InvalidatedAt         time.Time     `json:"invalidated_at"`
}

type SourceVersion struct {
	SourceRef   string    `json:"source_ref"`
	Version     string    `json:"version"`
	ContentHash string    `json:"content_hash"`
	ObservedAt  time.Time `json:"observed_at"`
}

type BeliefStatus string

const (
	BeliefCandidate   BeliefStatus = "Candidate"
	BeliefActive      BeliefStatus = "Active"
	BeliefContested   BeliefStatus = "Contested"
	BeliefStale       BeliefStatus = "Stale"
	BeliefNeedsReview BeliefStatus = "NeedsReview"
	BeliefInvalidated BeliefStatus = "Invalidated"
	BeliefSuperseded  BeliefStatus = "Superseded"
	BeliefRejected    BeliefStatus = "Rejected"
)

func (s BeliefStatus) Valid() bool {
	switch s {
	case BeliefCandidate, BeliefActive, BeliefContested, BeliefStale, BeliefNeedsReview,
		BeliefInvalidated, BeliefSuperseded, BeliefRejected:
		return true
	default:
		return false
	}
}

func (s BeliefStatus) HistoricalOnly() bool {
	return s == BeliefInvalidated || s == BeliefSuperseded || s == BeliefRejected
}

type EvidenceStrength string

const (
	StrengthUnknown         EvidenceStrength = "unknown"
	StrengthWeakSuggestion  EvidenceStrength = "weak_suggestion"
	StrengthExternal        EvidenceStrength = "external"
	StrengthRepository      EvidenceStrength = "repository"
	StrengthDirectAssertion EvidenceStrength = "direct_assertion"
	StrengthRuntimeVerified EvidenceStrength = "runtime_verified"
)

type FreshnessClass string

const (
	FreshnessUnknown FreshnessClass = "unknown"
	FreshnessFresh   FreshnessClass = "fresh"
	FreshnessAging   FreshnessClass = "aging"
	FreshnessStale   FreshnessClass = "stale"
)

type VerificationClass string

const (
	VerificationNone          VerificationClass = "none"
	VerificationCorroborated  VerificationClass = "corroborated"
	VerificationSourceChecked VerificationClass = "source_checked"
	VerificationTestConfirmed VerificationClass = "test_confirmed"
)

type ConflictLevel string

const (
	ConflictNone     ConflictLevel = "none"
	ConflictPossible ConflictLevel = "possible"
	ConflictVerified ConflictLevel = "verified"
)

type ConfidenceProfile struct {
	EvidenceStrength EvidenceStrength   `json:"evidence_strength"`
	Corroboration    uint16             `json:"corroboration"`
	Freshness        FreshnessClass     `json:"freshness"`
	InferenceDepth   uint16             `json:"inference_depth"`
	Verification     VerificationClass  `json:"verification"`
	ConflictLevel    ConflictLevel      `json:"conflict"`
	OptionalScore    *float32           `json:"optional_score,omitempty"`
}

type ValidityWindow struct {
	ValidFrom  *time.Time `json:"valid_from,omitempty"`
	ValidUntil *time.Time `json:"valid_until,omitempty"`
	AsOfRef    string     `json:"as_of_ref,omitempty"`
}

func (v ValidityWindow) Contains(at time.Time) bool {
	at = at.UTC()
	if v.ValidFrom != nil && at.Before(v.ValidFrom.UTC()) {
		return false
	}
	if v.ValidUntil != nil && !at.Before(v.ValidUntil.UTC()) {
		return false
	}
	return true
}

type EvidenceRelation string

const (
	EvidenceSupports         EvidenceRelation = "supports"
	EvidenceVerifies         EvidenceRelation = "verifies"
	EvidenceContradicts      EvidenceRelation = "contradicts"
	EvidenceSupersedesSource EvidenceRelation = "supersedes_source"
	EvidenceContextualizes   EvidenceRelation = "contextualizes"
	EvidenceWeaklySuggests   EvidenceRelation = "weakly_suggests"
)

func (r EvidenceRelation) Valid() bool {
	switch r {
	case EvidenceSupports, EvidenceVerifies, EvidenceContradicts, EvidenceSupersedesSource,
		EvidenceContextualizes, EvidenceWeaklySuggests:
		return true
	default:
		return false
	}
}

type EvidenceLink struct {
	BeliefID   id.BeliefID      `json:"belief_id"`
	EvidenceID id.EvidenceID    `json:"evidence_id"`
	Relation   EvidenceRelation `json:"relation"`
}

type BeliefRelation string

const (
	BeliefDerivedFrom BeliefRelation = "derived_from"
	BeliefRequires    BeliefRelation = "requires"
	BeliefSupports    BeliefRelation = "supports"
	BeliefContradicts BeliefRelation = "contradicts"
	BeliefSupersedes  BeliefRelation = "supersedes"
	BeliefRefines     BeliefRelation = "refines"
)

func (r BeliefRelation) Valid() bool {
	switch r {
	case BeliefDerivedFrom, BeliefRequires, BeliefSupports, BeliefContradicts, BeliefSupersedes, BeliefRefines:
		return true
	default:
		return false
	}
}

type BeliefEdge struct {
	From      id.BeliefID   `json:"from"`
	To        id.BeliefID   `json:"to"`
	Relation  BeliefRelation `json:"relation"`
	CreatedAt time.Time      `json:"created_at"`
}

type Belief struct {
	ID               id.BeliefID       `json:"id"`
	Proposition      string            `json:"proposition"`
	Fingerprint      string            `json:"fingerprint"`
	Scope            Scope             `json:"scope"`
	Status           BeliefStatus      `json:"status"`
	OriginProvenance ProvenanceClass   `json:"origin_provenance"`
	Confidence       ConfidenceProfile `json:"confidence"`
	Validity         ValidityWindow    `json:"validity"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
	LastReviewedAt   *time.Time        `json:"last_reviewed_at,omitempty"`
	Version          uint32            `json:"version"`
	Speculative      bool              `json:"speculative"`
	OriginForkID     id.ForkID         `json:"origin_fork_id,omitempty"`
}

type StatusTransition struct {
	BeliefID id.BeliefID     `json:"belief_id"`
	From     BeliefStatus    `json:"from"`
	To       BeliefStatus    `json:"to"`
	Reason   string          `json:"reason"`
	JobID    id.PropagationID `json:"job_id,omitempty"`
	ChangedAt time.Time      `json:"changed_at"`
}

type UsageRecord struct {
	BeliefID      id.BeliefID      `json:"belief_id"`
	AgentID       id.AgentID       `json:"agent_id"`
	InvocationID  id.InvocationID  `json:"invocation_id,omitempty"`
	ContextPageID id.ContextPageID `json:"context_page_id,omitempty"`
	Purpose       string           `json:"purpose,omitempty"`
	UsedAt        time.Time        `json:"used_at"`
}

type PropagationState string

const (
	PropagationPending PropagationState = "PENDING"
	PropagationRunning PropagationState = "RUNNING"
	PropagationDone    PropagationState = "DONE"
	PropagationLimited PropagationState = "LIMITED"
	PropagationFailed  PropagationState = "FAILED"
)

type PropagationNodeKind string

const (
	PropagationEvidence PropagationNodeKind = "evidence"
	PropagationBelief   PropagationNodeKind = "belief"
)

type PropagationJob struct {
	ID             id.PropagationID `json:"id"`
	Cause          string           `json:"cause"`
	State          PropagationState `json:"state"`
	MaxDepth       int              `json:"max_depth"`
	MaxWork        int              `json:"max_work"`
	ProcessedCount int              `json:"processed_count"`
	CreatedAt      time.Time        `json:"created_at"`
	UpdatedAt      time.Time        `json:"updated_at"`
	LastError      string           `json:"last_error,omitempty"`
}

type PropagationItem struct {
	JobID      id.PropagationID   `json:"job_id"`
	Kind       PropagationNodeKind `json:"kind"`
	NodeID     string             `json:"node_id"`
	Depth      int                `json:"depth"`
	Processed  bool               `json:"processed"`
	EnqueuedAt time.Time          `json:"enqueued_at"`
	ProcessedAt *time.Time        `json:"processed_at,omitempty"`
}

type EvidenceInput struct {
	ActorID       id.AgentID
	Kind          EvidenceKind
	Scope         Scope
	SourceRef     string
	Content       []byte
	ObjectRef     objectstore.Ref
	ContentHash   string
	ObservedAt    time.Time
	SourceVersion string
	Provenance    ProvenanceClass
	TrustClass    TrustClass
	Sensitivity   SensitivityClass
}

type BeliefInput struct {
	ActorID       id.AgentID
	Proposition   string
	Scope         Scope
	Status        BeliefStatus
	Confidence    ConfidenceProfile
	Validity      ValidityWindow
	Evidence      []EvidenceLinkInput
	Dependencies  []BeliefDependencyInput
	Speculative   bool
	OriginForkID  id.ForkID
}

type EvidenceLinkInput struct {
	EvidenceID id.EvidenceID
	Relation   EvidenceRelation
}

type BeliefDependencyInput struct {
	BeliefID  id.BeliefID
	Relation  BeliefRelation
}

type SearchQuery struct {
	Text              string
	Scopes            []Scope
	Statuses          []BeliefStatus
	Limit             int
	AsOf              *time.Time
	IncludeHistorical bool
	IncludeSpeculative bool
}

type SearchCandidate struct {
	Belief       Belief
	LexicalScore float64
}

type ContradictionSummary struct {
	BeliefID    id.BeliefID  `json:"belief_id"`
	Proposition string       `json:"proposition"`
	Status      BeliefStatus `json:"status"`
}

type RetrievedBelief struct {
	Belief          Belief                 `json:"belief"`
	Score           float64                `json:"score"`
	Evidence        []EvidenceLink         `json:"evidence"`
	Provenance      []ProvenanceClass      `json:"provenance"`
	Contradictions  []ContradictionSummary `json:"contradictions,omitempty"`
	NeedsEvidence   bool                   `json:"needs_evidence"`
	AuthoritySource string                 `json:"authority_source"`
}

type InspectResult struct {
	Belief         Belief             `json:"belief"`
	Evidence       []Evidence         `json:"evidence"`
	EvidenceLinks  []EvidenceLink     `json:"evidence_links"`
	Incoming       []BeliefEdge       `json:"incoming"`
	Outgoing       []BeliefEdge       `json:"outgoing"`
	History        []StatusTransition `json:"history"`
	Uses           []UsageRecord      `json:"uses"`
	Provenance     []ProvenanceClass  `json:"provenance"`
	Contradictions []Belief           `json:"contradictions"`
}

func NormalizeProposition(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
}

func PropositionFingerprint(value string) string {
	sum := sha256.Sum256([]byte(NormalizeProposition(value)))
	return hex.EncodeToString(sum[:])
}

func uniqueProvenance(values []ProvenanceClass) []ProvenanceClass {
	set := make(map[ProvenanceClass]struct{}, len(values))
	for _, value := range values {
		if value != "" {
			set[value] = struct{}{}
		}
	}
	out := make([]ProvenanceClass, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
