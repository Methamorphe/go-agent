package memory

import (
	"context"
	"errors"
	"sort"

	"github.com/Methamorphe/go-agent/internal/id"
)

func (s *Service) Retrieve(ctx context.Context, query SearchQuery) ([]RetrievedBelief, error) {
	if query.Limit <= 0 {
		query.Limit = 8
	}
	if query.Limit > s.cfg.RetrievalCandidateLimit {
		query.Limit = s.cfg.RetrievalCandidateLimit
	}
	candidateQuery := query
	candidateQuery.Limit = s.cfg.RetrievalCandidateLimit
	candidates, err := s.repo.SearchBeliefs(ctx, candidateQuery)
	if err != nil {
		return nil, err
	}
	asOf := s.clock.Now().UTC()
	if query.AsOf != nil {
		asOf = query.AsOf.UTC()
	}
	results := make([]RetrievedBelief, 0, len(candidates))
	for _, candidate := range candidates {
		belief := candidate.Belief
		if belief.CreatedAt.After(asOf) || !belief.Validity.Contains(asOf) {
			continue
		}
		if belief.Speculative && !query.IncludeSpeculative {
			continue
		}
		if belief.Status.HistoricalOnly() && !query.IncludeHistorical {
			continue
		}
		if !matchesScopes(belief.Scope, query.Scopes) {
			continue
		}
		if len(query.Statuses) > 0 && !containsStatus(query.Statuses, belief.Status) {
			continue
		}
		links, err := s.repo.BeliefEvidence(ctx, belief.ID)
		if err != nil {
			return nil, err
		}
		provenance, err := s.provenance(ctx, belief.ID)
		if err != nil {
			return nil, err
		}
		contradictions, err := s.contradictionSummaries(ctx, belief.ID)
		if err != nil {
			return nil, err
		}
		invalidatedEvidence := false
		for _, link := range links {
			invalidation, lookupErr := s.repo.EvidenceInvalidation(ctx, link.EvidenceID)
			if lookupErr != nil && !errors.Is(lookupErr, ErrNotFound) {
				return nil, lookupErr
			}
			if invalidation != nil {
				invalidatedEvidence = true
				break
			}
		}
		score := epistemicScore(belief, candidate.LexicalScore, query.Scopes, invalidatedEvidence)
		needsEvidence := invalidatedEvidence || belief.Status == BeliefContested || belief.Status == BeliefStale || belief.Status == BeliefNeedsReview || belief.Confidence.Verification == VerificationNone
		results = append(results, RetrievedBelief{
			Belief: belief,
			Score: score,
			Evidence: links,
			Provenance: provenance,
			Contradictions: contradictions,
			NeedsEvidence: needsEvidence,
			AuthoritySource: "none: memory is reasoning input; capability or approval remains authoritative",
		})
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score == results[j].Score {
			if results[i].Belief.UpdatedAt.Equal(results[j].Belief.UpdatedAt) {
				return results[i].Belief.ID.String() < results[j].Belief.ID.String()
			}
			return results[i].Belief.UpdatedAt.After(results[j].Belief.UpdatedAt)
		}
		return results[i].Score > results[j].Score
	})
	if len(results) > query.Limit {
		results = results[:query.Limit]
	}
	return results, nil
}

func (s *Service) Inspect(ctx context.Context, beliefID id.BeliefID) (InspectResult, error) {
	belief, err := s.repo.Belief(ctx, beliefID)
	if err != nil {
		return InspectResult{}, err
	}
	links, err := s.repo.BeliefEvidence(ctx, beliefID)
	if err != nil {
		return InspectResult{}, err
	}
	evidence := make([]Evidence, 0, len(links))
	for _, link := range links {
		item, err := s.repo.Evidence(ctx, link.EvidenceID)
		if err != nil {
			return InspectResult{}, err
		}
		evidence = append(evidence, item)
	}
	incoming, err := s.repo.BeliefIncoming(ctx, beliefID)
	if err != nil {
		return InspectResult{}, err
	}
	outgoing, err := s.repo.BeliefOutgoing(ctx, beliefID)
	if err != nil {
		return InspectResult{}, err
	}
	history, err := s.repo.StatusHistory(ctx, beliefID)
	if err != nil {
		return InspectResult{}, err
	}
	uses, err := s.repo.BeliefUses(ctx, beliefID, 256)
	if err != nil {
		return InspectResult{}, err
	}
	provenance, err := s.provenance(ctx, beliefID)
	if err != nil {
		return InspectResult{}, err
	}
	contradictions, err := s.contradictionBeliefs(ctx, beliefID)
	if err != nil {
		return InspectResult{}, err
	}
	return InspectResult{
		Belief: belief,
		Evidence: evidence,
		EvidenceLinks: links,
		Incoming: incoming,
		Outgoing: outgoing,
		History: history,
		Uses: uses,
		Provenance: provenance,
		Contradictions: contradictions,
	}, nil
}

