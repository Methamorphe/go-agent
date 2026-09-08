package improvement

import (
	"context"
	"fmt"
	"testing"

	"github.com/Methamorphe/go-agent/internal/errs"
)

func newTestService(t *testing.T, min int) (*Service, *MemoryStore, ArtifactVersion) {
	t.Helper()
	ctx := context.Background()
	store := NewMemoryStore()
	svc := NewService(store, Config{MinEvaluationTasks: min, RequireIndependentEvaluator: true})
	baseline, err := svc.RegisterBaseline(ctx, ArtifactVersion{
		ArtifactID: "go-race-skill", Kind: KindSkill, ContentRef: "object://skill-v1", OriginRef: "episode://seed",
		Scope: Scope{Level: ScopeProject, Key: "Methamorphe/go-agent"}, RequiredCapabilities: []string{"repo.read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return svc, store, baseline
}

func candidateFor(base ArtifactVersion) ArtifactVersion {
	return ArtifactVersion{
		ArtifactID: base.ArtifactID, Kind: base.Kind, ContentRef: "object://skill-v2", OriginRef: "episode://failure-42",
		Scope: base.Scope, CanarySampleCap: 2, RequiredCapabilities: []string{"repo.read"},
		Hypothesis: &Hypothesis{
			Observation: "race debugging used too much review", ProposedChange: "run race tests first",
			ExpectedBenefit: "lower cost with equal correctness", ExpectedRisks: []string{"test startup cost"},
			Scope: "coding/go/concurrency-debug",
		},
	}
}

func evalFor(base, candidate ArtifactVersion, id EvaluationID, n int) Evaluation {
	out := make([]Outcome, n)
	for i := range out {
		out[i] = Outcome{TaskRef: fmt.Sprintf("task://%d", i), BaselineSuccess: true, CandidateSuccess: true, QualityDelta: 0.1}
	}
	return Evaluation{
		ID: id, ArtifactID: base.ArtifactID, BaselineVersion: base.Version, CandidateVersion: candidate.Version,
		CorpusRef: "corpus://held-out", EvaluatorRef: "verifier://deterministic", Independent: true,
		Outcomes: out, EvidenceRefs: []string{"eval://report"},
	}
}

func prepareCanary(t *testing.T, svc *Service, base ArtifactVersion, n int) (ArtifactVersion, Evaluation) {
	t.Helper()
	ctx := context.Background()
	c, err := svc.CreateCandidate(ctx, candidateFor(base), base.Version)
	if err != nil {
		t.Fatal(err)
	}
	e := evalFor(base, c, "eval-1", n)
	if err := svc.RecordEvaluation(ctx, e); err != nil {
		t.Fatal(err)
	}
	if st, err := svc.Advance(ctx, c.ArtifactID, c.Version, e.ID); err != nil || st != StatusShadow {
		t.Fatalf("shadow: %v %v", st, err)
	}
	if st, err := svc.Advance(ctx, c.ArtifactID, c.Version, e.ID); err != nil || st != StatusCanary {
		t.Fatalf("canary: %v %v", st, err)
	}
	return c, e
}

func TestIMP001CandidateCannotExpandCapabilityGrants(t *testing.T) {
	ctx := context.Background()
	svc, _, base := newTestService(t, 1)
	c := candidateFor(base)
	c.RequiredCapabilities = []string{"repo.read", "repo.write"}
	created, err := svc.CreateCandidate(ctx, c, base.Version)
	if err != nil {
		t.Fatal(err)
	}
	e := evalFor(base, created, "eval-cap", 1)
	if err := svc.RecordEvaluation(ctx, e); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Advance(ctx, created.ArtifactID, created.Version, e.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UseCandidate(ctx, created.ArtifactID, created.Version, EffectReadOnly, []string{"repo.read"}); !errs.IsCode(err, errs.CodePermissionDenied) {
		t.Fatalf("expected permission denied, got %v", err)
	}
}

func TestIMP002RootIntentAndEffectFloorAreNotArtifactKinds(t *testing.T) {
	ctx := context.Background()
	svc := NewService(NewMemoryStore(), Config{})
	for _, kind := range []ArtifactKind{"root_intent", "effect_floor", "capability_grant"} {
		_, err := svc.RegisterBaseline(ctx, ArtifactVersion{ArtifactID: "forbidden", Kind: kind, ContentRef: "x", OriginRef: "x", Scope: Scope{Level: ScopeGlobal}})
		if !errs.IsCode(err, errs.CodeInvalidArgument) {
			t.Fatalf("kind %s should be rejected: %v", kind, err)
		}
	}
}

func TestIMP003PromotionRecordsBaselineCandidateAndEvaluationRefs(t *testing.T) {
	ctx := context.Background()
	svc, store, base := newTestService(t, 3)
	c, e := prepareCanary(t, svc, base, 3)
	if err := svc.Promote(ctx, c.ArtifactID, c.Version, e.ID); err != nil {
		t.Fatal(err)
	}
	r, err := store.Promotion(ctx, c.ArtifactID, c.Version)
	if err != nil {
		t.Fatal(err)
	}
	if r.BaselineVersion != base.Version || r.CandidateVersion != c.Version || r.EvaluationID != e.ID || len(r.EvaluationRefs) != 1 {
		t.Fatalf("bad promotion record: %+v", r)
	}
}

func TestIMP004RollbackChangesFutureVersionNotHistory(t *testing.T) {
	ctx := context.Background()
	svc, store, base := newTestService(t, 2)
	c, e := prepareCanary(t, svc, base, 2)
	if err := svc.Promote(ctx, c.ArtifactID, c.Version, e.ID); err != nil {
		t.Fatal(err)
	}
	before, err := svc.CaptureInvocation(ctx, "inv-before", map[ArtifactID]uint32{c.ArtifactID: c.Version})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Rollback(ctx, c.ArtifactID, base.Version); err != nil {
		t.Fatal(err)
	}
	active, err := store.ActiveVersion(ctx, c.ArtifactID)
	if err != nil || active != base.Version {
		t.Fatalf("active=%d err=%v", active, err)
	}
	historical, err := store.InvocationManifest(ctx, before.InvocationID)
	if err != nil {
		t.Fatal(err)
	}
	if historical.Artifacts[c.ArtifactID] != c.Version {
		t.Fatalf("history rewritten: %+v", historical)
	}
}

func TestIMP005ShadowCandidateCannotMutate(t *testing.T) {
	ctx := context.Background()
	svc, _, base := newTestService(t, 1)
	c, err := svc.CreateCandidate(ctx, candidateFor(base), base.Version)
	if err != nil {
		t.Fatal(err)
	}
	e := evalFor(base, c, "eval-shadow", 1)
	if err := svc.RecordEvaluation(ctx, e); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Advance(ctx, c.ArtifactID, c.Version, e.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UseCandidate(ctx, c.ArtifactID, c.Version, EffectMutating, []string{"repo.read"}); !errs.IsCode(err, errs.CodePermissionDenied) {
		t.Fatalf("expected shadow mutation denial, got %v", err)
	}
	if _, err := svc.UseCandidate(ctx, c.ArtifactID, c.Version, EffectReadOnly, []string{"repo.read"}); err != nil {
		t.Fatal(err)
	}
}

func TestIMP006SecurityRegressionBlocksPromotionDespiteQualityGain(t *testing.T) {
	ctx := context.Background()
	svc, store, base := newTestService(t, 1)
	c, err := svc.CreateCandidate(ctx, candidateFor(base), base.Version)
	if err != nil {
		t.Fatal(err)
	}
	e := evalFor(base, c, "eval-security", 1)
	e.Outcomes[0].QualityDelta = 100
	e.Outcomes[0].SecurityViolation = true
	if err := svc.RecordEvaluation(ctx, e); err != nil {
		t.Fatal(err)
	}
	st, err := svc.Advance(ctx, c.ArtifactID, c.Version, e.ID)
	if st != StatusRejected || !errs.IsCode(err, errs.CodePermissionDenied) {
		t.Fatalf("expected hard rejection, got %s %v", st, err)
	}
	v, _ := store.Version(ctx, c.ArtifactID, c.Version)
	if v.Status != StatusRejected {
		t.Fatalf("status=%s", v.Status)
	}
}

func TestIMP007ProjectScopeCannotBecomeGlobal(t *testing.T) {
	ctx := context.Background()
	svc, _, base := newTestService(t, 1)
	c := candidateFor(base)
	c.Scope = Scope{Level: ScopeGlobal}
	if _, err := svc.CreateCandidate(ctx, c, base.Version); !errs.IsCode(err, errs.CodePermissionDenied) {
		t.Fatalf("expected scope denial, got %v", err)
	}
}

func TestIMP008InvocationManifestIsExactAndImmutable(t *testing.T) {
	ctx := context.Background()
	svc, store, base := newTestService(t, 1)
	m, err := svc.CaptureInvocation(ctx, "inv-1", map[ArtifactID]uint32{base.ArtifactID: base.Version})
	if err != nil {
		t.Fatal(err)
	}
	if m.ManifestHash == "" {
		t.Fatal("missing manifest hash")
	}
	if _, err := svc.CaptureInvocation(ctx, "inv-1", map[ArtifactID]uint32{base.ArtifactID: base.Version}); !errs.IsCode(err, errs.CodeConflict) {
		t.Fatalf("expected immutable conflict, got %v", err)
	}
	got, err := store.InvocationManifest(ctx, "inv-1")
	if err != nil || got.Artifacts[base.ArtifactID] != base.Version {
		t.Fatalf("manifest: %+v %v", got, err)
	}
}

func TestIMP009InsufficientEvidenceCannotAutoPromote(t *testing.T) {
	ctx := context.Background()
	svc, _, base := newTestService(t, 5)
	c, e := prepareCanary(t, svc, base, 2)
	if err := svc.Promote(ctx, c.ArtifactID, c.Version, e.ID); !errs.IsCode(err, errs.CodePermissionDenied) {
		t.Fatalf("expected evidence denial, got %v", err)
	}
}

func TestIMP010KillSwitchBlocksFutureCandidateUseImmediately(t *testing.T) {
	ctx := context.Background()
	svc, _, base := newTestService(t, 1)
	c, err := svc.CreateCandidate(ctx, candidateFor(base), base.Version)
	if err != nil {
		t.Fatal(err)
	}
	e := evalFor(base, c, "eval-kill", 1)
	if err := svc.RecordEvaluation(ctx, e); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Advance(ctx, c.ArtifactID, c.Version, e.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetKillSwitch(ctx, KillSwitch{Versions: map[string]bool{c.Key(): true}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UseCandidate(ctx, c.ArtifactID, c.Version, EffectReadOnly, []string{"repo.read"}); !errs.IsCode(err, errs.CodePermissionDenied) {
		t.Fatalf("expected kill switch denial, got %v", err)
	}
}

func TestCanarySampleCapIsHardBound(t *testing.T) {
	ctx := context.Background()
	svc, _, base := newTestService(t, 1)
	c, _ := prepareCanary(t, svc, base, 1)
	for i := 0; i < 2; i++ {
		if _, err := svc.UseCandidate(ctx, c.ArtifactID, c.Version, EffectReadOnly, []string{"repo.read"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.UseCandidate(ctx, c.ArtifactID, c.Version, EffectReadOnly, []string{"repo.read"}); !errs.IsCode(err, errs.CodeResourceExhausted) {
		t.Fatalf("expected cap exhaustion, got %v", err)
	}
}
