package mmu

import (
	"context"
	"fmt"
	"strings"

	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
)

func (m *Manager) ResolveFault(ctx context.Context, request ContextFaultRequest) (FaultResolution, error) {
	if request.AgentID == "" || request.InvocationID == "" { return FaultResolution{}, errs.New(errs.CodeInvalidArgument, "mmu.fault.resolve", "agent and invocation ids are required") }
	if !validFaultKind(request.Kind) { return FaultResolution{}, errs.New(errs.CodeInvalidArgument, "mmu.fault.resolve", "invalid fault kind") }
	if request.Purpose == "" { request.Purpose = PurposeContinueReasoning }
	if request.Urgency == "" { request.Urgency = UrgencyNormal }
	if len(request.AllowedScopes) == 0 { return m.failFault(ctx, request, FaultDenied, "no visible context scope", errs.CodePermissionDenied) }
	if request.ID == "" {
		leaseID, err := m.ids.Lease(); if err != nil { return FaultResolution{}, errs.Wrap(errs.CodeInternal, "mmu.fault.resolve", "generate fault id", err) }
		request.ID = id.ContextFaultID("flt_" + strings.TrimPrefix(leaseID.String(), "lse_"))
	}
	if err := validateFaultShape(request); err != nil { return FaultResolution{}, err }
	now := m.clock.Now().UTC()
	m.journalFault(ctx, FaultRecord{Request: request, State: FaultDetected, CreatedAt: now, UpdatedAt: now})
	budget := normalizeFaultBudget(m.cfg.FaultBudget)
	if err := m.reserveFault(request, budget, now); err != nil {
		m.journalFault(ctx, FaultRecord{Request: request, State: FaultBudgetExceeded, Reason: err.Error(), CreatedAt: now, UpdatedAt: now})
		return FaultResolution{FaultID: request.ID, State: FaultBudgetExceeded, Reason: err.Error()}, err
	}
	resolveCtx := ctx
	if budget.MaxResolutionTime > 0 { var cancel context.CancelFunc; resolveCtx, cancel = context.WithTimeout(ctx, budget.MaxResolutionTime); defer cancel() }
	plan, err := m.planFault(resolveCtx, request)
	if err != nil {
		state := FaultUnresolved; if errs.IsCode(err, errs.CodePermissionDenied) { state = FaultDenied }
		m.journalFault(ctx, FaultRecord{Request: request, State: state, Reason: err.Error(), CreatedAt: now, UpdatedAt: m.clock.Now().UTC()})
		return FaultResolution{FaultID: request.ID, State: state, Reason: err.Error()}, err
	}
	pageIDs := uniquePageIDs(plan.PageIDs); resolvedRefs := uniqueRefs(plan.ResolvedRefs)
	if len(pageIDs) == 0 { return m.failFault(ctx, request, FaultUnresolved, "fault resolved to no context pages", errs.CodeNotFound) }
	maxTokens := request.MaxTokens; if maxTokens <= 0 || maxTokens > budget.MaxMaterializedTokens { maxTokens = budget.MaxMaterializedTokens }
	tokens := 0; metas := make([]PageMeta, 0, len(pageIDs))
	for _, pageID := range pageIDs {
		meta, err := m.currentPage(resolveCtx, request.AgentID, pageID, request.Kind == FaultFreshness); if err != nil { return m.failFault(ctx, request, FaultUnresolved, err.Error(), errs.CodeOf(err)) }
		if !scopeAllowed(meta.Scope, request.AllowedScopes) { return m.failFault(ctx, request, FaultDenied, "resolved page is outside visible scope", errs.CodePermissionDenied) }
		if meta.SupersededBy != nil && request.Kind != FaultFreshness { return m.failFault(ctx, request, FaultUnresolved, "resolved page is superseded", errs.CodeConflict) }
		tokens += meta.TokenEstimate
		if tokens > maxTokens {
			reason := "resolved representation exceeds fault token budget; projection or compaction required"
			m.journalFault(ctx, FaultRecord{Request: request, State: FaultNeedsProjection, PageIDs: pageIDs, ResolvedRefs: resolvedRefs, Tokens: tokens, Reason: reason, CreatedAt: now, UpdatedAt: m.clock.Now().UTC()})
			return FaultResolution{FaultID: request.ID, State: FaultNeedsProjection, PageIDs: pageIDs, ResolvedRefs: resolvedRefs, Tokens: tokens, Reason: reason}, errs.New(errs.CodeResourceExhausted, "mmu.fault.resolve", reason)
		}
		metas = append(metas, meta)
	}
	if err := m.commitFaultTokens(request, budget, tokens, pageIDs); err != nil { return FaultResolution{FaultID: request.ID, State: FaultBudgetExceeded, PageIDs: pageIDs, Tokens: tokens, Reason: err.Error()}, err }
	leaseIDs := make([]id.LeaseID, 0, len(metas))
	for _, meta := range metas { leaseID, err := m.grantFaultLease(ctx, request, meta); if err != nil { return FaultResolution{}, err }; leaseIDs = append(leaseIDs, leaseID) }
	result := FaultResolution{FaultID: request.ID, State: FaultResolved, PageIDs: pageIDs, ResolvedRefs: resolvedRefs, LeaseIDs: leaseIDs, Tokens: tokens, Reason: string(request.Purpose)}
	m.journalFault(ctx, FaultRecord{Request: request, State: FaultResolved, PageIDs: pageIDs, ResolvedRefs: resolvedRefs, LeaseIDs: leaseIDs, Tokens: tokens, Reason: string(request.Purpose), CreatedAt: now, UpdatedAt: m.clock.Now().UTC()})
	return result, nil
}

func validateFaultShape(request ContextFaultRequest) error {
	switch request.Kind {
	case FaultRecall:
		if request.Query == nil { return errs.New(errs.CodeInvalidArgument, "mmu.fault.resolve", "RecallFault requires a query") }
	case FaultDependency:
		if request.Reference == nil && len(request.RequiredBy) == 0 { return errs.New(errs.CodeInvalidArgument, "mmu.fault.resolve", "DependencyFault requires references") }
	default:
		if request.Reference == nil { return errs.New(errs.CodeInvalidArgument, "mmu.fault.resolve", fmt.Sprintf("%s requires a reference", request.Kind)) }
	}
	return nil
}

func validFaultKind(kind FaultKind) bool {
	switch kind { case FaultReference, FaultRecall, FaultEvidence, FaultFreshness, FaultDependency, FaultRepresentation: return true; default: return false }
}

func (m *Manager) failFault(ctx context.Context, request ContextFaultRequest, state FaultState, reason string, code errs.Code) (FaultResolution, error) {
	now := m.clock.Now().UTC(); m.journalFault(ctx, FaultRecord{Request: request, State: state, Reason: reason, CreatedAt: now, UpdatedAt: now})
	return FaultResolution{FaultID: request.ID, State: state, Reason: reason}, errs.New(code, "mmu.fault.resolve", reason)
}

func (m *Manager) journalFault(ctx context.Context, record FaultRecord) {
	if journal, ok := m.repo.(FaultJournal); ok { _ = journal.PutContextFault(ctx, record) }
}
