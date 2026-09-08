package mmu

import (
	"context"
	"testing"

	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
)

func ref(t *testing.T, value string) CognitiveRef {
	t.Helper()
	r, err := ParseCognitiveRef(value)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestCF001StableCognitiveReferences(t *testing.T) {
	for _, value := range []string{"ctx://ctx_1", "belief://blf_1", "evidence://evd_1", "object://sha256:abc", "event://evt_1", "checkpoint://chk_1", "agent://agt_1"} {
		r, err := ParseCognitiveRef(value)
		if err != nil || r.String() != value {
			t.Fatalf("parse %q: %v %q", value, err, r)
		}
	}
	if _, err := ParseCognitiveRef("file:///etc/passwd"); !errs.IsCode(err, errs.CodeInvalidArgument) {
		t.Fatalf("expected invalid scheme, got %v", err)
	}
}

func TestCF002ReferenceFaultLeasesPageForNextBuild(t *testing.T) {
	manager, _, _, _, _ := newTestManager(t)
	page := createPage(t, manager, PageDecision, ScopeProject, "decision", "session rotation uses redis", 1)
	r := ref(t, "ctx://"+page.ID.String())
	result, err := manager.ResolveFault(context.Background(), ContextFaultRequest{AgentID: testAgent, InvocationID: "inv_1", Kind: FaultReference, Reference: &r, Purpose: PurposeContinueReasoning, AllowedScopes: visibleScopes()})
	if err != nil {
		t.Fatal(err)
	}
	if result.State != FaultResolved || len(result.PageIDs) != 1 || result.PageIDs[0] != page.ID {
		t.Fatalf("unexpected resolution: %+v", result)
	}
	set, err := manager.Build(context.Background(), BuildRequest{AgentID: testAgent, Model: "test", Budget: standardBudget(), AllowedScopes: visibleScopes()})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, selected := range set.Pages {
		if selected.Meta.ID == page.ID && selected.Reason == "context_lease" {
			found = true
		}
	}
	if !found {
		t.Fatal("faulted page was not leased into next working set")
	}
}

func TestCF003EvidenceFaultPagesRawCompactionSources(t *testing.T) {
	manager, _, _, _, _ := newTestManager(t)
	a := createPage(t, manager, PageSourceExcerpt, ScopeProject, "a", "raw evidence alpha", 1)
	b := createPage(t, manager, PageSourceExcerpt, ScopeProject, "b", "raw evidence beta", 1)
	summary, err := manager.Compact(context.Background(), CompactRequest{AgentID: testAgent, SourceIDs: []id.ContextPageID{a.ID, b.ID}, Summary: "alpha and beta summary", Scope: ScopeProject})
	if err != nil {
		t.Fatal(err)
	}
	r := ref(t, "ctx://"+summary.ID.String())
	result, err := manager.ResolveFault(context.Background(), ContextFaultRequest{AgentID: testAgent, InvocationID: "inv_2", Kind: FaultEvidence, Reference: &r, Purpose: PurposeVerifyClaim, AllowedScopes: visibleScopes()})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.PageIDs) != 2 || result.PageIDs[0] != a.ID || result.PageIDs[1] != b.ID {
		t.Fatalf("raw evidence not paged: %+v", result.PageIDs)
	}
}

func TestCF004FreshnessFaultFollowsSupersession(t *testing.T) {
	manager, repo, _, _, _ := newTestManager(t)
	old := createPage(t, manager, PageDecision, ScopeProject, "decision", "old architecture", 1)
	fresh := createPage(t, manager, PageDecision, ScopeProject, "decision", "fresh architecture", 1)
	repo.mu.Lock()
	meta := repo.pages[old.ID]
	next := fresh.ID
	meta.SupersededBy = &next
	repo.pages[old.ID] = meta
	repo.mu.Unlock()
	r := ref(t, "ctx://"+old.ID.String())
	result, err := manager.ResolveFault(context.Background(), ContextFaultRequest{AgentID: testAgent, InvocationID: "inv_3", Kind: FaultFreshness, Reference: &r, AllowedScopes: visibleScopes()})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.PageIDs) != 1 || result.PageIDs[0] != fresh.ID {
		t.Fatalf("freshness did not follow supersession: %+v", result)
	}
}

type staticResolver struct {
	page       id.ContextPageID
	dependency CognitiveRef
}

