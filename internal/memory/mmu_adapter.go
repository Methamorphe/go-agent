package memory

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/mmu"
)

type MMUPageWriter interface {
	CreatePage(context.Context, mmu.PageInput) (mmu.PageMeta, error)
}

type mmuBeliefPayload struct {
	BeliefID         id.BeliefID            `json:"belief_id"`
	Proposition      string                 `json:"proposition"`
	Status           BeliefStatus           `json:"status"`
	Confidence       ConfidenceProfile      `json:"confidence"`
	Provenance       []ProvenanceClass      `json:"provenance"`
	Evidence         []EvidenceLink         `json:"evidence_refs"`
	Contradictions   []ContradictionSummary `json:"contradictions,omitempty"`
	NeedsEvidence    bool                   `json:"needs_evidence"`
	AuthoritySource  string                 `json:"authority_source"`
}

func ToMMUPageInput(agentID id.AgentID, item RetrievedBelief) (mmu.PageInput, error) {
	if agentID == "" || item.Belief.ID == "" {
		return mmu.PageInput{}, errs.New(errs.CodeInvalidArgument, "memory.mmu.page", "agent and belief ids are required")
	}
	payload := mmuBeliefPayload{
		BeliefID: item.Belief.ID,
		Proposition: item.Belief.Proposition,
		Status: item.Belief.Status,
		Confidence: item.Belief.Confidence,
		Provenance: append([]ProvenanceClass(nil), item.Provenance...),
		Evidence: append([]EvidenceLink(nil), item.Evidence...),
		Contradictions: append([]ContradictionSummary(nil), item.Contradictions...),
		NeedsEvidence: item.NeedsEvidence,
		AuthoritySource: item.AuthoritySource,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return mmu.PageInput{}, errs.Wrap(errs.CodeInternal, "memory.mmu.page", "encode belief payload", err)
	}
	return mmu.PageInput{
		AgentID: agentID,
		Type: mmu.PageRecallResult,
		Scope: mmuScope(item.Belief.Scope.Kind),
		SourceRef: "belief:" + item.Belief.ID.String(),
		Content: string(body),
		Importance: beliefImportance(item.Belief.Status),
		Confidence: beliefNumericConfidence(item.Belief),
	}, nil
}

func (s *Service) PageBeliefIntoMMU(ctx context.Context, writer MMUPageWriter, agentID id.AgentID, item RetrievedBelief) (mmu.PageMeta, error) {
	if writer == nil {
		return mmu.PageMeta{}, errs.New(errs.CodeInvalidArgument, "memory.mmu.page", "MMU writer is required")
	}
	input, err := ToMMUPageInput(agentID, item)
	if err != nil {
		return mmu.PageMeta{}, err
	}
	page, err := writer.CreatePage(ctx, input)
	if err != nil {
		return mmu.PageMeta{}, err
	}
	if err := s.RecordUse(ctx, UsageRecord{BeliefID: item.Belief.ID, AgentID: agentID, ContextPageID: page.ID, Purpose: "cognitive_mmu", UsedAt: s.clock.Now().UTC()}); err != nil {
		return mmu.PageMeta{}, err
	}
	return page, nil
}

func mmuScope(scope ScopeKind) mmu.Scope {
	switch scope {
	case ScopeProcess:
		return mmu.ScopeProcess
	case ScopeRootTask:
		return mmu.ScopeRootTask
	case ScopeProject:
		return mmu.ScopeProject
	case ScopeWorkspace:
		return mmu.ScopeWorkspace
	case ScopeUser:
		return mmu.ScopeUser
	case ScopeWorld:
		return mmu.ScopeWorld
	default:
		return mmu.ScopeProcess
	}
}

func beliefImportance(status BeliefStatus) float64 {
	switch status {
	case BeliefActive:
		return 0.9
	case BeliefCandidate:
		return 0.65
	case BeliefContested:
		return 0.7
	case BeliefNeedsReview, BeliefStale:
		return 0.5
	default:
		return 0.25
	}
}

func beliefNumericConfidence(belief Belief) float64 {
	value := 0.35
	switch belief.Confidence.EvidenceStrength {
	case StrengthRuntimeVerified:
		value = 0.95
	case StrengthDirectAssertion:
		value = 0.85
	case StrengthRepository:
		value = 0.8
	case StrengthExternal:
		value = 0.6
	case StrengthWeakSuggestion:
		value = 0.45
	}
	if belief.Confidence.Freshness == FreshnessStale || belief.Status == BeliefStale || belief.Status == BeliefNeedsReview {
		value *= 0.4
	}
	if belief.Status == BeliefContested || belief.Confidence.ConflictLevel == ConflictVerified {
		value *= 0.5
	}
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

var _ = fmt.Sprintf
