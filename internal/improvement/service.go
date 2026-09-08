package improvement

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Methamorphe/go-agent/internal/errs"
)

type Config struct {
	MinEvaluationTasks          int
	RequireIndependentEvaluator bool
	Now                         func() time.Time
}

type Service struct {
	store Store
	cfg   Config
}

func NewService(store Store, cfg Config) *Service {
	if cfg.MinEvaluationTasks <= 0 {
		cfg.MinEvaluationTasks = 5
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Service{store: store, cfg: cfg}
}

func (s *Service) RegisterBaseline(ctx context.Context, v ArtifactVersion) (ArtifactVersion, error) {
	const op = "improvement.register_baseline"
	if err := validateVersion(v, false); err != nil {
		return ArtifactVersion{}, errs.Wrap(errs.CodeInvalidArgument, op, "invalid baseline", err)
	}
	versions, err := s.store.Versions(ctx, v.ArtifactID)
	if err == nil && len(versions) > 0 {
		return ArtifactVersion{}, errs.New(errs.CodeConflict, op, "artifact already exists")
	}
	if err != nil && !errs.IsCode(err, errs.CodeNotFound) {
		return ArtifactVersion{}, err
	}
	v.Version = 1
	v.ParentVersion = 0
	v.Hypothesis = nil
	v.Status = StatusPromoted
	v.RequiredCapabilities = normalizeCapabilities(v.RequiredCapabilities)
	v.CreatedAt = s.cfg.Now().UTC()
	if err := s.store.CreateVersion(ctx, v); err != nil {
		return ArtifactVersion{}, err
	}
	rec := PromotionRecord{ArtifactID: v.ArtifactID, CandidateVersion: 1, PromotedAt: v.CreatedAt}
	if err := s.store.Promote(ctx, rec); err != nil {
		return ArtifactVersion{}, err
	}
	return v, nil
}

func (s *Service) CreateCandidate(ctx context.Context, candidate ArtifactVersion, baselineVersion uint32) (ArtifactVersion, error) {
	const op = "improvement.create_candidate"
	if err := validateVersion(candidate, true); err != nil {
		return ArtifactVersion{}, errs.Wrap(errs.CodeInvalidArgument, op, "invalid candidate", err)
	}
	baseline, err := s.store.Version(ctx, candidate.ArtifactID, baselineVersion)
	if err != nil {
		return ArtifactVersion{}, err
	}
	if baseline.Status != StatusPromoted {
		return ArtifactVersion{}, errs.New(errs.CodeConflict, op, "baseline is not promoted")
	}
	if candidate.Kind != baseline.Kind {
		return ArtifactVersion{}, errs.New(errs.CodeInvalidArgument, op, "candidate kind differs from baseline")
	}
	if !candidate.Scope.Equal(baseline.Scope) {
		return ArtifactVersion{}, errs.New(errs.CodePermissionDenied, op, "candidate scope cannot widen or silently change")
	}
	versions, err := s.store.Versions(ctx, candidate.ArtifactID)
	if err != nil {
		return ArtifactVersion{}, err
	}
	var max uint32
	for _, v := range versions {
		if v.Version > max {
			max = v.Version
		}
	}
	candidate.Version = max + 1
	candidate.ParentVersion = baseline.Version
	candidate.Status = StatusCandidate
	candidate.RequiredCapabilities = normalizeCapabilities(candidate.RequiredCapabilities)
	candidate.CreatedAt = s.cfg.Now().UTC()
	if err := s.store.CreateVersion(ctx, candidate); err != nil {
		return ArtifactVersion{}, err
	}
	return candidate, nil
}

func (s *Service) RecordEvaluation(ctx context.Context, e Evaluation) error {
	const op = "improvement.record_evaluation"
	if strings.TrimSpace(string(e.ID)) == "" || strings.TrimSpace(e.CorpusRef) == "" || strings.TrimSpace(e.EvaluatorRef) == "" {
		return errs.New(errs.CodeInvalidArgument, op, "evaluation id, corpus and evaluator are required")
	}
	if e.BaselineVersion == 0 || e.CandidateVersion == 0 || e.BaselineVersion == e.CandidateVersion {
		return errs.New(errs.CodeInvalidArgument, op, "distinct baseline and candidate versions are required")
	}
	baseline, err := s.store.Version(ctx, e.ArtifactID, e.BaselineVersion)
	if err != nil {
		return err
	}
	candidate, err := s.store.Version(ctx, e.ArtifactID, e.CandidateVersion)
	if err != nil {
		return err
	}
	if baseline.Status != StatusPromoted {
		return errs.New(errs.CodeConflict, op, "evaluation baseline is not promoted")
	}
	if candidate.Status != StatusCandidate && candidate.Status != StatusEvaluating {
		return errs.New(errs.CodeConflict, op, "candidate is not evaluable")
	}
	if candidate.ParentVersion != baseline.Version {
		return errs.New(errs.CodeConflict, op, "evaluation baseline does not match candidate parent")
	}
	if len(e.Outcomes) == 0 {
		return errs.New(errs.CodeInvalidArgument, op, "evaluation outcomes are required")
	}
	seen := map[string]struct{}{}
	for _, o := range e.Outcomes {
		if strings.TrimSpace(o.TaskRef) == "" {
			return errs.New(errs.CodeInvalidArgument, op, "every outcome needs a task ref")
		}
		if _, ok := seen[o.TaskRef]; ok {
			return errs.New(errs.CodeInvalidArgument, op, "duplicate task ref would contaminate evaluation")
		}
		seen[o.TaskRef] = struct{}{}
	}
	e.CreatedAt = s.cfg.Now().UTC()
	if err := s.store.PutEvaluation(ctx, e); err != nil {
		return err
	}
	return s.store.SetStatus(ctx, e.ArtifactID, e.CandidateVersion, StatusEvaluating)
}

func (s *Service) Advance(ctx context.Context, artifactID ArtifactID, version uint32, evaluationID EvaluationID) (ArtifactStatus, error) {
	const op = "improvement.advance"
	candidate, err := s.store.Version(ctx, artifactID, version)
	if err != nil {
		return "", err
	}
	if err := s.ensureEnabled(ctx, candidate); err != nil {
		return "", err
	}
	eval, err := s.store.Evaluation(ctx, evaluationID)
	if err != nil {
		return "", err
	}
	if eval.ArtifactID != artifactID || eval.CandidateVersion != version {
		return "", errs.New(errs.CodeInvalidArgument, op, "evaluation does not belong to candidate")
	}
	if eval.HardRegression() {
		_ = s.store.SetStatus(ctx, artifactID, version, StatusRejected)
		return StatusRejected, errs.New(errs.CodePermissionDenied, op, "hard non-regression gate failed")
	}
	next := ArtifactStatus("")
	switch candidate.Status {
	case StatusEvaluating:
		next = StatusShadow
	case StatusShadow:
		if candidate.CanarySampleCap == 0 {
			return "", errs.New(errs.CodeInvalidArgument, op, "canary requires a positive sample cap")
		}
		next = StatusCanary
	default:
		return "", errs.New(errs.CodeConflict, op, "candidate cannot advance from current state")
	}
	if err := s.store.SetStatus(ctx, artifactID, version, next); err != nil {
		return "", err
	}
	return next, nil
}

func (s *Service) Promote(ctx context.Context, artifactID ArtifactID, version uint32, evaluationID EvaluationID) error {
	const op = "improvement.promote"
	candidate, err := s.store.Version(ctx, artifactID, version)
	if err != nil {
		return err
	}
	if candidate.Status != StatusCanary {
		return errs.New(errs.CodeConflict, op, "candidate must complete canary before promotion")
	}
	if err := s.ensureEnabled(ctx, candidate); err != nil {
		return err
	}
	eval, err := s.store.Evaluation(ctx, evaluationID)
	if err != nil {
		return err
	}
	if eval.ArtifactID != artifactID || eval.CandidateVersion != version || eval.BaselineVersion != candidate.ParentVersion {
		return errs.New(errs.CodeInvalidArgument, op, "evaluation does not match candidate/baseline")
	}
	if eval.HardRegression() {
		_ = s.store.SetStatus(ctx, artifactID, version, StatusRejected)
		return errs.New(errs.CodePermissionDenied, op, "security or verification regression blocks promotion")
	}
	if eval.UniqueTasks() < s.cfg.MinEvaluationTasks {
		return errs.New(errs.CodePermissionDenied, op, fmt.Sprintf("insufficient evaluation evidence: got %d unique tasks, need %d", eval.UniqueTasks(), s.cfg.MinEvaluationTasks))
	}
	if s.cfg.RequireIndependentEvaluator && !eval.Independent {
		return errs.New(errs.CodePermissionDenied, op, "independent evaluation is required")
	}
	wins, losses, _ := eval.WinLossTie()
	if wins < losses {
		return errs.New(errs.CodePermissionDenied, op, "candidate regresses baseline task success")
	}
	active, err := s.store.ActiveVersion(ctx, artifactID)
	if err != nil {
		return err
	}
	if active != candidate.ParentVersion {
		return errs.New(errs.CodeConflict, op, "active baseline changed during evaluation")
	}
	rec := PromotionRecord{ArtifactID: artifactID, BaselineVersion: candidate.ParentVersion, CandidateVersion: version, EvaluationID: evaluationID, EvaluationRefs: append([]string(nil), eval.EvidenceRefs...), PromotedAt: s.cfg.Now().UTC()}
	return s.store.Promote(ctx, rec)
}

func (s *Service) Rollback(ctx context.Context, artifactID ArtifactID, targetVersion uint32) error {
	const op = "improvement.rollback"
	current, err := s.store.ActiveVersion(ctx, artifactID)
	if err != nil {
		return err
	}
	if current == targetVersion {
		return errs.New(errs.CodeConflict, op, "target is already active")
	}
	target, err := s.store.Version(ctx, artifactID, targetVersion)
	if err != nil {
		return err
	}
	if target.Status != StatusDeprecated && target.Status != StatusPromoted {
		return errs.New(errs.CodeConflict, op, "rollback target was not a previously promoted version")
	}
	return s.store.Rollback(ctx, artifactID, current, targetVersion, s.cfg.Now().UTC())
}

func (s *Service) Resolve(ctx context.Context, artifactID ArtifactID, granted []string) (ArtifactVersion, error) {
	version, err := s.store.ActiveVersion(ctx, artifactID)
	if err != nil {
		return ArtifactVersion{}, err
	}
	v, err := s.store.Version(ctx, artifactID, version)
	if err != nil {
		return ArtifactVersion{}, err
	}
	if err := s.ensureEnabled(ctx, v); err != nil {
		return ArtifactVersion{}, err
	}
	if !hasRequired(v.RequiredCapabilities, granted) {
		return ArtifactVersion{}, errs.New(errs.CodePermissionDenied, "improvement.resolve", "artifact requires capabilities the process was not granted")
	}
	return v, nil
}

func (s *Service) UseCandidate(ctx context.Context, artifactID ArtifactID, version uint32, effect EffectClass, granted []string) (ArtifactVersion, error) {
	const op = "improvement.use_candidate"
	v, err := s.store.Version(ctx, artifactID, version)
	if err != nil {
		return ArtifactVersion{}, err
	}
	if err := s.ensureEnabled(ctx, v); err != nil {
		return ArtifactVersion{}, err
	}
	if !hasRequired(v.RequiredCapabilities, granted) {
		return ArtifactVersion{}, errs.New(errs.CodePermissionDenied, op, "candidate requirements exceed runtime capability grants")
	}
	switch v.Status {
	case StatusShadow:
		if effect == EffectMutating {
			return ArtifactVersion{}, errs.New(errs.CodePermissionDenied, op, "shadow candidate cannot execute mutating effects")
		}
	case StatusCanary:
		if err := s.store.ReserveCanaryUse(ctx, artifactID, version, v.CanarySampleCap); err != nil {
			return ArtifactVersion{}, err
		}
	default:
		return ArtifactVersion{}, errs.New(errs.CodeConflict, op, "candidate is not in shadow or canary")
	}
	return v, nil
}

func (s *Service) CaptureInvocation(ctx context.Context, invocationID string, active map[ArtifactID]uint32) (InvocationManifest, error) {
	const op = "improvement.capture_invocation"
	if strings.TrimSpace(invocationID) == "" {
		return InvocationManifest{}, errs.New(errs.CodeInvalidArgument, op, "invocation id is required")
	}
	keys := make([]string, 0, len(active))
	for id, v := range active {
		if _, err := s.store.Version(ctx, id, v); err != nil {
			return InvocationManifest{}, err
		}
		keys = append(keys, fmt.Sprintf("%s=%d", id, v))
	}
	sort.Strings(keys)
	sum := sha256.Sum256([]byte(strings.Join(keys, "\n")))
	copyMap := make(map[ArtifactID]uint32, len(active))
	for k, v := range active {
		copyMap[k] = v
	}
	m := InvocationManifest{InvocationID: invocationID, Artifacts: copyMap, ManifestHash: hex.EncodeToString(sum[:]), CapturedAt: s.cfg.Now().UTC()}
	if err := s.store.BindInvocation(ctx, m); err != nil {
		return InvocationManifest{}, err
	}
	return m, nil
}

func (s *Service) SetKillSwitch(ctx context.Context, k KillSwitch) error {
	if k.Families == nil {
		k.Families = map[ArtifactKind]bool{}
	}
	if k.Versions == nil {
		k.Versions = map[string]bool{}
	}
	return s.store.SetKillSwitch(ctx, k)
}
func (s *Service) ensureEnabled(ctx context.Context, v ArtifactVersion) error {
	k, err := s.store.KillSwitch(ctx)
	if err != nil {
		return err
	}
	if k.Blocks(v) {
		return errs.New(errs.CodePermissionDenied, "improvement.kill_switch", "artifact use disabled by runtime kill switch")
	}
	return nil
}

func validateVersion(v ArtifactVersion, candidate bool) error {
	if strings.TrimSpace(string(v.ArtifactID)) == "" {
		return fmt.Errorf("artifact id is required")
	}
	if !v.Kind.Valid() {
		return fmt.Errorf("unsupported artifact kind %q", v.Kind)
	}
	if strings.TrimSpace(v.ContentRef) == "" || strings.TrimSpace(v.OriginRef) == "" {
		return fmt.Errorf("content and origin refs are required")
	}
	if !v.Scope.Valid() {
		return fmt.Errorf("invalid artifact scope")
	}
	if candidate && (v.Hypothesis == nil || !v.Hypothesis.Valid()) {
		return fmt.Errorf("candidate requires explicit hypothesis")
	}
	return nil
}
