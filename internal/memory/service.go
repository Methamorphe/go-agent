package memory

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Methamorphe/go-agent/internal/clock"
	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
)

type Config struct {
	MaxPropagationDepth    int
	MaxPropagationWork     int
	PropagationBatchSize   int
	RetrievalCandidateLimit int
	ProvenanceDepth        int
}

func DefaultConfig() Config {
	return Config{
		MaxPropagationDepth:     64,
		MaxPropagationWork:      4096,
		PropagationBatchSize:    128,
		RetrievalCandidateLimit: 128,
		ProvenanceDepth:         32,
	}
}

type Service struct {
	repo    Repository
	objects ObjectStore
	ids     IDGenerator
	clock   clock.Clock
	policy  WritePolicy
	cfg     Config
}

func New(repo Repository, objects ObjectStore, ids IDGenerator, source clock.Clock, policy WritePolicy, cfg Config) (*Service, error) {
	if repo == nil {
		return nil, errs.New(errs.CodeInvalidArgument, "memory.new", "repository is required")
	}
	if ids == nil {
		return nil, errs.New(errs.CodeInvalidArgument, "memory.new", "id generator is required")
	}
	if source == nil {
		source = clock.NewSystemClock()
	}
	if policy == nil {
		policy = LocalOnlyPolicy{}
	}
	defaults := DefaultConfig()
	if cfg.MaxPropagationDepth <= 0 {
		cfg.MaxPropagationDepth = defaults.MaxPropagationDepth
	}
	if cfg.MaxPropagationWork <= 0 {
		cfg.MaxPropagationWork = defaults.MaxPropagationWork
	}
	if cfg.PropagationBatchSize <= 0 {
		cfg.PropagationBatchSize = defaults.PropagationBatchSize
	}
	if cfg.RetrievalCandidateLimit <= 0 {
		cfg.RetrievalCandidateLimit = defaults.RetrievalCandidateLimit
	}
	if cfg.ProvenanceDepth <= 0 {
		cfg.ProvenanceDepth = defaults.ProvenanceDepth
	}
	return &Service{repo: repo, objects: objects, ids: ids, clock: source, policy: policy, cfg: cfg}, nil
}

func (s *Service) RecordEvidence(ctx context.Context, input EvidenceInput) (Evidence, error) {
	if input.ActorID == "" {
		return Evidence{}, errs.New(errs.CodeInvalidArgument, "memory.record_evidence", "actor id is required")
	}
	if !input.Scope.Valid() {
		return Evidence{}, errs.New(errs.CodeInvalidArgument, "memory.record_evidence", "invalid scope")
	}
	if !s.policy.AuthorizeMemoryWrite(ctx, input.ActorID, WritePropose, input.Scope) {
		return Evidence{}, ErrPolicyDenied
	}
	if !validEvidenceProvenance(input.Provenance) {
		return Evidence{}, errs.New(errs.CodeInvalidArgument, "memory.record_evidence", "invalid evidence provenance")
	}
	if strings.TrimSpace(input.SourceRef) == "" {
		return Evidence{}, errs.New(errs.CodeInvalidArgument, "memory.record_evidence", "source ref is required")
	}
	if input.Kind == "" {
		input.Kind = EvidenceOther
	}
	if input.TrustClass == "" {
		input.TrustClass = TrustUnknown
	}
	if input.Sensitivity == "" {
		input.Sensitivity = SensitivityInternal
	}
	if input.ObservedAt.IsZero() {
		input.ObservedAt = s.clock.Now().UTC()
	}

	objectRef := input.ObjectRef
	contentHash := strings.ToLower(strings.TrimSpace(input.ContentHash))
	if len(input.Content) > 0 {
		if s.objects == nil {
			return Evidence{}, errs.New(errs.CodeInvalidArgument, "memory.record_evidence", "object store is required when evidence content is provided")
		}
		sum := sha256.Sum256(input.Content)
		actualHash := hex.EncodeToString(sum[:])
		if contentHash != "" && contentHash != actualHash {
			return Evidence{}, errs.New(errs.CodeConflict, "memory.record_evidence", "provided content hash does not match evidence content")
		}
		meta, err := s.objects.Put(ctx, bytes.NewReader(input.Content))
		if err != nil {
			return Evidence{}, err
		}
		objectRef = meta.Ref
		contentHash = actualHash
	}
	if contentHash == "" && objectRef != "" {
		contentHash = hashFromObjectRef(string(objectRef))
	}
	if contentHash == "" {
		return Evidence{}, errs.New(errs.CodeInvalidArgument, "memory.record_evidence", "content hash or evidence content is required")
	}
	if len(contentHash) != sha256.Size*2 {
		return Evidence{}, errs.New(errs.CodeInvalidArgument, "memory.record_evidence", "content hash must be a sha256 digest")
	}

	evidenceID, err := s.ids.Evidence()
	if err != nil {
		return Evidence{}, errs.Wrap(errs.CodeInternal, "memory.record_evidence", "generate evidence id", err)
	}
	evidence := Evidence{
		ID: evidenceID, Kind: input.Kind, Scope: input.Scope, SourceRef: strings.TrimSpace(input.SourceRef),
		ObjectRef: objectRef, ContentHash: contentHash, ObservedAt: input.ObservedAt.UTC(),
		SourceVersion: strings.TrimSpace(input.SourceVersion), Provenance: input.Provenance,
		TrustClass: input.TrustClass, Sensitivity: input.Sensitivity,
	}
	if err := s.repo.PutEvidence(ctx, evidence); err != nil {
		return Evidence{}, err
	}
	return evidence, nil
}

