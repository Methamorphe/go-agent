package mmu

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Methamorphe/go-agent/internal/clock"
	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/objectstore"
)

const testAgent id.AgentID = "agt_test"

type fakeIDs struct {
	mu sync.Mutex
	n  int
}

func (f *fakeIDs) next(prefix string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.n++
	return prefix + "_" + strings.Repeat("0", 28) + fourDigits(f.n)
}

func fourDigits(value int) string {
	digits := []byte{'0', '0', '0', '0'}
	for i := len(digits) - 1; i >= 0 && value > 0; i-- {
		digits[i] = byte('0' + value%10)
		value /= 10
	}
	return string(digits)
}

func (f *fakeIDs) ContextPage() (id.ContextPageID, error) {
	return id.ContextPageID(f.next("ctx")), nil
}
func (f *fakeIDs) Lease() (id.LeaseID, error) { return id.LeaseID(f.next("lse")), nil }

type fakeObjects struct {
	mu    sync.Mutex
	items map[objectstore.Ref]string
	opens int
}

func newFakeObjects() *fakeObjects { return &fakeObjects{items: make(map[objectstore.Ref]string)} }

func (f *fakeObjects) Put(_ context.Context, reader io.Reader) (objectstore.Meta, error) {
	body, err := io.ReadAll(reader)
	if err != nil {
		return objectstore.Meta{}, err
	}
	sum := sha256.Sum256(body)
	ref := objectstore.Ref("sha256:" + hex.EncodeToString(sum[:]))
	f.mu.Lock()
	f.items[ref] = string(body)
	f.mu.Unlock()
	return objectstore.Meta{Ref: ref, Algorithm: "sha256", Digest: hex.EncodeToString(sum[:]), Size: int64(len(body))}, nil
}

func (f *fakeObjects) Open(ref objectstore.Ref) (io.ReadCloser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	body, ok := f.items[ref]
	if !ok {
		return nil, errs.New(errs.CodeNotFound, "fake.objects", "object not found")
	}
	f.opens++
	return io.NopCloser(strings.NewReader(body)), nil
}

func (f *fakeObjects) openCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.opens
}

type fakeRepo struct {
	mu        sync.Mutex
	pages     map[id.ContextPageID]PageMeta
	search    map[id.ContextPageID]string
	leases    map[id.ContextPageID]ContextLease
	manifests map[id.InvocationID]objectstore.Ref
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		pages:     make(map[id.ContextPageID]PageMeta),
		search:    make(map[id.ContextPageID]string),
		leases:    make(map[id.ContextPageID]ContextLease),
		manifests: make(map[id.InvocationID]objectstore.Ref),
	}
}

func (f *fakeRepo) PutContextPage(_ context.Context, meta PageMeta, searchText string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pages[meta.ID] = meta
	f.search[meta.ID] = strings.ToLower(searchText)
	return nil
}

func (f *fakeRepo) ContextPage(_ context.Context, agentID id.AgentID, pageID id.ContextPageID) (PageMeta, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	meta, ok := f.pages[pageID]
	if !ok || meta.AgentID != agentID {
		return PageMeta{}, errs.New(errs.CodeNotFound, "fake.page", "not found")
	}
	return meta, nil
}

func (f *fakeRepo) SearchContextPages(_ context.Context, query CandidateQuery) ([]Candidate, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	terms := strings.Fields(strings.ToLower(query.Text))
	results := make([]Candidate, 0, query.Limit)
	for pageID, meta := range f.pages {
		if meta.AgentID != query.AgentID || !scopeAllowed(meta.Scope, query.Scopes) || !typeAllowed(meta.Type, query.Types) {
			continue
		}
		text := f.search[pageID]
		matches := 0
		for _, term := range terms {
			if strings.Contains(text, term) {
				matches++
			}
		}
		if len(terms) > 0 && matches == 0 {
			continue
		}
		score := 0.0
		if len(terms) > 0 {
			score = float64(matches) / float64(len(terms))
		}
		results = append(results, Candidate{Meta: meta, LexicalScore: score})
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Meta.ID < results[j].Meta.ID })
	if len(results) > query.Limit {
		results = results[:query.Limit]
	}
	return results, nil
}

