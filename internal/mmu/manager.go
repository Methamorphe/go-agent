package mmu

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Methamorphe/go-agent/internal/clock"
	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
)

type recallTracker struct {
	key       string
	count     int
	updatedAt time.Time
}

type Manager struct {
	repo      Repository
	objects   ObjectStore
	ids       IDGenerator
	clock     clock.Clock
	cfg       Config
	estimator *cachedEstimator

	recallMu       sync.Mutex
	recallTrackers map[id.AgentID]recallTracker
}

func New(repo Repository, objects ObjectStore, ids IDGenerator, source clock.Clock, cfg Config) (*Manager, error) {
	return NewWithEstimator(repo, objects, ids, source, cfg, nil)
}

func NewWithEstimator(repo Repository, objects ObjectStore, ids IDGenerator, source clock.Clock, cfg Config, estimator TokenEstimator) (*Manager, error) {
	if repo == nil {
		return nil, errs.New(errs.CodeInvalidArgument, "mmu.new", "repository is required")
	}
	if objects == nil {
		return nil, errs.New(errs.CodeInvalidArgument, "mmu.new", "object store is required")
	}
	if ids == nil {
		return nil, errs.New(errs.CodeInvalidArgument, "mmu.new", "id generator is required")
	}
	if source == nil {
		source = clock.NewSystemClock()
	}
	if estimator == nil {
		estimator = ConservativeEstimator{}
	}
	cfg = normalizeConfig(cfg)

	return &Manager{
		repo:           repo,
		objects:        objects,
		ids:            ids,
		clock:          source,
		cfg:            cfg,
		estimator:      newCachedEstimator(estimator, cfg.MaxTokenCacheEntries),
		recallTrackers: make(map[id.AgentID]recallTracker),
	}, nil
}

func normalizeConfig(cfg Config) Config {
	defaults := DefaultConfig()
	if cfg.CandidateLimit <= 0 {
		cfg.CandidateLimit = defaults.CandidateLimit
	}
	if cfg.DefaultRecallLimit <= 0 {
		cfg.DefaultRecallLimit = defaults.DefaultRecallLimit
	}
	if cfg.DefaultRecallMaxTokens <= 0 {
		cfg.DefaultRecallMaxTokens = defaults.DefaultRecallMaxTokens
	}
	if cfg.MaxPageTokens <= 0 {
		cfg.MaxPageTokens = defaults.MaxPageTokens
	}
	if cfg.MaxMaterializeBytes <= 0 {
		cfg.MaxMaterializeBytes = defaults.MaxMaterializeBytes
	}
	if cfg.MaxIndexTextBytes <= 0 {
		cfg.MaxIndexTextBytes = defaults.MaxIndexTextBytes
	}
	if cfg.MaxTokenCacheEntries <= 0 {
		cfg.MaxTokenCacheEntries = defaults.MaxTokenCacheEntries
	}
	if cfg.RecallLoopThreshold <= 0 {
		cfg.RecallLoopThreshold = defaults.RecallLoopThreshold
	}
	if cfg.MaxRecallTrackers <= 0 {
		cfg.MaxRecallTrackers = defaults.MaxRecallTrackers
	}
	if cfg.Weights == (RankingWeights{}) {
		cfg.Weights = defaults.Weights
	}
	return cfg
}

func (m *Manager) EstimateText(model, text string) int {
	return m.estimator.EstimateText(model, text)
}

func (m *Manager) EstimateRequestOverhead(model string, shape RequestShape) int {
	return m.estimator.EstimateRequestOverhead(model, shape)
}