func (s *Service) CreateBelief(ctx context.Context, input BeliefInput) (Belief, error) {
	return s.createBelief(ctx, input, 1)
}

func (s *Service) createBelief(ctx context.Context, input BeliefInput, version uint32) (Belief, error) {
	if input.ActorID == "" {
		return Belief{}, errs.New(errs.CodeInvalidArgument, "memory.create_belief", "actor id is required")
	}
	if !input.Scope.Valid() {
		return Belief{}, errs.New(errs.CodeInvalidArgument, "memory.create_belief", "invalid scope")
	}
	proposition := strings.TrimSpace(input.Proposition)
	if proposition == "" {
		return Belief{}, errs.New(errs.CodeInvalidArgument, "memory.create_belief", "proposition is required")
	}
	status := input.Status
	if status == "" {
		status = BeliefCandidate
	}
	if !status.Valid() {
		return Belief{}, errs.New(errs.CodeInvalidArgument, "memory.create_belief", "invalid belief status")
	}
	action := WritePropose
	if status != BeliefCandidate {
		action = WritePublish
	}
	if !s.policy.AuthorizeMemoryWrite(ctx, input.ActorID, action, input.Scope) {
		return Belief{}, ErrPolicyDenied
	}
	if input.Speculative && input.OriginForkID == "" {
		return Belief{}, errs.New(errs.CodeInvalidArgument, "memory.create_belief", "speculative belief requires an origin fork")
	}

	evidence := make([]Evidence, 0, len(input.Evidence))
	seenEvidence := make(map[id.EvidenceID]struct{}, len(input.Evidence))
	for _, link := range input.Evidence {
		if link.EvidenceID == "" || !link.Relation.Valid() {
			return Belief{}, errs.New(errs.CodeInvalidArgument, "memory.create_belief", "invalid evidence link")
		}
		if _, ok := seenEvidence[link.EvidenceID]; ok {
			return Belief{}, errs.New(errs.CodeConflict, "memory.create_belief", "duplicate evidence link")
		}
		item, err := s.repo.Evidence(ctx, link.EvidenceID)
		if err != nil {
			return Belief{}, err
		}
		evidence = append(evidence, item)
		seenEvidence[link.EvidenceID] = struct{}{}
	}

	dependencies := make([]Belief, 0, len(input.Dependencies))
	seenDependencies := make(map[id.BeliefID]struct{}, len(input.Dependencies))
	for _, dependency := range input.Dependencies {
		if dependency.BeliefID == "" || !dependency.Relation.Valid() {
			return Belief{}, errs.New(errs.CodeInvalidArgument, "memory.create_belief", "invalid belief dependency")
		}
		if _, ok := seenDependencies[dependency.BeliefID]; ok {
			return Belief{}, errs.New(errs.CodeConflict, "memory.create_belief", "duplicate belief dependency")
		}
		item, err := s.repo.Belief(ctx, dependency.BeliefID)
		if err != nil {
			return Belief{}, err
		}
		dependencies = append(dependencies, item)
		seenDependencies[dependency.BeliefID] = struct{}{}
	}

	profile := deriveConfidence(input.Confidence, evidence, input.Evidence, dependencies, input.Dependencies)
	if profile.ConflictLevel == ConflictVerified && !status.HistoricalOnly() {
		status = BeliefContested
	}
	origin := ProvenanceModelInference
	if len(evidence) > 0 || len(dependencies) > 0 {
		origin = ProvenanceConsolidated
	}
	now := s.clock.Now().UTC()
	beliefID, err := s.ids.Belief()
	if err != nil {
		return Belief{}, errs.Wrap(errs.CodeInternal, "memory.create_belief", "generate belief id", err)
	}
	belief := Belief{
		ID: beliefID, Proposition: proposition, Fingerprint: PropositionFingerprint(proposition), Scope: input.Scope,
		Status: status, OriginProvenance: origin, Confidence: profile, Validity: normalizeValidity(input.Validity),
		CreatedAt: now, UpdatedAt: now, Version: version, Speculative: input.Speculative, OriginForkID: input.OriginForkID,
	}
	links := make([]EvidenceLink, 0, len(input.Evidence))
	for _, link := range input.Evidence {
		links = append(links, EvidenceLink{BeliefID: beliefID, EvidenceID: link.EvidenceID, Relation: link.Relation})
	}
	edges := make([]BeliefEdge, 0, len(input.Dependencies)*2)
	for _, dependency := range input.Dependencies {
		switch dependency.Relation {
		case BeliefSupersedes:
			edges = append(edges, BeliefEdge{From: beliefID, To: dependency.BeliefID, Relation: BeliefSupersedes, CreatedAt: now})
		case BeliefContradicts:
			edges = append(edges,
				BeliefEdge{From: dependency.BeliefID, To: beliefID, Relation: BeliefContradicts, CreatedAt: now},
				BeliefEdge{From: beliefID, To: dependency.BeliefID, Relation: BeliefContradicts, CreatedAt: now},
			)
		default:
			edges = append(edges, BeliefEdge{From: dependency.BeliefID, To: beliefID, Relation: dependency.Relation, CreatedAt: now})
		}
	}
	if err := s.repo.CreateBelief(ctx, belief, links, edges); err != nil {
		return Belief{}, err
	}
	for _, dependency := range input.Dependencies {
		switch dependency.Relation {
		case BeliefContradicts:
			_, _ = s.transition(ctx, dependency.BeliefID, BeliefContested, "verified contradiction linked", "")
		case BeliefSupersedes:
			_, _ = s.transition(ctx, dependency.BeliefID, BeliefSuperseded, "superseded by "+beliefID.String(), "")
		}
	}
	return belief, nil
}