func typeAllowed(pageType PageType, allowed []PageType) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, candidate := range allowed {
		if pageType == candidate {
			return true
		}
	}
	return false
}

func (f *fakeRepo) TouchContextPages(context.Context, id.AgentID, []id.ContextPageID, time.Time) error {
	return nil
}

func (f *fakeRepo) MarkContextPagesCompacted(_ context.Context, agentID id.AgentID, pageIDs []id.ContextPageID, summaryID id.ContextPageID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, pageID := range pageIDs {
		meta := f.pages[pageID]
		if meta.AgentID != agentID {
			continue
		}
		value := summaryID
		meta.CompactedBy = &value
		f.pages[pageID] = meta
	}
	return nil
}

func (f *fakeRepo) PutContextLease(_ context.Context, lease ContextLease) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.leases[lease.PageID] = lease
	return nil
}

func (f *fakeRepo) ActiveContextLeases(_ context.Context, agentID id.AgentID, now time.Time) ([]ContextLease, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	leases := make([]ContextLease, 0)
	for _, lease := range f.leases {
		if lease.AgentID != agentID || lease.RemainingBuilds <= 0 || (lease.ExpiresAt != nil && !lease.ExpiresAt.After(now)) {
			continue
		}
		leases = append(leases, lease)
	}
	sort.Slice(leases, func(i, j int) bool { return leases[i].PageID < leases[j].PageID })
	return leases, nil
}

func (f *fakeRepo) ConsumeContextLeases(_ context.Context, agentID id.AgentID, pageIDs []id.ContextPageID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, pageID := range pageIDs {
		lease, ok := f.leases[pageID]
		if !ok || lease.AgentID != agentID {
			continue
		}
		lease.RemainingBuilds--
		if lease.RemainingBuilds <= 0 {
			delete(f.leases, pageID)
		} else {
			f.leases[pageID] = lease
		}
	}
	return nil
}

func (f *fakeRepo) PutContextManifest(_ context.Context, invocationID id.InvocationID, _ id.AgentID, ref objectstore.Ref, _ time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.manifests[invocationID] = ref
	return nil
}

