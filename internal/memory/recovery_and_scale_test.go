package memory_test

import (
	"testing"

	"github.com/Methamorphe/go-agent/internal/memory"
)

func TestMEM010RawEpisodeRemainsAddressableAfterConsolidation(t *testing.T) {
	h := newHarness(t, memory.Config{})
	defer h.close(t)
	e := h.evidence(t, "episode:task-42", "1", memory.ProvenanceRuntimeVerified, memory.EvidenceEpisode)
	b := h.belief(t, "task 42 established the repository build command", memory.BeliefActive, memory.EvidenceLinkInput{EvidenceID: e.ID, Relation: memory.EvidenceSupports})
	inspection, err := h.svc.Inspect(h.ctx, b.ID)
	if err != nil { t.Fatal(err) }
	if len(inspection.Evidence) != 1 || inspection.Evidence[0].Kind != memory.EvidenceEpisode || inspection.Evidence[0].SourceRef != "episode:task-42" {
		t.Fatalf("raw episode no longer addressable: %#v", inspection.Evidence)
	}
}

func TestMEM011CrashMidPropagationResumesWithoutCorruption(t *testing.T) {
	cfg := memory.Config{PropagationBatchSize: 1, MaxPropagationWork: 64, MaxPropagationDepth: 16}
	h := newHarness(t, cfg)
	e := h.evidence(t, "repo:auth.go", "A", memory.ProvenanceRepositorySource, memory.EvidenceRepository)
	b1 := h.belief(t, "sessions use Redis", memory.BeliefActive, memory.EvidenceLinkInput{EvidenceID: e.ID, Relation: memory.EvidenceVerifies})
	b2, err := h.svc.CreateBelief(h.ctx, memory.BeliefInput{ActorID: h.actor, Proposition: "Redis is required for login", Scope: b1.Scope, Status: memory.BeliefActive, Dependencies: []memory.BeliefDependencyInput{{BeliefID: b1.ID, Relation: memory.BeliefDerivedFrom}}})
	if err != nil { t.Fatal(err) }
	job, err := h.svc.RetractEvidence(h.ctx, h.actor, e.ID, "source retracted")
	if err != nil { t.Fatal(err) }
	partial, err := h.svc.RunPropagationBatch(h.ctx, job.ID)
	if err != nil { t.Fatal(err) }
	if partial.ProcessedCount != 1 { t.Fatalf("expected one durable work item, got %d", partial.ProcessedCount) }
	path := h.path
	h.close(t)

	reopened := openHarness(t, path, cfg)
	defer reopened.close(t)
	finished, err := reopened.svc.DrainPropagation(reopened.ctx, job.ID)
	if err != nil { t.Fatal(err) }
	if finished.State != memory.PropagationDone { t.Fatalf("job did not resume to DONE: %s", finished.State) }
	got, err := reopened.store.Belief(reopened.ctx, b2.ID)
	if err != nil { t.Fatal(err) }
	if got.Status != memory.BeliefNeedsReview && got.Status != memory.BeliefStale { t.Fatalf("dependent escaped invalidation: %s", got.Status) }
	history, err := reopened.store.StatusHistory(reopened.ctx, b1.ID)
	if err != nil { t.Fatal(err) }
	if len(history) > 3 { t.Fatalf("replay duplicated status history: %d entries", len(history)) }
}

func TestMEM012GraphCycleCannotProduceInfinitePropagation(t *testing.T) {
	h := newHarness(t, memory.Config{MaxPropagationWork: 16, MaxPropagationDepth: 16, PropagationBatchSize: 2})
	defer h.close(t)
	e := h.evidence(t, "repo:a.go", "A", memory.ProvenanceRepositorySource, memory.EvidenceRepository)
	b1 := h.belief(t, "A supports B", memory.BeliefActive, memory.EvidenceLinkInput{EvidenceID: e.ID, Relation: memory.EvidenceSupports})
	b2 := h.belief(t, "B supports A", memory.BeliefActive)
	if err := h.svc.LinkBeliefs(h.ctx, h.actor, b1.ID, b2.ID, memory.BeliefSupports); err != nil { t.Fatal(err) }
	if err := h.svc.LinkBeliefs(h.ctx, h.actor, b2.ID, b1.ID, memory.BeliefSupports); err != nil { t.Fatal(err) }
	job, err := h.svc.RetractEvidence(h.ctx, h.actor, e.ID, "cycle root changed")
	if err != nil { t.Fatal(err) }
	finished, err := h.svc.DrainPropagation(h.ctx, job.ID)
	if err != nil { t.Fatal(err) }
	if finished.State != memory.PropagationDone { t.Fatalf("cycle propagation = %s", finished.State) }
	if finished.ProcessedCount > 3 { t.Fatalf("cycle was revisited: processed=%d", finished.ProcessedCount) }
}