func (s *Service) AmendBelief(ctx context.Context, oldID id.BeliefID, input BeliefInput) (Belief, error) {
	old, err := s.repo.Belief(ctx, oldID)
	if err != nil {
		return Belief{}, err
	}
	if input.Scope.Kind == "" {
		input.Scope = old.Scope
	}
	input.Dependencies = append(input.Dependencies, BeliefDependencyInput{BeliefID: oldID, Relation: BeliefSupersedes})
	if input.Status == "" {
		input.Status = BeliefActive
	}
	return s.createBelief(ctx, input, old.Version+1)
}

func (s *Service) LinkBeliefs(ctx context.Context, actor id.AgentID, fromID, toID id.BeliefID, relation BeliefRelation) error {
	if actor == "" || fromID == "" || toID == "" || fromID == toID || !relation.Valid() {
		return errs.New(errs.CodeInvalidArgument, "memory.link_beliefs", "invalid belief edge")
	}
	from, err := s.repo.Belief(ctx, fromID)
	if err != nil {
		return err
	}
	to, err := s.repo.Belief(ctx, toID)
	if err != nil {
		return err
	}
	if !s.policy.AuthorizeMemoryWrite(ctx, actor, WritePublish, to.Scope) {
		return ErrPolicyDenied
	}
	now := s.clock.Now().UTC()
	if relation == BeliefContradicts {
		if err := s.repo.PutBeliefEdge(ctx, BeliefEdge{From: fromID, To: toID, Relation: relation, CreatedAt: now}); err != nil && !errors.Is(err, ErrConflict) {
			return err
		}
		if err := s.repo.PutBeliefEdge(ctx, BeliefEdge{From: toID, To: fromID, Relation: relation, CreatedAt: now}); err != nil && !errors.Is(err, ErrConflict) {
			return err
		}
		_, _ = s.transition(ctx, from.ID, BeliefContested, "verified contradiction linked", "")
		_, _ = s.transition(ctx, to.ID, BeliefContested, "verified contradiction linked", "")
		return nil
	}
	if err := s.repo.PutBeliefEdge(ctx, BeliefEdge{From: fromID, To: toID, Relation: relation, CreatedAt: now}); err != nil {
		return err
	}
	if relation == BeliefSupersedes {
		_, err = s.transition(ctx, toID, BeliefSuperseded, "superseded by "+fromID.String(), "")
		return err
	}
	return nil
}

