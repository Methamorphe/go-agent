package mmu

import (
	"context"

	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
)

func (m *Manager) planFault(ctx context.Context, request ContextFaultRequest) (FaultResolutionPlan, error) {
	switch request.Kind {
	case FaultRecall:
		result, err := m.Recall(ctx, RecallRequest{AgentID: request.AgentID, AllowedScopes: request.AllowedScopes, Query: *request.Query})
		if err != nil { return FaultResolutionPlan{}, err }
		pages := make([]id.ContextPageID, 0, len(result.Selected))
		for _, selection := range result.Selected { pages = append(pages, selection.PageID) }
		return FaultResolutionPlan{PageIDs: pages}, nil
	case FaultDependency:
		refs := append([]CognitiveRef(nil), request.RequiredBy...)
		if request.Reference != nil { refs = append(refs, *request.Reference) }
		return m.resolveRefs(ctx, request, refs)
	case FaultEvidence:
		if request.Reference != nil && request.Reference.Scheme() == RefContext {
			meta, err := m.repo.ContextPage(ctx, request.AgentID, id.ContextPageID(request.Reference.Key()))
			if err != nil { return FaultResolutionPlan{}, err }
			if len(meta.SummaryOf) > 0 {
				refs := make([]CognitiveRef, 0, len(meta.SummaryOf))
				for _, pageID := range meta.SummaryOf { refs = append(refs, CognitiveRef("ctx://"+pageID.String())) }
				return FaultResolutionPlan{PageIDs: append([]id.ContextPageID(nil), meta.SummaryOf...), ResolvedRefs: refs}, nil
			}
		}
		fallthrough
	case FaultReference, FaultFreshness, FaultRepresentation:
		return m.resolveRefs(ctx, request, []CognitiveRef{*request.Reference})
	default:
		return FaultResolutionPlan{}, errs.New(errs.CodeUnsupported, "mmu.fault.resolve", "unsupported fault kind")
	}
}

func (m *Manager) resolveRefs(ctx context.Context, request ContextFaultRequest, refs []CognitiveRef) (FaultResolutionPlan, error) {
	plan := FaultResolutionPlan{}
	queue := append([]CognitiveRef(nil), refs...)
	seen := make(map[CognitiveRef]struct{})
	maxNodes := m.cfg.CandidateLimit
	if maxNodes <= 0 { maxNodes = DefaultConfig().CandidateLimit }
	for len(queue) > 0 {
		if len(seen) >= maxNodes { return FaultResolutionPlan{}, errs.New(errs.CodeResourceExhausted, "mmu.fault.dependencies", "dependency paging bound exceeded") }
		ref := queue[0]; queue = queue[1:]
		if _, ok := seen[ref]; ok { continue }; seen[ref] = struct{}{}
		if _, err := ParseCognitiveRef(ref.String()); err != nil { return FaultResolutionPlan{}, err }
		if ref.Scheme() == RefContext {
			meta, err := m.currentPage(ctx, request.AgentID, id.ContextPageID(ref.Key()), request.Kind == FaultFreshness)
			if err != nil { return FaultResolutionPlan{}, err }
			plan.PageIDs = append(plan.PageIDs, meta.ID)
			plan.ResolvedRefs = append(plan.ResolvedRefs, CognitiveRef("ctx://"+meta.ID.String()))
			continue
		}
		rt := faultRuntimeFor(m); rt.mu.Lock(); resolver := rt.resolvers[ref.Scheme()]; rt.mu.Unlock()
		if resolver == nil { return FaultResolutionPlan{}, errs.New(errs.CodeNotFound, "mmu.fault.resolve", "no resolver registered for cognitive reference scheme") }
		resolved, err := resolver.ResolveCognitiveRef(ctx, request, ref); if err != nil { return FaultResolutionPlan{}, err }
		plan.PageIDs = append(plan.PageIDs, resolved.PageIDs...)
		plan.ResolvedRefs = append(plan.ResolvedRefs, resolved.ResolvedRefs...)
		queue = append(queue, resolved.Dependencies...)
	}
	return plan, nil
}

func (m *Manager) currentPage(ctx context.Context, agentID id.AgentID, pageID id.ContextPageID, follow bool) (PageMeta, error) {
	seen := make(map[id.ContextPageID]struct{})
	for {
		if _, ok := seen[pageID]; ok { return PageMeta{}, errs.New(errs.CodeCorruption, "mmu.fault.freshness", "supersession cycle detected") }
		seen[pageID] = struct{}{}
		meta, err := m.repo.ContextPage(ctx, agentID, pageID); if err != nil { return PageMeta{}, err }
		if !follow || meta.SupersededBy == nil { return meta, nil }
		pageID = *meta.SupersededBy
	}
}

func (m *Manager) grantFaultLease(ctx context.Context, request ContextFaultRequest, meta PageMeta) (id.LeaseID, error) {
	leaseID, err := m.ids.Lease(); if err != nil { return "", errs.Wrap(errs.CodeInternal, "mmu.fault.lease", "generate lease id", err) }
	if err := m.repo.PutContextLease(ctx, ContextLease{ID: leaseID, PageID: meta.ID, AgentID: request.AgentID, Reason: "context_fault:"+request.ID.String(), RemainingBuilds: 1, CreatedAt: m.clock.Now().UTC()}); err != nil { return "", err }
	return leaseID, nil
}

func uniquePageIDs(values []id.ContextPageID) []id.ContextPageID {
	seen := make(map[id.ContextPageID]struct{}, len(values)); out := make([]id.ContextPageID, 0, len(values))
	for _, value := range values { if value != "" { if _, ok := seen[value]; !ok { seen[value] = struct{}{}; out = append(out, value) } } }
	return out
}
func uniqueRefs(values []CognitiveRef) []CognitiveRef {
	seen := make(map[CognitiveRef]struct{}, len(values)); out := make([]CognitiveRef, 0, len(values))
	for _, value := range values { if value != "" { if _, ok := seen[value]; !ok { seen[value] = struct{}{}; out = append(out, value) } } }
	return out
}