func (m *Manager) CreatePage(ctx context.Context, input PageInput) (PageMeta, error) {
	if input.AgentID == "" {
		return PageMeta{}, errs.New(errs.CodeInvalidArgument, "mmu.create_page", "agent id is required")
	}
	if !validPageType(input.Type) {
		return PageMeta{}, errs.New(errs.CodeInvalidArgument, "mmu.create_page", "invalid page type")
	}
	if !validScope(input.Scope) {
		return PageMeta{}, errs.New(errs.CodeInvalidArgument, "mmu.create_page", "invalid page scope")
	}
	if strings.TrimSpace(input.Content) == "" {
		return PageMeta{}, errs.New(errs.CodeInvalidArgument, "mmu.create_page", "page content is empty")
	}
	if input.Importance < 0 || input.Importance > 1 || input.Confidence < 0 || input.Confidence > 1 {
		return PageMeta{}, errs.New(errs.CodeInvalidArgument, "mmu.create_page", "importance and confidence must be between 0 and 1")
	}

	tokens := m.estimator.EstimateText("generic", input.Content)
	if tokens > m.cfg.MaxPageTokens {
		return PageMeta{}, errs.New(
			errs.CodeResourceExhausted,
			"mmu.create_page",
			fmt.Sprintf("page estimate %d exceeds normal page ceiling %d; segment or summarize it", tokens, m.cfg.MaxPageTokens),
		)
	}

	object, err := m.objects.Put(ctx, bytes.NewReader([]byte(input.Content)))
	if err != nil {
		return PageMeta{}, err
	}
	pageID, err := m.ids.ContextPage()
	if err != nil {
		return PageMeta{}, errs.Wrap(errs.CodeInternal, "mmu.create_page", "generate page id", err)
	}
	now := m.clock.Now().UTC()
	meta := PageMeta{
		ID:             pageID,
		AgentID:        input.AgentID,
		Type:           input.Type,
		Scope:          input.Scope,
		SourceRef:      input.SourceRef,
		ObjectRef:      object.Ref,
		TokenEstimate:  tokens,
		Importance:     input.Importance,
		Confidence:     input.Confidence,
		CreatedAt:      now,
		LastAccessedAt: now,
		PinnedUntil:    input.PinnedUntil,
		SummaryOf:      append([]id.ContextPageID(nil), input.SummaryOf...),
	}
	searchText := boundedUTF8(input.SourceRef+"\n"+input.Content, m.cfg.MaxIndexTextBytes)
	if err := m.repo.PutContextPage(ctx, meta, searchText); err != nil {
		return PageMeta{}, err
	}
	return meta, nil
}

func (m *Manager) Supersede(ctx context.Context, agentID id.AgentID, oldID, newID id.ContextPageID) error {
	if agentID == "" || oldID == "" || newID == "" {
		return errs.New(errs.CodeInvalidArgument, "mmu.supersede", "agent id and both page ids are required")
	}
	if oldID == newID {
		return errs.New(errs.CodeInvalidArgument, "mmu.supersede", "a page cannot supersede itself")
	}
	oldMeta, err := m.repo.ContextPage(ctx, agentID, oldID)
	if err != nil {
		return err
	}
	if _, err := m.repo.ContextPage(ctx, agentID, newID); err != nil {
		return err
	}
	if oldMeta.SupersededBy != nil {
		return errs.New(errs.CodeConflict, "mmu.supersede", "page is already superseded")
	}
	return m.repo.MarkContextPageSuperseded(ctx, agentID, oldID, newID)
}