func newTestManager(t *testing.T) (*Manager, *fakeRepo, *fakeObjects, *fakeIDs, *clock.FakeClock) {
	t.Helper()
	repo := newFakeRepo()
	objects := newFakeObjects()
	ids := &fakeIDs{}
	fakeClock := clock.NewFakeClock(time.Date(2026, 8, 28, 20, 0, 0, 0, time.UTC))
	manager, err := New(repo, objects, ids, fakeClock, DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	return manager, repo, objects, ids, fakeClock
}

func createPage(t *testing.T, manager *Manager, pageType PageType, scope Scope, source, content string, importance float64) PageMeta {
	t.Helper()
	meta, err := manager.CreatePage(context.Background(), PageInput{
		AgentID: testAgent, Type: pageType, Scope: scope, SourceRef: source,
		Content: content, Importance: importance, Confidence: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	return meta
}

func standardBudget() Budget {
	return Budget{ModelMaxContext: 2_000, ReservedOutputTokens: 200, ProviderOverhead: 100, SafetyMargin: 100, SchemaTokens: 100, ActiveTokens: 100}
}

func visibleScopes() []Scope {
	return []Scope{ScopeProcess, ScopeRootTask, ScopeProject, ScopeWorkspace, ScopeWorld}
}

func TestMMU001HardBudgetNeverExceeded(t *testing.T) {
	manager, _, _, _, _ := newTestManager(t)
	mandatory := createPage(t, manager, PageDocumentation, ScopeProcess, "system", strings.Repeat("mandatory ", 30), 1)
	for i := 0; i < 30; i++ {
		createPage(t, manager, PageSourceExcerpt, ScopeWorkspace, "auth", strings.Repeat("auth rotation evidence ", 25), 0.8)
	}
	set, err := manager.Build(context.Background(), BuildRequest{AgentID: testAgent, Model: "test", Budget: standardBudget(), AllowedScopes: visibleScopes(), MandatoryIDs: []id.ContextPageID{mandatory.ID}, Query: "auth rotation"})
	if err != nil {
		t.Fatal(err)
	}
	if set.Manifest.EstimatedUsed > set.Manifest.InputBudget {
		t.Fatalf("used %d > budget %d", set.Manifest.EstimatedUsed, set.Manifest.InputBudget)
	}
}

func TestMMU002MandatoryOverflowFailsExplicitly(t *testing.T) {
	manager, _, _, _, _ := newTestManager(t)
	mandatory := createPage(t, manager, PageDocumentation, ScopeProcess, "system", strings.Repeat("critical ", 700), 1)
	budget := Budget{ModelMaxContext: 1_200, ReservedOutputTokens: 100, ProviderOverhead: 100, SafetyMargin: 100, SchemaTokens: 50, ActiveTokens: 50}
	_, err := manager.Build(context.Background(), BuildRequest{AgentID: testAgent, Model: "test", Budget: budget, AllowedScopes: visibleScopes(), MandatoryIDs: []id.ContextPageID{mandatory.ID}})
	if !errors.Is(err, ErrContextBudgetImpossible) {
		t.Fatalf("expected ContextBudgetImpossible, got %v", err)
	}
}

func TestMMU003SupersededPageNotSelectedNormally(t *testing.T) {
	manager, repo, _, _, _ := newTestManager(t)
	old := createPage(t, manager, PageDecision, ScopeProject, "decision", "auth sessions use files", 1)
	newer := createPage(t, manager, PageDecision, ScopeProject, "decision", "auth sessions use redis", 1)
	repo.mu.Lock()
	meta := repo.pages[old.ID]
	value := newer.ID
	meta.SupersededBy = &value
	repo.pages[old.ID] = meta
	repo.mu.Unlock()
	set, err := manager.Build(context.Background(), BuildRequest{AgentID: testAgent, Model: "test", Budget: standardBudget(), AllowedScopes: visibleScopes(), Query: "auth sessions"})
	if err != nil {
		t.Fatal(err)
	}
	for _, page := range set.Pages {
		if page.Meta.ID == old.ID {
			t.Fatal("superseded page was selected")
		}
	}
}

func TestMMU004ExplicitVisiblePageOutranksGenericSearch(t *testing.T) {
	manager, _, _, _, _ := newTestManager(t)
	explicit := createPage(t, manager, PageDocumentation, ScopeProject, "old", "legacy auth detail", 0.1)
	createPage(t, manager, PageDecision, ScopeProject, "new", "auth rotation current decision", 1)
	result, err := manager.Recall(context.Background(), RecallRequest{AgentID: testAgent, AllowedScopes: visibleScopes(), Query: RecallQuery{Text: "auth rotation", ExplicitIDs: []id.ContextPageID{explicit.ID}, Limit: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Selected) != 1 || result.Selected[0].PageID != explicit.ID || result.Selected[0].Reason != "explicit_reference" {
		t.Fatalf("explicit page did not dominate: %+v", result.Selected)
	}
}

func TestMMU005InaccessibleScopeNeverReturned(t *testing.T) {
	manager, _, _, _, _ := newTestManager(t)
	private := createPage(t, manager, PageDecision, ScopeWorkspace, "secret", "workspace-only fact", 1)
	result, err := manager.Recall(context.Background(), RecallRequest{AgentID: testAgent, AllowedScopes: []Scope{ScopeProcess}, Query: RecallQuery{Text: "workspace fact", ExplicitIDs: []id.ContextPageID{private.ID}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Selected) != 0 || len(result.Unresolved) != 1 || result.Unresolved[0] != private.ID {
		t.Fatalf("inaccessible page leaked: %+v", result)
	}
}

func TestMMU006HundredThousandPagesDoNotMaterializeAllBodies(t *testing.T) {
	manager, repo, objects, _, fakeClock := newTestManager(t)
	body := "needle relevant durable fact"
	object, err := objects.Put(context.Background(), strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	now := fakeClock.Now()
	repo.mu.Lock()
	for i := 0; i < 100_000; i++ {
		pageID := id.ContextPageID("ctx_bulk_" + fourDigits(i/10_000) + fourDigits(i%10_000))
		meta := PageMeta{ID: pageID, AgentID: testAgent, Type: PageSourceExcerpt, Scope: ScopeProject, ObjectRef: object.Ref, TokenEstimate: 10, Importance: 0.5, Confidence: 1, CreatedAt: now, LastAccessedAt: now}
		repo.pages[pageID] = meta
		if i < 20 {
			repo.search[pageID] = "needle relevant durable fact"
		} else {
			repo.search[pageID] = "unrelated historical page"
		}
	}
	repo.mu.Unlock()
	set, err := manager.Build(context.Background(), BuildRequest{AgentID: testAgent, Model: "test", Budget: standardBudget(), AllowedScopes: []Scope{ScopeProject}, Query: "needle relevant"})
	if err != nil {
		t.Fatal(err)
	}
	if len(set.Pages) == 0 {
		t.Fatal("expected relevant pages")
	}
	if opens := objects.openCount(); opens > 256 {
		t.Fatalf("materialized %d bodies for 100k metadata corpus", opens)
	}
	if set.Manifest.EstimatedUsed > set.Manifest.InputBudget {
		t.Fatal("100k corpus escaped hard context budget")
	}
}

func TestMMU007TokenCacheBounded(t *testing.T) {
	cache := newCachedEstimator(ConservativeEstimator{}, 4)
	for i := 0; i < 100; i++ {
		cache.EstimateText("test", "body "+fourDigits(i))
	}
	if cache.Len() > 4 {
		t.Fatalf("token cache grew to %d", cache.Len())
	}
}

func TestMMU008RepeatedRecallLoopDetected(t *testing.T) {
	manager, _, _, _, _ := newTestManager(t)
	createPage(t, manager, PageDecision, ScopeProject, "decision", "session rotation decision", 1)
	request := RecallRequest{AgentID: testAgent, AllowedScopes: visibleScopes(), Query: RecallQuery{Text: "session rotation", Limit: 2}}
	for i := 0; i < 2; i++ {
		if _, err := manager.Recall(context.Background(), request); err != nil {
			t.Fatalf("recall %d failed early: %v", i+1, err)
		}
	}
	_, err := manager.Recall(context.Background(), request)
	if !errors.Is(err, ErrRecallLoopDetected) {
		t.Fatalf("expected RecallLoopDetected, got %v", err)
	}
}

func TestMMU009SummaryRetainsSourceReferences(t *testing.T) {
	manager, repo, objects, _, _ := newTestManager(t)
	first := createPage(t, manager, PageToolResultSummary, ScopeProject, "tool:a", "first evidence", 0.8)
	second := createPage(t, manager, PageToolResultSummary, ScopeProject, "tool:b", "second evidence", 0.9)
	summary, err := manager.Compact(context.Background(), CompactRequest{AgentID: testAgent, SourceIDs: []id.ContextPageID{first.ID, second.ID}, Summary: "both observations support the migration", Type: PageAssistantSummary})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(summary.SummaryOf, []id.ContextPageID{first.ID, second.ID}) {
		t.Fatalf("summary lost source refs: %+v", summary.SummaryOf)
	}
	for _, sourceID := range []id.ContextPageID{first.ID, second.ID} {
		meta, err := repo.ContextPage(context.Background(), testAgent, sourceID)
		if err != nil {
			t.Fatal(err)
		}
		if meta.CompactedBy == nil || *meta.CompactedBy != summary.ID {
			t.Fatalf("source %s not linked to summary", sourceID)
		}
		if _, err := objects.Open(meta.ObjectRef); err != nil {
			t.Fatalf("original evidence was deleted: %v", err)
		}
	}
}

func TestMMU010ManifestDeterministicallyExplainsSelection(t *testing.T) {
	manager, _, _, _, _ := newTestManager(t)
	mandatory := createPage(t, manager, PageDocumentation, ScopeProcess, "system", "fixed runtime instructions", 1)
	createPage(t, manager, PageDecision, ScopeProject, "decision", "auth session rotation uses redis", 0.9)
	request := BuildRequest{AgentID: testAgent, Model: "test", Budget: standardBudget(), AllowedScopes: visibleScopes(), MandatoryIDs: []id.ContextPageID{mandatory.ID}, Query: "auth session rotation"}
	first, err := manager.Build(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.Build(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first.Manifest, second.Manifest) {
		t.Fatalf("manifest changed for fixed inputs:\nfirst=%+v\nsecond=%+v", first.Manifest, second.Manifest)
	}
}
