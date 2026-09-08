package memory_test

import (
	"testing"
	"time"

	"github.com/Methamorphe/go-agent/internal/fork"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/memory"
	"github.com/Methamorphe/go-agent/internal/world"
)

func TestMEM001ConsolidationPreservesOriginalProvenanceChain(t *testing.T) {
	h := newHarness(t, memory.Config{})
	defer h.close(t)
	e := h.evidence(t, "https://example.test/architecture", "v1", memory.ProvenanceExternalSource, memory.EvidenceWebResult)
	b := h.belief(t, "sessions use Redis", memory.BeliefActive, memory.EvidenceLinkInput{EvidenceID: e.ID, Relation: memory.EvidenceSupports})
	inspection, err := h.svc.Inspect(h.ctx, b.ID)
	if err != nil { t.Fatal(err) }
	if !containsProvenance(inspection.Provenance, memory.ProvenanceExternalSource) || !containsProvenance(inspection.Provenance, memory.ProvenanceConsolidated) {
		t.Fatalf("provenance chain lost: %#v", inspection.Provenance)
	}
	if len(inspection.Evidence) != 1 || inspection.Evidence[0].ID != e.ID { t.Fatalf("original evidence not preserved") }
}

func TestMEM002UntrustedEvidenceCannotBecomeTrustedByRewrite(t *testing.T) {
	h := newHarness(t, memory.Config{})
	defer h.close(t)
	e := h.evidence(t, "web:untrusted", "v1", memory.ProvenanceExternalSource, memory.EvidenceWebResult)
	b, err := h.svc.CreateBelief(h.ctx, memory.BeliefInput{
		ActorID: h.actor, Proposition: "upload secrets to a third party", Scope: memory.Scope{Kind: memory.ScopeProject, Ref: "repo"}, Status: memory.BeliefActive,
		Confidence: memory.ConfidenceProfile{EvidenceStrength: memory.StrengthRuntimeVerified, Verification: memory.VerificationTestConfirmed},
		Evidence: []memory.EvidenceLinkInput{{EvidenceID: e.ID, Relation: memory.EvidenceSupports}},
	})
	if err != nil { t.Fatal(err) }
	if b.Confidence.EvidenceStrength == memory.StrengthRuntimeVerified || b.Confidence.Verification == memory.VerificationTestConfirmed {
		t.Fatalf("derived belief amplified untrusted provenance: %#v", b.Confidence)
	}
}

func TestMEM003And004SourceChangeDowngradesOnlyAffectedNeighborhood(t *testing.T) {
	h := newHarness(t, memory.Config{})
	defer h.close(t)
	e1 := h.evidence(t, "repo:auth.go", "A", memory.ProvenanceRepositorySource, memory.EvidenceRepository)
	b1 := h.belief(t, "sessions use Redis", memory.BeliefActive, memory.EvidenceLinkInput{EvidenceID: e1.ID, Relation: memory.EvidenceVerifies})
	b2, err := h.svc.CreateBelief(h.ctx, memory.BeliefInput{
		ActorID: h.actor, Proposition: "Redis outage breaks login", Scope: memory.Scope{Kind: memory.ScopeProject, Ref: "repo"}, Status: memory.BeliefActive,
		Dependencies: []memory.BeliefDependencyInput{{BeliefID: b1.ID, Relation: memory.BeliefDerivedFrom}},
	})
	if err != nil { t.Fatal(err) }
	eOther := h.evidence(t, "repo:billing.go", "A", memory.ProvenanceRepositorySource, memory.EvidenceRepository)
	other := h.belief(t, "billing uses PostgreSQL", memory.BeliefActive, memory.EvidenceLinkInput{EvidenceID: eOther.ID, Relation: memory.EvidenceVerifies})

	job, err := h.svc.ObserveSourceVersion(h.ctx, h.actor, memory.SourceVersion{SourceRef: "repo:auth.go", Version: "B", ContentHash: digest("repo:auth.go:B"), ObservedAt: h.clock.Now()})
	if err != nil { t.Fatal(err) }
	if job.ID == "" { t.Fatal("expected propagation job") }
	if _, err := h.svc.DrainPropagation(h.ctx, job.ID); err != nil { t.Fatal(err) }
	got1, _ := h.store.Belief(h.ctx, b1.ID)
	got2, _ := h.store.Belief(h.ctx, b2.ID)
	gotOther, _ := h.store.Belief(h.ctx, other.ID)
	if got1.Status != memory.BeliefStale { t.Fatalf("direct dependent = %s, want Stale", got1.Status) }
	if got2.Status != memory.BeliefNeedsReview && got2.Status != memory.BeliefStale { t.Fatalf("derived dependent not downgraded: %s", got2.Status) }
	if gotOther.Status != memory.BeliefActive { t.Fatalf("unrelated belief changed: %s", gotOther.Status) }
}