func (m *Manager) Recall(ctx context.Context, request RecallRequest) (RecallResult, error) {
	if request.AgentID == "" {
		return RecallResult{}, errs.New(errs.CodeInvalidArgument, "mmu.recall", "agent id is required")
	}

	query := request.Query
	if query.Limit <= 0 {
		query.Limit = m.cfg.DefaultRecallLimit
	}
	if query.Limit > m.cfg.CandidateLimit {
		query.Limit = m.cfg.CandidateLimit
	}
	if query.MaxTokens <= 0 {
		query.MaxTokens = m.cfg.DefaultRecallMaxTokens
	}

	scopes := effectiveScopes(request.AllowedScopes, query.Scopes)
	if len(scopes) == 0 {
		return RecallResult{}, errs.New(errs.CodePermissionDenied, "mmu.recall", "no visible context scope")
	}

	type ranked struct {
		meta       PageMeta
		score      float64
		components RankingComponents
		reason     string
	}

	seen := make(map[id.ContextPageID]struct{})
	rankedPages := make([]ranked, 0, len(query.ExplicitIDs)+m.cfg.CandidateLimit)
	unresolved := make([]id.ContextPageID, 0)
	now := m.clock.Now().UTC()

	for _, pageID := range query.ExplicitIDs {
		if _, ok := seen[pageID]; ok {
			continue
		}
		meta, err := m.repo.ContextPage(ctx, request.AgentID, pageID)
		if err != nil {
			if errs.IsCode(err, errs.CodeNotFound) || errs.IsCode(err, errs.CodePermissionDenied) {
				unresolved = append(unresolved, pageID)
				continue
			}
			return RecallResult{}, err
		}
		if !scopeAllowed(meta.Scope, scopes) || meta.SupersededBy != nil {
			unresolved = append(unresolved, pageID)
			continue
		}
		components := m.rank(meta, 1, 1, now)
		rankedPages = append(rankedPages, ranked{
			meta:       meta,
			score:      m.weightedScore(components),
			components: components,
			reason:     "explicit_reference",
		})
		seen[pageID] = struct{}{}
	}

	if strings.TrimSpace(query.Text) != "" {
		candidates, err := m.repo.SearchContextPages(ctx, CandidateQuery{
			AgentID: request.AgentID,
			Text:    query.Text,
			Scopes:  scopes,
			Types:   query.Types,
			Limit:   m.cfg.CandidateLimit,
		})
		if err != nil {
			return RecallResult{}, err
		}
		for _, candidate := range candidates {
			if _, ok := seen[candidate.Meta.ID]; ok {
				continue
			}
			if !scopeAllowed(candidate.Meta.Scope, scopes) || candidate.Meta.SupersededBy != nil || candidate.Meta.CompactedBy != nil {
				continue
			}
			components := m.rank(candidate.Meta, candidate.LexicalScore, 0, now)
			rankedPages = append(rankedPages, ranked{
				meta:       candidate.Meta,
				score:      m.weightedScore(components),
				components: components,
				reason:     "deterministic_rank",
			})
			seen[candidate.Meta.ID] = struct{}{}
		}
	}

	sort.SliceStable(rankedPages, func(i, j int) bool {
		if rankedPages[i].score != rankedPages[j].score {
			return rankedPages[i].score > rankedPages[j].score
		}
		if !rankedPages[i].meta.CreatedAt.Equal(rankedPages[j].meta.CreatedAt) {
			return rankedPages[i].meta.CreatedAt.After(rankedPages[j].meta.CreatedAt)
		}
		return rankedPages[i].meta.ID < rankedPages[j].meta.ID
	})

	result := RecallResult{Unresolved: unresolved}
	for _, page := range rankedPages {
		if len(result.Selected) >= query.Limit {
			break
		}
		if page.meta.TokenEstimate > query.MaxTokens-result.BudgetUsed {
			continue
		}
		result.Selected = append(result.Selected, RecallSelection{
			PageID:     page.meta.ID,
			Score:      page.score,
			Components: page.components,
			Reason:     page.reason,
			Tokens:     page.meta.TokenEstimate,
		})
		result.BudgetUsed += page.meta.TokenEstimate
	}

	if err := m.trackRecall(request.AgentID, query, result, now); err != nil {
		return RecallResult{}, err
	}

	for _, selection := range result.Selected {
		leaseID, err := m.ids.Lease()
		if err != nil {
			return RecallResult{}, errs.Wrap(errs.CodeInternal, "mmu.recall", "generate lease id", err)
		}
		if err := m.repo.PutContextLease(ctx, ContextLease{
			ID:              leaseID,
			PageID:          selection.PageID,
			AgentID:         request.AgentID,
			Reason:          "explicit_recall",
			RemainingBuilds: 1,
			CreatedAt:       now,
		}); err != nil {
			return RecallResult{}, err
		}
	}
	return result, nil
}