func (s *Service) SetStatus(ctx context.Context, actor id.AgentID, beliefID id.BeliefID, target BeliefStatus, reason string) (Belief, error) {
	belief, err := s.repo.Belief(ctx, beliefID)
	if err != nil {
		return Belief{}, err
	}
	action := WritePublish
	if target == BeliefInvalidated || target == BeliefRejected || target == BeliefSuperseded {
		action = WriteInvalidate
	}
	if !s.policy.AuthorizeMemoryWrite(ctx, actor, action, belief.Scope) {
		return Belief{}, ErrPolicyDenied
	}
	if !allowedTransition(belief.Status, target) {
		return Belief{}, fmt.Errorf("%w: %s -> %s", ErrInvalidState, belief.Status, target)
	}
	return s.transition(ctx, beliefID, target, reason, "")
}

func (s *Service) RetractEvidence(ctx context.Context, actor id.AgentID, evidenceID id.EvidenceID, reason string) (PropagationJob, error) {
	evidence, err := s.repo.Evidence(ctx, evidenceID)
	if err != nil {
		return PropagationJob{}, err
	}
	if !s.policy.AuthorizeMemoryWrite(ctx, actor, WriteInvalidate, evidence.Scope) {
		return PropagationJob{}, ErrPolicyDenied
	}
	if strings.TrimSpace(reason) == "" {
		reason = "evidence retracted"
	}
	invalidation := EvidenceInvalidation{EvidenceID: evidenceID, Reason: reason, InvalidatedAt: s.clock.Now().UTC()}
	if err := s.repo.InvalidateEvidence(ctx, invalidation); err != nil && !errors.Is(err, ErrConflict) {
		return PropagationJob{}, err
	}
	return s.startPropagation(ctx, reason, []PropagationItem{{Kind: PropagationEvidence, NodeID: evidenceID.String()}})
}