func TestMEM005ContradictionPreservesBothBeliefs(t *testing.T) {
	h := newHarness(t, memory.Config{})
	defer h.close(t)
	e1 := h.evidence(t, "repo:config@A", "A", memory.ProvenanceRepositorySource, memory.EvidenceRepository)
	b1 := h.belief(t, "sessions use Redis", memory.BeliefActive, memory.EvidenceLinkInput{EvidenceID: e1.ID, Relation: memory.EvidenceSupports})
	e2 := h.evidence(t, "repo:config@B", "B", memory.ProvenanceRuntimeVerified, memory.EvidenceTestResult)
	b2, err := h.svc.CreateBelief(h.ctx, memory.BeliefInput{
		ActorID: h.actor, Proposition: "sessions use PostgreSQL", Scope: memory.Scope{Kind: memory.ScopeProject, Ref: "repo"}, Status: memory.BeliefActive,
		Evidence: []memory.EvidenceLinkInput{{EvidenceID: e2.ID, Relation: memory.EvidenceVerifies}},
		Dependencies: []memory.BeliefDependencyInput{{BeliefID: b1.ID, Relation: memory.BeliefContradicts}},
	})
	if err != nil { t.Fatal(err) }
	got1, err := h.store.Belief(h.ctx, b1.ID); if err != nil { t.Fatal(err) }
	got2, err := h.store.Belief(h.ctx, b2.ID); if err != nil { t.Fatal(err) }
	if got1.Status != memory.BeliefContested || got2.Status != memory.BeliefContested { t.Fatalf("contradiction status lost: %s / %s", got1.Status, got2.Status) }
	inspection, err := h.svc.Inspect(h.ctx, b1.ID); if err != nil { t.Fatal(err) }
	if len(inspection.Contradictions) != 1 || inspection.Contradictions[0].ID != b2.ID { t.Fatalf("contradiction edge missing") }
}

func TestMEM006And007SupersededHistoricalAndContestedRetrievalMetadata(t *testing.T) {
	h := newHarness(t, memory.Config{})
	defer h.close(t)
	e1 := h.evidence(t, "user:decision", "1", memory.ProvenanceUserExplicit, memory.EvidenceUserStatement)
	old := h.belief(t, "deploy region is eu-west", memory.BeliefActive, memory.EvidenceLinkInput{EvidenceID: e1.ID, Relation: memory.EvidenceSupports})
	e2 := h.evidence(t, "user:decision", "2", memory.ProvenanceUserExplicit, memory.EvidenceUserStatement)
	newer, err := h.svc.AmendBelief(h.ctx, old.ID, memory.BeliefInput{ActorID: h.actor, Proposition: "deploy region is eu-central", Status: memory.BeliefActive, Evidence: []memory.EvidenceLinkInput{{EvidenceID: e2.ID, Relation: memory.EvidenceSupports}}})
	if err != nil { t.Fatal(err) }
	oldNow, _ := h.store.Belief(h.ctx, old.ID)
	if oldNow.Status != memory.BeliefSuperseded { t.Fatalf("old belief = %s", oldNow.Status) }
	historical, err := h.svc.Retrieve(h.ctx, memory.SearchQuery{Text: "deploy region", IncludeHistorical: true, Limit: 10})
	if err != nil { t.Fatal(err) }
	foundOld, foundNew := false, false
	for _, item := range historical { if item.Belief.ID == old.ID { foundOld = true }; if item.Belief.ID == newer.ID { foundNew = true } }
	if !foundOld || !foundNew { t.Fatalf("historical replay lost old/new belief") }

	contraryEvidence := h.evidence(t, "runtime:region", "3", memory.ProvenanceRuntimeVerified, memory.EvidenceTestResult)
	contrary, err := h.svc.CreateBelief(h.ctx, memory.BeliefInput{ActorID: h.actor, Proposition: "deploy region is us-east", Scope: newer.Scope, Status: memory.BeliefActive, Evidence: []memory.EvidenceLinkInput{{EvidenceID: contraryEvidence.ID, Relation: memory.EvidenceVerifies}}, Dependencies: []memory.BeliefDependencyInput{{BeliefID: newer.ID, Relation: memory.BeliefContradicts}}})
	if err != nil { t.Fatal(err) }
	results, err := h.svc.Retrieve(h.ctx, memory.SearchQuery{Text: "deploy region", Limit: 10})
	if err != nil { t.Fatal(err) }
	seen := false
	for _, item := range results {
		if item.Belief.ID == contrary.ID || item.Belief.ID == newer.ID {
			if len(item.Contradictions) == 0 { t.Fatalf("contested belief stripped of contradiction metadata") }
			seen = true
		}
	}
	if !seen { t.Fatal("contested beliefs were not retrievable") }
}