func (m *Manager) Build(ctx context.Context, request BuildRequest) (WorkingSet, error) {
	if request.AgentID == "" {
		return WorkingSet{}, errs.New(errs.CodeInvalidArgument, "mmu.build", "agent id is required")
	}
	request.Budget = normalizeBudget(request.Budget)
	inputBudget := request.Budget.InputBudget()
	fixedTokens := request.Budget.SchemaTokens + request.Budget.ActiveTokens
	if inputBudget <= 0 || fixedTokens > inputBudget {
		return WorkingSet{}, errs.Wrap(errs.CodeResourceExhausted, "mmu.build", "fixed context exceeds input budget", ErrContextBudgetImpossible)
	}
	if len(request.AllowedScopes) == 0 {
		return WorkingSet{}, errs.New(errs.CodePermissionDenied, "mmu.build", "no visible context scope")
	}

	type selected struct {
		meta       PageMeta
		tier       int
		score      float64
		components RankingComponents
		reason     string
	}

	selectedPages := make([]selected, 0, len(request.MandatoryIDs)+len(request.ActiveIDs)+m.cfg.CandidateLimit)
	selectedIDs := make(map[id.ContextPageID]struct{})
	excluded := make([]ManifestExclusion, 0)
	estimatedPages := 0
	now := m.clock.Now().UTC()

	add := func(meta PageMeta, tier int, score float64, components RankingComponents, reason string, mandatory bool) error {
		if _, ok := selectedIDs[meta.ID]; ok {
			return nil
		}
		if !scopeAllowed(meta.Scope, request.AllowedScopes) {
			if mandatory {
				return errs.New(errs.CodePermissionDenied, "mmu.build", "mandatory page is outside visible scope")
			}
			excluded = append(excluded, ManifestExclusion{ID: meta.ID, Reason: "scope_denied"})
			return nil
		}
		if meta.SupersededBy != nil {
			if mandatory {
				return errs.New(errs.CodeConflict, "mmu.build", "mandatory page is superseded")
			}
			excluded = append(excluded, ManifestExclusion{ID: meta.ID, Reason: "superseded"})
			return nil
		}
		if !mandatory && estimatedPages+meta.TokenEstimate > inputBudget-fixedTokens {
			excluded = append(excluded, ManifestExclusion{ID: meta.ID, Reason: "budget_or_low_rank"})
			return nil
		}
		if mandatory && estimatedPages+meta.TokenEstimate > inputBudget-fixedTokens {
			return errs.Wrap(errs.CodeResourceExhausted, "mmu.build", "mandatory pages exceed input budget", ErrContextBudgetImpossible)
		}
		selectedPages = append(selectedPages, selected{meta: meta, tier: tier, score: score, components: components, reason: reason})
		selectedIDs[meta.ID] = struct{}{}
		estimatedPages += meta.TokenEstimate
		return nil
	}

	for _, pageID := range request.MandatoryIDs {
		meta, err := m.repo.ContextPage(ctx, request.AgentID, pageID)
		if err != nil {
			return WorkingSet{}, err
		}
		if err := add(meta, 0, 0, RankingComponents{}, "mandatory", true); err != nil {
			return WorkingSet{}, err
		}
	}

	pinned, err := m.repo.PinnedContextPages(ctx, request.AgentID, request.AllowedScopes, now)
	if err != nil {
		return WorkingSet{}, err
	}
	for _, meta := range pinned {
		if meta.SupersededBy != nil {
			excluded = append(excluded, ManifestExclusion{ID: meta.ID, Reason: "superseded_pin"})
			continue
		}
		if err := add(meta, 0, 0, RankingComponents{}, "pinned_until", true); err != nil {
			return WorkingSet{}, err
		}
	}

	for _, pageID := range request.ActiveIDs {
		meta, err := m.repo.ContextPage(ctx, request.AgentID, pageID)
		if err != nil {
			if errs.IsCode(err, errs.CodeNotFound) {
				excluded = append(excluded, ManifestExclusion{ID: pageID, Reason: "active_page_missing"})
				continue
			}
			return WorkingSet{}, err
		}
		if err := add(meta, 1, 0, RankingComponents{}, "active_state", false); err != nil {
			return WorkingSet{}, err
		}
	}

	leases, err := m.repo.ActiveContextLeases(ctx, request.AgentID, now)
	if err != nil {
		return WorkingSet{}, err
	}
	leasedSelected := make([]id.ContextPageID, 0, len(leases))
	for _, lease := range leases {
		meta, err := m.repo.ContextPage(ctx, request.AgentID, lease.PageID)
		if err != nil {
			if errs.IsCode(err, errs.CodeNotFound) {
				continue
			}
			return WorkingSet{}, err
		}
		tier := 2
		mandatory := lease.HardPin
		if mandatory {
			tier = 0
		}
		before := len(selectedPages)
		if err := add(meta, tier, m.cfg.Weights.Explicit, RankingComponents{Explicit: 1}, "context_lease", mandatory); err != nil {
			return WorkingSet{}, err
		}
		if len(selectedPages) > before {
			leasedSelected = append(leasedSelected, lease.PageID)
		}
	}

	candidates, err := m.repo.SearchContextPages(ctx, CandidateQuery{
		AgentID: request.AgentID,
		Text:    request.Query,
		Scopes:  request.AllowedScopes,
		Limit:   m.cfg.CandidateLimit,
	})
	if err != nil {
		return WorkingSet{}, err
	}
	type rankedCandidate struct {
		candidate  Candidate
		score      float64
		components RankingComponents
	}
	rankedCandidates := make([]rankedCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if _, ok := selectedIDs[candidate.Meta.ID]; ok {
			continue
		}
		if candidate.Meta.SupersededBy != nil {
			excluded = append(excluded, ManifestExclusion{ID: candidate.Meta.ID, Reason: "superseded"})
			continue
		}
		if candidate.Meta.CompactedBy != nil {
			excluded = append(excluded, ManifestExclusion{ID: candidate.Meta.ID, Reason: "compacted_cold"})
			continue
		}
		components := m.rank(candidate.Meta, candidate.LexicalScore, 0, now)
		rankedCandidates = append(rankedCandidates, rankedCandidate{
			candidate:  candidate,
			score:      m.weightedScore(components),
			components: components,
		})
	}
	sort.SliceStable(rankedCandidates, func(i, j int) bool {
		if rankedCandidates[i].score != rankedCandidates[j].score {
			return rankedCandidates[i].score > rankedCandidates[j].score
		}
		if !rankedCandidates[i].candidate.Meta.CreatedAt.Equal(rankedCandidates[j].candidate.Meta.CreatedAt) {
			return rankedCandidates[i].candidate.Meta.CreatedAt.After(rankedCandidates[j].candidate.Meta.CreatedAt)
		}
		return rankedCandidates[i].candidate.Meta.ID < rankedCandidates[j].candidate.Meta.ID
	})

	// Preserve ranking while preventing one noisy source or one page type from
	// monopolizing the first-pass packing. Deferred pages remain eligible if
	// budget is still available.
	diverse := make([]rankedCandidate, 0, len(rankedCandidates))
	deferred := make([]rankedCandidate, 0)
	sourceCount := make(map[string]int)
	typeCount := make(map[PageType]int)
	for _, candidate := range rankedCandidates {
		source := candidate.candidate.Meta.SourceRef
		pageType := candidate.candidate.Meta.Type
		if (source != "" && sourceCount[source] >= 2) || typeCount[pageType] >= 4 {
			deferred = append(deferred, candidate)
			continue
		}
		diverse = append(diverse, candidate)
		if source != "" {
			sourceCount[source]++
		}
		typeCount[pageType]++
	}
	diverse = append(diverse, deferred...)
	for _, candidate := range diverse {
		if err := add(candidate.candidate.Meta, 3, candidate.score, candidate.components, "deterministic_rank", false); err != nil {
			return WorkingSet{}, err
		}
	}

	// Tier 4 preserves a small amount of recent continuity when relevance did
	// not consume the whole budget. It is deliberately attempted after all
	// stronger tiers and is never a correctness dependency.
	recent, err := m.repo.SearchContextPages(ctx, CandidateQuery{
		AgentID: request.AgentID,
		Scopes:  request.AllowedScopes,
		Limit:   m.cfg.CandidateLimit,
	})
	if err != nil {
		return WorkingSet{}, err
	}
	for _, candidate := range recent {
		if _, ok := selectedIDs[candidate.Meta.ID]; ok {
			continue
		}
		if candidate.Meta.SupersededBy != nil || candidate.Meta.CompactedBy != nil {
			continue
		}
		components := m.rank(candidate.Meta, 0, 0, now)
		if err := add(candidate.Meta, 4, m.weightedScore(components), components, "recent_continuity", false); err != nil {
			return WorkingSet{}, err
		}
	}

	materialized := make([]MaterializedPage, 0, len(selectedPages))
	actualPageTokens := 0
	for _, page := range selectedPages {
		content, tokens, err := m.materialize(ctx, request.Model, page.meta)
		if err != nil {
			return WorkingSet{}, err
		}
		materialized = append(materialized, MaterializedPage{
			Meta:    page.meta,
			Tier:    page.tier,
			Score:   page.score,
			Reason:  page.reason,
			Content: content,
			Tokens:  tokens,
		})
		actualPageTokens += tokens
	}

	for fixedTokens+actualPageTokens > inputBudget {
		evict := -1
		for i := len(materialized) - 1; i >= 0; i-- {
			if materialized[i].Tier > 0 {
				evict = i
				break
			}
		}
		if evict < 0 {
			return WorkingSet{}, errs.Wrap(errs.CodeResourceExhausted, "mmu.build", "mandatory pages exceed input budget after conservative re-estimate", ErrContextBudgetImpossible)
		}
		excluded = append(excluded, ManifestExclusion{ID: materialized[evict].Meta.ID, Reason: "final_reestimate_budget"})
		actualPageTokens -= materialized[evict].Tokens
		materialized = append(materialized[:evict], materialized[evict+1:]...)
	}

	pageIDs := make([]id.ContextPageID, 0, len(materialized))
	manifestPages := make([]ManifestPage, 0, len(materialized))
	included := make(map[id.ContextPageID]struct{}, len(materialized))
	for _, page := range materialized {
		pageIDs = append(pageIDs, page.Meta.ID)
		included[page.Meta.ID] = struct{}{}
		components := RankingComponents{}
		for _, raw := range selectedPages {
			if raw.meta.ID == page.Meta.ID {
				components = raw.components
				break
			}
		}
		manifestPages = append(manifestPages, ManifestPage{
			ID:         page.Meta.ID,
			Tier:       page.Tier,
			Reason:     page.Reason,
			Tokens:     page.Tokens,
			Score:      page.Score,
			Components: components,
		})
	}
	if len(pageIDs) > 0 {
		if err := m.repo.TouchContextPages(ctx, request.AgentID, pageIDs, now); err != nil {
			return WorkingSet{}, err
		}
	}
	consumed := make([]id.ContextPageID, 0, len(leasedSelected))
	for _, pageID := range leasedSelected {
		if _, ok := included[pageID]; ok {
			consumed = append(consumed, pageID)
		}
	}
	if len(consumed) > 0 {
		if err := m.repo.ConsumeContextLeases(ctx, request.AgentID, consumed); err != nil {
			return WorkingSet{}, err
		}
	}

	return WorkingSet{
		Pages: materialized,
		Manifest: ContextManifest{
			AgentID:        request.AgentID,
			Model:          request.Model,
			InputBudget:    inputBudget,
			EstimatedUsed:  fixedTokens + actualPageTokens,
			ReservedOutput: request.Budget.ReservedOutputTokens,
			SchemaTokens:   request.Budget.SchemaTokens,
			ActiveTokens:   request.Budget.ActiveTokens,
			Pages:          manifestPages,
			Excluded:       excluded,
		},
	}, nil
}