func (s *Service) ObserveSourceVersion(ctx context.Context, actor id.AgentID, version SourceVersion) (PropagationJob, error) {
	version.SourceRef = strings.TrimSpace(version.SourceRef)
	version.Version = strings.TrimSpace(version.Version)
	version.ContentHash = strings.ToLower(strings.TrimSpace(version.ContentHash))
	if version.SourceRef == "" || (version.Version == "" && version.ContentHash == "") {
		return PropagationJob{}, errs.New(errs.CodeInvalidArgument, "memory.observe_source_version", "source ref and version or content hash are required")
	}
	if version.ObservedAt.IsZero() {
		version.ObservedAt = s.clock.Now().UTC()
	}
	if err := s.repo.PutSourceVersion(ctx, version); err != nil && !errors.Is(err, ErrConflict) {
		return PropagationJob{}, err
	}
	evidence, err := s.repo.EvidenceBySource(ctx, version.SourceRef)
	if err != nil {
		return PropagationJob{}, err
	}
	roots := make([]PropagationItem, 0)
	for _, item := range evidence {
		obsolete := false
		if version.Version != "" && item.SourceVersion != "" && item.SourceVersion != version.Version {
			obsolete = true
		}
		if version.ContentHash != "" && item.ContentHash != version.ContentHash {
			obsolete = true
		}
		if !obsolete {
			continue
		}
		if !s.policy.AuthorizeMemoryWrite(ctx, actor, WriteInvalidate, item.Scope) {
			return PropagationJob{}, ErrPolicyDenied
		}
		invalidation := EvidenceInvalidation{EvidenceID: item.ID, Reason: "source changed: " + version.SourceRef, InvalidatedAt: version.ObservedAt.UTC()}
		if err := s.repo.InvalidateEvidence(ctx, invalidation); err != nil && !errors.Is(err, ErrConflict) {
			return PropagationJob{}, err
		}
		roots = append(roots, PropagationItem{Kind: PropagationEvidence, NodeID: item.ID.String()})
	}
	if len(roots) == 0 {
		return PropagationJob{}, nil
	}
	return s.startPropagation(ctx, "source changed: "+version.SourceRef, roots)
}

func (s *Service) RecordUse(ctx context.Context, record UsageRecord) error {
	if record.BeliefID == "" || record.AgentID == "" {
		return errs.New(errs.CodeInvalidArgument, "memory.record_use", "belief and agent ids are required")
	}
	if record.UsedAt.IsZero() {
		record.UsedAt = s.clock.Now().UTC()
	}
	return s.repo.RecordBeliefUse(ctx, record)
}

func (s *Service) transition(ctx context.Context, beliefID id.BeliefID, target BeliefStatus, reason string, jobID id.PropagationID) (Belief, error) {
	belief, err := s.repo.Belief(ctx, beliefID)
	if err != nil {
		return Belief{}, err
	}
	if belief.Status == target {
		return belief, nil
	}
	if !allowedTransition(belief.Status, target) {
		return belief, nil
	}
	profile := belief.Confidence
	now := s.clock.Now().UTC()
	var reviewed *time.Time
	switch target {
	case BeliefStale, BeliefNeedsReview:
		profile.Freshness = FreshnessStale
	case BeliefContested:
		profile.ConflictLevel = ConflictVerified
	case BeliefActive:
		profile.ConflictLevel = ConflictNone
		if profile.Freshness == FreshnessStale || profile.Freshness == FreshnessUnknown {
			profile.Freshness = FreshnessFresh
		}
		reviewed = &now
	}
	return s.repo.TransitionBelief(ctx, beliefID, belief.Status, target, profile, reviewed, strings.TrimSpace(reason), jobID, now)
}