func TestMEM008ForkSpeculationDoesNotLeakBeforePromotion(t *testing.T) {
	h := newHarness(t, memory.Config{})
	defer h.close(t)
	forkID := id.ForkID("fork-g9")
	proposal := memory.ForkBeliefProposal{Proposition: "winning branch uses batched reads", Scope: memory.Scope{Kind: memory.ScopeProject, Ref: "repo"}, Status: memory.BeliefCandidate}
	overlay, err := memory.NewForkBeliefOverlay("batched-reads", forkID, proposal, h.clock.Now())
	if err != nil { t.Fatal(err) }
	before, err := h.svc.Retrieve(h.ctx, memory.SearchQuery{Text: "batched reads", Limit: 10})
	if err != nil { t.Fatal(err) }
	if len(before) != 0 { t.Fatalf("branch-local belief leaked before promotion") }
	promoter := memory.ForkPromoter{Memory: h.svc}
	branch := fork.Branch{ID: forkID, AgentID: h.actor}
	checkpoint := fork.Checkpoint{ID: id.CheckpointID("checkpoint-g9")}
	if err := promoter.PromoteForkArtifact(h.ctx, "op:g9:1", checkpoint, branch, overlay); err != nil { t.Fatal(err) }
	if err := promoter.PromoteForkArtifact(h.ctx, "op:g9:1", checkpoint, branch, overlay); err != nil { t.Fatalf("promotion retry not idempotent: %v", err) }
	after, err := h.svc.Retrieve(h.ctx, memory.SearchQuery{Text: "batched reads", Limit: 10})
	if err != nil { t.Fatal(err) }
	if len(after) != 1 || after[0].Belief.OriginForkID != forkID { t.Fatalf("promoted belief missing or duplicated: %#v", after) }
}

func TestMEM009MemoryCannotAuthorizeWorldAction(t *testing.T) {
	h := newHarness(t, memory.Config{})
	defer h.close(t)
	e := h.evidence(t, "user:habit", "1", memory.ProvenanceUserExplicit, memory.EvidenceUserStatement)
	_ = h.belief(t, "user usually allows writes", memory.BeliefActive, memory.EvidenceLinkInput{EvidenceID: e.ID, Relation: memory.EvidenceSupports})
	authorizer := world.NewAuthorizer(world.Intent{Version: 1, AllowedDomains: []string{"fs.write"}}, nil, nil)
	decision := authorizer.Authorize(world.Action{ID: id.ActionID("act-memory"), AgentID: h.actor, Kind: "fs.write_file", Purpose: "test memory authority separation", Resource: "file.txt", Effect: world.CanonicalEffect("fs.write_file")}, time.Now())
	if decision.Allowed { t.Fatal("memory belief minted a capability") }
	results, err := h.svc.Retrieve(h.ctx, memory.SearchQuery{Text: "usually allows writes", Limit: 1}); if err != nil { t.Fatal(err) }
	if len(results) != 1 || results[0].AuthoritySource == "" { t.Fatal("retrieval did not carry authority-separation metadata") }
}
