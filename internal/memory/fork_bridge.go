package memory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Methamorphe/go-agent/internal/fork"
	"github.com/Methamorphe/go-agent/internal/id"
)

type ForkBeliefProposal struct {
	Proposition  string                  `json:"proposition"`
	Scope        Scope                   `json:"scope"`
	Status       BeliefStatus            `json:"status,omitempty"`
	Confidence   ConfidenceProfile       `json:"confidence"`
	Validity     ValidityWindow          `json:"validity"`
	Evidence     []EvidenceLinkInput     `json:"evidence,omitempty"`
	Dependencies []BeliefDependencyInput `json:"dependencies,omitempty"`
}

type ForkPromotionLookup interface {
	PromotedForkBelief(context.Context, id.ForkID, string, Scope) (Belief, error)
}

func NewForkBeliefOverlay(key string, forkID id.ForkID, proposal ForkBeliefProposal, createdAt time.Time) (fork.OverlayEntry, error) {
	if forkID == "" || strings.TrimSpace(proposal.Proposition) == "" || !proposal.Scope.Valid() {
		return fork.OverlayEntry{}, fmt.Errorf("invalid fork belief proposal")
	}
	if strings.TrimSpace(key) == "" {
		key = PropositionFingerprint(proposal.Proposition)
	}
	body, err := json.Marshal(proposal)
	if err != nil {
		return fork.OverlayEntry{}, err
	}
	sum := sha256.Sum256(body)
	return fork.OverlayEntry{
		Kind: fork.OverlayMemory,
		Key: key,
		Value: body,
		SourceRef: "fork:" + forkID.String(),
		ProvenanceRef: "sha256:" + hex.EncodeToString(sum[:]),
		Promotable: true,
		CreatedAt: createdAt.UTC(),
	}, nil
}

type ForkPromoter struct {
	Memory *Service
}

func (p ForkPromoter) PromoteForkArtifact(ctx context.Context, operationKey string, checkpoint fork.Checkpoint, branch fork.Branch, entry fork.OverlayEntry) error {
	if p.Memory == nil {
		return errors.New("epistemic memory service is required")
	}
	if strings.TrimSpace(operationKey) == "" {
		return errors.New("fork promotion operation key is required")
	}
	if entry.Kind != fork.OverlayMemory {
		return fmt.Errorf("epistemic memory promoter cannot promote overlay kind %s", entry.Kind)
	}
	if branch.ID == "" || branch.AgentID == "" || checkpoint.ID == "" {
		return errors.New("fork promotion identity is incomplete")
	}
	if entry.SourceRef != "fork:"+branch.ID.String() {
		return errors.New("fork memory overlay source does not match promoted branch")
	}
	sum := sha256.Sum256(entry.Value)
	expectedProvenance := "sha256:" + hex.EncodeToString(sum[:])
	if entry.ProvenanceRef != expectedProvenance {
		return errors.New("fork memory overlay provenance digest mismatch")
	}
	var proposal ForkBeliefProposal
	if err := json.Unmarshal(entry.Value, &proposal); err != nil {
		return fmt.Errorf("decode fork belief proposal: %w", err)
	}
	if strings.TrimSpace(proposal.Proposition) == "" || !proposal.Scope.Valid() {
		return errors.New("fork belief proposal is invalid")
	}
	fingerprint := PropositionFingerprint(proposal.Proposition)
	lookup, _ := p.Memory.repo.(ForkPromotionLookup)
	if lookup != nil {
		if _, err := lookup.PromotedForkBelief(ctx, branch.ID, fingerprint, proposal.Scope); err == nil {
			return nil
		} else if !errors.Is(err, ErrNotFound) {
			return err
		}
	}
	status := proposal.Status
	if status == "" {
		status = BeliefCandidate
	}
	_, err := p.Memory.CreateBelief(ctx, BeliefInput{
		ActorID: branch.AgentID,
		Proposition: proposal.Proposition,
		Scope: proposal.Scope,
		Status: status,
		Confidence: proposal.Confidence,
		Validity: proposal.Validity,
		Evidence: append([]EvidenceLinkInput(nil), proposal.Evidence...),
		Dependencies: append([]BeliefDependencyInput(nil), proposal.Dependencies...),
		Speculative: false,
		OriginForkID: branch.ID,
	})
	if err == nil {
		return nil
	}
	if !errors.Is(err, ErrConflict) || lookup == nil {
		return err
	}
	_, lookupErr := lookup.PromotedForkBelief(ctx, branch.ID, fingerprint, proposal.Scope)
	return lookupErr
}

var _ fork.CognitivePromoter = ForkPromoter{}