func (s *Service) provenance(ctx context.Context, beliefID id.BeliefID) ([]ProvenanceClass, error) {
	visited := make(map[id.BeliefID]struct{})
	var values []ProvenanceClass
	var walk func(id.BeliefID, int) error
	walk = func(current id.BeliefID, depth int) error {
		if depth > s.cfg.ProvenanceDepth {
			return nil
		}
		if _, ok := visited[current]; ok {
			return nil
		}
		visited[current] = struct{}{}
		belief, err := s.repo.Belief(ctx, current)
		if err != nil {
			return err
		}
		values = append(values, belief.OriginProvenance)
		links, err := s.repo.BeliefEvidence(ctx, current)
		if err != nil {
			return err
		}
		for _, link := range links {
			evidence, err := s.repo.Evidence(ctx, link.EvidenceID)
			if err != nil {
				return err
			}
			values = append(values, evidence.Provenance)
		}
		incoming, err := s.repo.BeliefIncoming(ctx, current)
		if err != nil {
			return err
		}
		for _, edge := range incoming {
			switch edge.Relation {
			case BeliefDerivedFrom, BeliefRequires, BeliefSupports, BeliefRefines:
				if err := walk(edge.From, depth+1); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(beliefID, 0); err != nil {
		return nil, err
	}
	return uniqueProvenance(values), nil
}

func (s *Service) contradictionSummaries(ctx context.Context, beliefID id.BeliefID) ([]ContradictionSummary, error) {
	beliefs, err := s.contradictionBeliefs(ctx, beliefID)
	if err != nil {
		return nil, err
	}
	out := make([]ContradictionSummary, 0, len(beliefs))
	for _, belief := range beliefs {
		out = append(out, ContradictionSummary{BeliefID: belief.ID, Proposition: belief.Proposition, Status: belief.Status})
	}
	return out, nil
}

func (s *Service) contradictionBeliefs(ctx context.Context, beliefID id.BeliefID) ([]Belief, error) {
	edges, err := s.repo.BeliefOutgoing(ctx, beliefID)
	if err != nil {
		return nil, err
	}
	seen := make(map[id.BeliefID]struct{})
	out := make([]Belief, 0)
	for _, edge := range edges {
		if edge.Relation != BeliefContradicts {
			continue
		}
		if _, ok := seen[edge.To]; ok {
			continue
		}
		item, err := s.repo.Belief(ctx, edge.To)
		if err != nil {
			return nil, err
		}
		seen[edge.To] = struct{}{}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID.String() < out[j].ID.String() })
	return out, nil
}

func epistemicScore(belief Belief, lexical float64, scopes []Scope, invalidatedEvidence bool) float64 {
	status := 0.0
	switch belief.Status {
	case BeliefActive:
		status = 5
	case BeliefCandidate:
		status = 3.5
	case BeliefContested:
		status = 2.25
	case BeliefNeedsReview:
		status = 1.5
	case BeliefStale:
		status = 0.75
	case BeliefSuperseded, BeliefInvalidated, BeliefRejected:
		status = -2
	}
	freshness := 0.0
	switch belief.Confidence.Freshness {
	case FreshnessFresh:
		freshness = 3
	case FreshnessAging:
		freshness = 1.5
	case FreshnessUnknown:
		freshness = 0.75
	case FreshnessStale:
		freshness = 0.25
	}
	if invalidatedEvidence {
		freshness = 0
		if status > 0.75 {
			status = 0.75
		}
	}
	strength := 0.0
	switch belief.Confidence.EvidenceStrength {
	case StrengthRuntimeVerified:
		strength = 2
	case StrengthDirectAssertion:
		strength = 1.8
	case StrengthRepository:
		strength = 1.6
	case StrengthExternal:
		strength = 1
	case StrengthWeakSuggestion:
		strength = 0.5
	}
	scopeScore := 0.0
	if len(scopes) == 0 || matchesScopes(belief.Scope, scopes) {
		scopeScore = 1
	}
	conflictPenalty := 0.0
	if belief.Confidence.ConflictLevel == ConflictVerified || belief.Status == BeliefContested {
		conflictPenalty = 1.5
	} else if belief.Confidence.ConflictLevel == ConflictPossible {
		conflictPenalty = 0.5
	}
	if lexical < 0 {
		lexical = 0
	}
	if lexical > 1 {
		lexical = 1
	}
	return status + freshness + strength + lexical + scopeScore - conflictPenalty
}

func matchesScopes(scope Scope, allowed []Scope) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, candidate := range allowed {
		if scope.Matches(candidate) {
			return true
		}
	}
	return false
}

func containsStatus(values []BeliefStatus, want BeliefStatus) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