func allowedTransition(from, to BeliefStatus) bool {
	if from == to {
		return true
	}
	switch from {
	case BeliefCandidate:
		return to == BeliefActive || to == BeliefContested || to == BeliefNeedsReview || to == BeliefRejected || to == BeliefInvalidated || to == BeliefSuperseded
	case BeliefActive:
		return to == BeliefContested || to == BeliefStale || to == BeliefNeedsReview || to == BeliefInvalidated || to == BeliefSuperseded || to == BeliefRejected
	case BeliefContested:
		return to == BeliefActive || to == BeliefStale || to == BeliefNeedsReview || to == BeliefInvalidated || to == BeliefSuperseded || to == BeliefRejected
	case BeliefStale, BeliefNeedsReview:
		return to == BeliefActive || to == BeliefContested || to == BeliefInvalidated || to == BeliefSuperseded || to == BeliefRejected || to == BeliefStale || to == BeliefNeedsReview
	case BeliefInvalidated, BeliefSuperseded, BeliefRejected:
		return false
	default:
		return false
	}
}

func deriveConfidence(requested ConfidenceProfile, evidence []Evidence, links []EvidenceLinkInput, dependencies []Belief, dependencyLinks []BeliefDependencyInput) ConfidenceProfile {
	profile := requested
	profile.Corroboration = 0
	profile.InferenceDepth = 0
	profile.EvidenceStrength = StrengthUnknown
	profile.Verification = VerificationNone
	profile.ConflictLevel = ConflictNone
	if len(evidence) > 0 {
		profile.Freshness = FreshnessFresh
	} else if profile.Freshness == "" {
		profile.Freshness = FreshnessUnknown
	}
	for i, item := range evidence {
		relation := links[i].Relation
		if relation == EvidenceSupports || relation == EvidenceVerifies {
			profile.Corroboration++
		}
		profile.EvidenceStrength = strongerEvidence(profile.EvidenceStrength, strengthForEvidence(item))
		if relation == EvidenceContradicts {
			profile.ConflictLevel = ConflictVerified
		}
		if relation == EvidenceVerifies {
			if item.Provenance == ProvenanceRuntimeVerified || item.Kind == EvidenceTestResult {
				profile.Verification = VerificationTestConfirmed
			} else if profile.Verification == VerificationNone {
				profile.Verification = VerificationSourceChecked
			}
		}
	}
	for i, dependency := range dependencies {
		depth := dependency.Confidence.InferenceDepth + 1
		if depth > profile.InferenceDepth {
			profile.InferenceDepth = depth
		}
		if dependencyLinks[i].Relation == BeliefContradicts {
			profile.ConflictLevel = ConflictVerified
		}
	}
	if len(dependencies) == 0 && len(evidence) == 0 {
		profile.InferenceDepth = 1
	}
	if profile.Verification == VerificationNone && profile.Corroboration > 1 {
		profile.Verification = VerificationCorroborated
	}
	return profile
}

func strengthForEvidence(evidence Evidence) EvidenceStrength {
	switch evidence.Provenance {
	case ProvenanceRuntimeVerified:
		return StrengthRuntimeVerified
	case ProvenanceUserExplicit:
		return StrengthDirectAssertion
	case ProvenanceRepositorySource:
		return StrengthRepository
	case ProvenanceTrustedConnector, ProvenanceExternalSource:
		return StrengthExternal
	default:
		return StrengthWeakSuggestion
	}
}

func strongerEvidence(left, right EvidenceStrength) EvidenceStrength {
	rank := func(value EvidenceStrength) int {
		switch value {
		case StrengthRuntimeVerified:
			return 6
		case StrengthDirectAssertion:
			return 5
		case StrengthRepository:
			return 4
		case StrengthExternal:
			return 3
		case StrengthWeakSuggestion:
			return 2
		default:
			return 1
		}
	}
	if rank(right) > rank(left) {
		return right
	}
	return left
}

func normalizeValidity(value ValidityWindow) ValidityWindow {
	if value.ValidFrom != nil {
		t := value.ValidFrom.UTC()
		value.ValidFrom = &t
	}
	if value.ValidUntil != nil {
		t := value.ValidUntil.UTC()
		value.ValidUntil = &t
	}
	return value
}

func hashFromObjectRef(ref string) string {
	algorithm, digest, ok := strings.Cut(ref, ":")
	if ok && algorithm == "sha256" {
		return strings.ToLower(digest)
	}
	return ""
}