func (m *Manager) PersistManifest(ctx context.Context, invocationID id.InvocationID, manifest ContextManifest) (string, error) {
	if invocationID == "" {
		return "", errs.New(errs.CodeInvalidArgument, "mmu.persist_manifest", "invocation id is required")
	}
	manifest.InvocationID = invocationID
	body, err := json.Marshal(manifest)
	if err != nil {
		return "", errs.Wrap(errs.CodeInternal, "mmu.persist_manifest", "encode manifest", err)
	}
	object, err := m.objects.Put(ctx, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	if err := m.repo.PutContextManifest(ctx, invocationID, manifest.AgentID, object.Ref, m.clock.Now().UTC()); err != nil {
		return "", err
	}
	return string(object.Ref), nil
}

func (m *Manager) Compact(ctx context.Context, request CompactRequest) (PageMeta, error) {
	if request.AgentID == "" || len(request.SourceIDs) == 0 {
		return PageMeta{}, errs.New(errs.CodeInvalidArgument, "mmu.compact", "agent id and source pages are required")
	}
	if strings.TrimSpace(request.Summary) == "" {
		return PageMeta{}, errs.New(errs.CodeInvalidArgument, "mmu.compact", "summary is required")
	}

	importance := 0.0
	confidence := 1.0
	var inheritedScope Scope
	for i, pageID := range request.SourceIDs {
		meta, err := m.repo.ContextPage(ctx, request.AgentID, pageID)
		if err != nil {
			return PageMeta{}, err
		}
		if meta.SupersededBy != nil {
			return PageMeta{}, errs.New(errs.CodeConflict, "mmu.compact", "cannot compact superseded source page")
		}
		if i == 0 {
			inheritedScope = meta.Scope
		}
		if meta.Importance > importance {
			importance = meta.Importance
		}
		if meta.Confidence < confidence {
			confidence = meta.Confidence
		}
	}
	if request.Type == "" {
		request.Type = PageAssistantSummary
	}
	if request.Scope == "" {
		request.Scope = inheritedScope
	}
	if request.SourceRef == "" {
		request.SourceRef = "compaction"
	}

	summary, err := m.CreatePage(ctx, PageInput{
		AgentID:    request.AgentID,
		Type:       request.Type,
		Scope:      request.Scope,
		SourceRef:  request.SourceRef,
		Content:    request.Summary,
		Importance: importance,
		Confidence: confidence,
		SummaryOf:  append([]id.ContextPageID(nil), request.SourceIDs...),
	})
	if err != nil {
		return PageMeta{}, err
	}
	if err := m.repo.MarkContextPagesCompacted(ctx, request.AgentID, request.SourceIDs, summary.ID); err != nil {
		return PageMeta{}, err
	}
	return summary, nil
}

func (m *Manager) materialize(ctx context.Context, model string, meta PageMeta) (string, int, error) {
	reader, err := m.objects.Open(meta.ObjectRef)
	if err != nil {
		return "", 0, err
	}
	defer reader.Close()

	limited := io.LimitReader(reader, m.cfg.MaxMaterializeBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return "", 0, errs.Wrap(errs.CodeUnavailable, "mmu.materialize", "read page object", err)
	}
	if int64(len(body)) > m.cfg.MaxMaterializeBytes {
		return "", 0, errs.New(errs.CodeResourceExhausted, "mmu.materialize", "page representation exceeds materialization bound")
	}
	content := string(body)
	return content, m.estimator.EstimateText(model, content), nil
}

func (m *Manager) rank(meta PageMeta, lexical, explicit float64, now time.Time) RankingComponents {
	hours := now.Sub(meta.LastAccessedAt).Hours()
	if hours < 0 {
		hours = 0
	}
	return RankingComponents{
		Explicit:   clamp01(explicit),
		Task:       clamp01(lexical),
		Recency:    1 / (1 + hours/24),
		Importance: clamp01(meta.Importance),
		Confidence: clamp01(meta.Confidence),
		Type:       pageTypePriority(meta.Type),
	}
}

func (m *Manager) weightedScore(c RankingComponents) float64 {
	w := m.cfg.Weights
	return w.Explicit*c.Explicit + w.Task*c.Task + w.Recency*c.Recency + w.Importance*c.Importance + w.Confidence*c.Confidence + w.Type*c.Type
}

func (m *Manager) trackRecall(agentID id.AgentID, query RecallQuery, result RecallResult, now time.Time) error {
	key := recallFingerprint(query, result)
	m.recallMu.Lock()
	defer m.recallMu.Unlock()

	tracker := m.recallTrackers[agentID]
	if tracker.key == key {
		tracker.count++
	} else {
		tracker = recallTracker{key: key, count: 1}
	}
	tracker.updatedAt = now
	m.recallTrackers[agentID] = tracker

	if len(m.recallTrackers) > m.cfg.MaxRecallTrackers {
		var oldestID id.AgentID
		var oldest time.Time
		for candidateID, candidate := range m.recallTrackers {
			if candidateID == agentID {
				continue
			}
			if oldestID == "" || candidate.updatedAt.Before(oldest) {
				oldestID = candidateID
				oldest = candidate.updatedAt
			}
		}
		if oldestID != "" {
			delete(m.recallTrackers, oldestID)
		}
	}

	if tracker.count >= m.cfg.RecallLoopThreshold {
		return errs.Wrap(errs.CodeResourceExhausted, "mmu.recall", "same recall query/page set repeated without progress", ErrRecallLoopDetected)
	}
	return nil
}

func recallFingerprint(query RecallQuery, result RecallResult) string {
	var builder strings.Builder
	builder.WriteString(strings.ToLower(strings.Join(strings.Fields(query.Text), " ")))
	builder.WriteByte('|')
	for _, scope := range query.Scopes {
		builder.WriteString(string(scope))
		builder.WriteByte(',')
	}
	builder.WriteByte('|')
	for _, pageType := range query.Types {
		builder.WriteString(string(pageType))
		builder.WriteByte(',')
	}
	builder.WriteByte('|')
	for _, pageID := range query.ExplicitIDs {
		builder.WriteString(pageID.String())
		builder.WriteByte(',')
	}
	builder.WriteByte('|')
	for _, selection := range result.Selected {
		builder.WriteString(selection.PageID.String())
		builder.WriteByte(',')
	}
	sum := sha256.Sum256([]byte(builder.String()))
	return hex.EncodeToString(sum[:])
}

func effectiveScopes(allowed, requested []Scope) []Scope {
	if len(allowed) == 0 {
		return nil
	}
	if len(requested) == 0 {
		return append([]Scope(nil), allowed...)
	}
	result := make([]Scope, 0, len(requested))
	for _, scope := range requested {
		if scopeAllowed(scope, allowed) {
			result = append(result, scope)
		}
	}
	return result
}

func scopeAllowed(scope Scope, allowed []Scope) bool {
	for _, candidate := range allowed {
		if candidate == scope {
			return true
		}
	}
	return false
}

func normalizeBudget(b Budget) Budget {
	if b.ModelMaxContext <= 0 {
		b.ModelMaxContext = DefaultModelMaxContext
	}
	if b.ReservedOutputTokens <= 0 {
		b.ReservedOutputTokens = DefaultReservedOutputTokens
	}
	if b.ProviderOverhead <= 0 {
		b.ProviderOverhead = DefaultProviderOverhead
	}
	if b.SafetyMargin <= 0 {
		b.SafetyMargin = DefaultSafetyMargin
	}
	return b
}

func validScope(scope Scope) bool {
	switch scope {
	case ScopeProcess, ScopeRootTask, ScopeProject, ScopeWorkspace, ScopeUser, ScopeWorld:
		return true
	default:
		return false
	}
}

func validPageType(pageType PageType) bool {
	switch pageType {
	case PageUserMessage, PageAssistantSummary, PageSourceExcerpt, PageFileSummary, PageToolResultSummary, PageDecision, PagePlan, PageChildReport, PageErrorTrace, PageDocumentation, PageRecallResult:
		return true
	default:
		return false
	}
}

func pageTypePriority(pageType PageType) float64 {
	switch pageType {
	case PageDecision:
		return 1
	case PagePlan:
		return 0.95
	case PageErrorTrace:
		return 0.9
	case PageChildReport:
		return 0.85
	case PageSourceExcerpt, PageFileSummary:
		return 0.8
	case PageUserMessage:
		return 0.75
	case PageToolResultSummary:
		return 0.7
	case PageAssistantSummary:
		return 0.65
	case PageDocumentation:
		return 0.6
	case PageRecallResult:
		return 0.5
	default:
		return 0
	}
}

func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func boundedUTF8(value string, maxBytes int) string {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value
	}
	value = value[:maxBytes]
	for !utf8.ValidString(value) && len(value) > 0 {
		value = value[:len(value)-1]
	}
	return value
}