func (s staticResolver) ResolveCognitiveRef(context.Context, ContextFaultRequest, CognitiveRef) (FaultResolutionPlan, error) {
	return FaultResolutionPlan{PageIDs: []id.ContextPageID{s.page}, Dependencies: []CognitiveRef{s.dependency}}, nil
}

func TestCF005DependencyPagingIsBoundedAndResolverIndependent(t *testing.T) {
	manager, _, _, _, _ := newTestManager(t)
	beliefPage := createPage(t, manager, PageRecallResult, ScopeProject, "belief", "belief projection", 1)
	evidencePage := createPage(t, manager, PageSourceExcerpt, ScopeProject, "evidence", "direct evidence", 1)
	if err := manager.RegisterResolver(RefBelief, staticResolver{page: beliefPage.ID, dependency: ref(t, "ctx://"+evidencePage.ID.String())}); err != nil {
		t.Fatal(err)
	}
	r := ref(t, "belief://blf_arch")
	result, err := manager.ResolveFault(context.Background(), ContextFaultRequest{AgentID: testAgent, InvocationID: "inv_4", Kind: FaultDependency, Reference: &r, Purpose: PurposeSatisfyDependency, AllowedScopes: visibleScopes()})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.PageIDs) != 2 {
		t.Fatalf("expected resolver page + dependency, got %+v", result.PageIDs)
	}
}

func TestCF006FaultStormStopsRepeatedPaging(t *testing.T) {
	manager, _, _, _, _ := newTestManager(t)
	page := createPage(t, manager, PageDecision, ScopeProject, "decision", "repeat me", 1)
	r := ref(t, "ctx://"+page.ID.String())
	request := ContextFaultRequest{AgentID: testAgent, InvocationID: "inv_storm", Kind: FaultReference, Reference: &r, AllowedScopes: visibleScopes()}
	for i := 0; i < DefaultFaultBudget().MaxRepeatedSetHits; i++ {
		if _, err := manager.ResolveFault(context.Background(), request); err != nil {
			t.Fatalf("fault %d failed early: %v", i, err)
		}
	}
	result, err := manager.ResolveFault(context.Background(), request)
	if err == nil || !errs.IsCode(err, errs.CodeResourceExhausted) {
		t.Fatalf("expected storm rejection, got result=%+v err=%v", result, err)
	}
	if result.State != FaultBudgetExceeded {
		t.Fatalf("expected budget exceeded state, got %s", result.State)
	}
}

func TestCF007RepresentationFaultRequiresProjectionWhenTooLarge(t *testing.T) {
	manager, _, _, _, _ := newTestManager(t)
	page := createPage(t, manager, PageDocumentation, ScopeProject, "large", "representation with enough tokens to exceed tiny fault budget", 1)
	r := ref(t, "ctx://"+page.ID.String())
	result, err := manager.ResolveFault(context.Background(), ContextFaultRequest{AgentID: testAgent, InvocationID: "inv_repr", Kind: FaultRepresentation, Reference: &r, MaxTokens: 1, AllowedScopes: visibleScopes()})
	if err == nil || result.State != FaultNeedsProjection {
		t.Fatalf("expected projection state, got %+v err=%v", result, err)
	}
}

func TestCF008EvidenceFaultCanInspectSupersededHistoricalEvidence(t *testing.T) {
	manager, repo, _, _, _ := newTestManager(t)
	historical := createPage(t, manager, PageSourceExcerpt, ScopeProject, "evidence", "historical source", 1)
	current := createPage(t, manager, PageSourceExcerpt, ScopeProject, "evidence", "current source", 1)
	repo.mu.Lock()
	meta := repo.pages[historical.ID]
	next := current.ID
	meta.SupersededBy = &next
	repo.pages[historical.ID] = meta
	repo.mu.Unlock()

	r := ref(t, "ctx://"+historical.ID.String())
	result, err := manager.ResolveFault(context.Background(), ContextFaultRequest{AgentID: testAgent, InvocationID: "inv_history", Kind: FaultEvidence, Reference: &r, Purpose: PurposeInspectHistory, AllowedScopes: visibleScopes()})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.PageIDs) != 1 || result.PageIDs[0] != historical.ID {
		t.Fatalf("historical evidence not preserved: %+v", result.PageIDs)
	}
}