func TestMEM013StaleEvidenceCannotRankAsFreshVerifiedEvidence(t *testing.T) {
	h := newHarness(t, memory.Config{})
	defer h.close(t)
	oldEvidence := h.evidence(t, "repo:session.go", "A", memory.ProvenanceRepositorySource, memory.EvidenceRepository)
	oldBelief := h.belief(t, "session storage uses Redis architecture", memory.BeliefActive, memory.EvidenceLinkInput{EvidenceID: oldEvidence.ID, Relation: memory.EvidenceVerifies})
	job, err := h.svc.ObserveSourceVersion(h.ctx, h.actor, memory.SourceVersion{SourceRef: "repo:session.go", Version: "B", ContentHash: digest("repo:session.go:B"), ObservedAt: h.clock.Now()})
	if err != nil { t.Fatal(err) }
	if _, err := h.svc.DrainPropagation(h.ctx, job.ID); err != nil { t.Fatal(err) }
	freshEvidence := h.evidence(t, "runtime:session-test", "B", memory.ProvenanceRuntimeVerified, memory.EvidenceTestResult)
	freshBelief := h.belief(t, "session storage uses PostgreSQL architecture", memory.BeliefActive, memory.EvidenceLinkInput{EvidenceID: freshEvidence.ID, Relation: memory.EvidenceVerifies})
	results, err := h.svc.Retrieve(h.ctx, memory.SearchQuery{Text: "session storage architecture", Limit: 10})
	if err != nil { t.Fatal(err) }
	if len(results) < 2 { t.Fatalf("expected both historical candidates, got %d", len(results)) }
	if results[0].Belief.ID != freshBelief.ID { t.Fatalf("stale belief ranked over fresh verified belief: first=%s old=%s", results[0].Belief.ID, oldBelief.ID) }
	for _, item := range results {
		if item.Belief.ID == oldBelief.ID && !item.NeedsEvidence { t.Fatal("stale belief did not request evidence") }
	}
}

func TestMEM015UserAmendmentCreatesVersionedSupersession(t *testing.T) {
	h := newHarness(t, memory.Config{})
	defer h.close(t)
	e1 := h.evidence(t, "user:release-policy", "1", memory.ProvenanceUserExplicit, memory.EvidenceUserStatement)
	old := h.belief(t, "release branch is main", memory.BeliefActive, memory.EvidenceLinkInput{EvidenceID: e1.ID, Relation: memory.EvidenceSupports})
	e2 := h.evidence(t, "user:release-policy", "2", memory.ProvenanceUserExplicit, memory.EvidenceUserStatement)
	newer, err := h.svc.AmendBelief(h.ctx, old.ID, memory.BeliefInput{ActorID: h.actor, Proposition: "release branch is stable", Status: memory.BeliefActive, Evidence: []memory.EvidenceLinkInput{{EvidenceID: e2.ID, Relation: memory.EvidenceSupports}}})
	if err != nil { t.Fatal(err) }
	if newer.Version != old.Version+1 { t.Fatalf("new version=%d old=%d", newer.Version, old.Version) }
	oldNow, err := h.store.Belief(h.ctx, old.ID); if err != nil { t.Fatal(err) }
	if oldNow.Status != memory.BeliefSuperseded { t.Fatalf("amended old belief = %s", oldNow.Status) }
	incoming, err := h.store.BeliefIncoming(h.ctx, old.ID); if err != nil { t.Fatal(err) }
	found := false
	for _, edge := range incoming { if edge.From == newer.ID && edge.Relation == memory.BeliefSupersedes { found = true } }
	if !found { t.Fatal("versioned supersession edge missing") }
	if _, err := h.svc.Inspect(h.ctx, old.ID); err != nil { t.Fatalf("superseded belief no longer inspectable: %v", err) }
}
