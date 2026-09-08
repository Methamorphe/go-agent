package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Methamorphe/go-agent/internal/improvement"
)

func TestImprovementStateSurvivesRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "g12.db")

	store, err := Open(ctx, Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	svc := improvement.NewService(store, improvement.Config{MinEvaluationTasks: 1, RequireIndependentEvaluator: true})
	base, err := svc.RegisterBaseline(ctx, improvement.ArtifactVersion{
		ArtifactID: "routing-default", Kind: improvement.KindRoutingPolicy,
		ContentRef: "object://routing-v1", OriginRef: "config://seed", Scope: improvement.Scope{Level: improvement.ScopeProject, Key: "Methamorphe/go-agent"},
	})
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := svc.CreateCandidate(ctx, improvement.ArtifactVersion{
		ArtifactID: base.ArtifactID, Kind: base.Kind, ContentRef: "object://routing-v2", OriginRef: "episode://42", Scope: base.Scope, CanarySampleCap: 1,
		Hypothesis: &improvement.Hypothesis{Observation: "avoidable expensive route", ProposedChange: "prefer cheaper verified route", ExpectedBenefit: "lower cost", Scope: "coding/go"},
	}, base.Version)
	if err != nil {
		t.Fatal(err)
	}
	eval := improvement.Evaluation{
		ID: "eval-routing-v2", ArtifactID: base.ArtifactID, BaselineVersion: base.Version, CandidateVersion: candidate.Version,
		CorpusRef: "corpus://held-out", EvaluatorRef: "verifier://deterministic", Independent: true,
		Outcomes: []improvement.Outcome{{TaskRef: "task://1", BaselineSuccess: true, CandidateSuccess: true, CostDelta: -0.3}}, EvidenceRefs: []string{"eval://routing-v2"},
	}
	if err := svc.RecordEvaluation(ctx, eval); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Advance(ctx, candidate.ArtifactID, candidate.Version, eval.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Advance(ctx, candidate.ArtifactID, candidate.Version, eval.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UseCandidate(ctx, candidate.ArtifactID, candidate.Version, improvement.EffectReadOnly, nil); err != nil {
		t.Fatal(err)
	}
	if err := svc.Promote(ctx, candidate.ArtifactID, candidate.Version, eval.ID); err != nil {
		t.Fatal(err)
	}
	manifest, err := svc.CaptureInvocation(ctx, "inv-routing-v2", map[improvement.ArtifactID]uint32{candidate.ArtifactID: candidate.Version})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = Open(ctx, Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	active, err := store.ActiveVersion(ctx, candidate.ArtifactID)
	if err != nil || active != candidate.Version {
		t.Fatalf("active=%d err=%v", active, err)
	}
	got, err := store.InvocationManifest(ctx, manifest.InvocationID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ManifestHash != manifest.ManifestHash || got.Artifacts[candidate.ArtifactID] != candidate.Version {
		t.Fatalf("manifest changed across restart: %+v", got)
	}
	promotion, err := store.Promotion(ctx, candidate.ArtifactID, candidate.Version)
	if err != nil {
		t.Fatal(err)
	}
	if promotion.BaselineVersion != base.Version || promotion.EvaluationID != eval.ID {
		t.Fatalf("promotion changed across restart: %+v", promotion)
	}
	kill := improvement.KillSwitch{Versions: map[string]bool{candidate.Key(): true}}
	if err := store.SetKillSwitch(ctx, kill); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(ctx, Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	persistedKill, err := store.KillSwitch(ctx)
	if err != nil || !persistedKill.Versions[candidate.Key()] {
		t.Fatalf("kill switch not durable: %+v %v", persistedKill, err)
	}
}
