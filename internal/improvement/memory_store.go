package improvement

import (
	"context"
	"sync"
	"time"

	"github.com/Methamorphe/go-agent/internal/errs"
)

type MemoryStore struct {
	mu          sync.RWMutex
	versions    map[ArtifactID]map[uint32]ArtifactVersion
	evaluations map[EvaluationID]Evaluation
	promotions  map[string]PromotionRecord
	active      map[ArtifactID]uint32
	invocations map[string]InvocationManifest
	kill        KillSwitch
	canary      map[string]uint64
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{versions: map[ArtifactID]map[uint32]ArtifactVersion{}, evaluations: map[EvaluationID]Evaluation{}, promotions: map[string]PromotionRecord{}, active: map[ArtifactID]uint32{}, invocations: map[string]InvocationManifest{}, kill: KillSwitch{Families: map[ArtifactKind]bool{}, Versions: map[string]bool{}}, canary: map[string]uint64{}}
}
func cloneVersion(v ArtifactVersion) ArtifactVersion {
	v.RequiredCapabilities = append([]string(nil), v.RequiredCapabilities...)
	if v.Hypothesis != nil {
		h := *v.Hypothesis
		h.ExpectedRisks = append([]string(nil), h.ExpectedRisks...)
		v.Hypothesis = &h
	}
	return v
}
func cloneEval(e Evaluation) Evaluation {
	e.Outcomes = append([]Outcome(nil), e.Outcomes...)
	e.EvidenceRefs = append([]string(nil), e.EvidenceRefs...)
	e.KnownBias = append([]string(nil), e.KnownBias...)
	return e
}
func (s *MemoryStore) CreateVersion(_ context.Context, v ArtifactVersion) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.versions[v.ArtifactID] == nil {
		s.versions[v.ArtifactID] = map[uint32]ArtifactVersion{}
	}
	if _, ok := s.versions[v.ArtifactID][v.Version]; ok {
		return errs.New(errs.CodeConflict, "improvement.memory.create_version", "artifact version already exists")
	}
	s.versions[v.ArtifactID][v.Version] = cloneVersion(v)
	return nil
}
func (s *MemoryStore) Version(_ context.Context, id ArtifactID, version uint32) (ArtifactVersion, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.versions[id][version]
	if !ok {
		return ArtifactVersion{}, errs.New(errs.CodeNotFound, "improvement.memory.version", "artifact version not found")
	}
	return cloneVersion(v), nil
}
func (s *MemoryStore) Versions(_ context.Context, id ArtifactID) ([]ArtifactVersion, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m := s.versions[id]
	if len(m) == 0 {
		return nil, errs.New(errs.CodeNotFound, "improvement.memory.versions", "artifact not found")
	}
	out := make([]ArtifactVersion, 0, len(m))
	for _, v := range m {
		out = append(out, cloneVersion(v))
	}
	return out, nil
}
func (s *MemoryStore) SetStatus(_ context.Context, id ArtifactID, version uint32, status ArtifactStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.versions[id][version]
	if !ok {
		return errs.New(errs.CodeNotFound, "improvement.memory.set_status", "artifact version not found")
	}
	v.Status = status
	s.versions[id][version] = v
	return nil
}
func (s *MemoryStore) PutEvaluation(_ context.Context, e Evaluation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.evaluations[e.ID]; ok {
		return errs.New(errs.CodeConflict, "improvement.memory.put_evaluation", "evaluation exists")
	}
	s.evaluations[e.ID] = cloneEval(e)
	return nil
}
func (s *MemoryStore) Evaluation(_ context.Context, id EvaluationID) (Evaluation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.evaluations[id]
	if !ok {
		return Evaluation{}, errs.New(errs.CodeNotFound, "improvement.memory.evaluation", "evaluation not found")
	}
	return cloneEval(e), nil
}
func (s *MemoryStore) Promote(_ context.Context, r PromotionRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	candidate, ok := s.versions[r.ArtifactID][r.CandidateVersion]
	if !ok {
		return errs.New(errs.CodeNotFound, "improvement.memory.promote", "candidate not found")
	}
	if r.BaselineVersion == 0 {
		if _, exists := s.active[r.ArtifactID]; exists {
			return errs.New(errs.CodeConflict, "improvement.memory.promote", "artifact already active")
		}
	} else {
		if s.active[r.ArtifactID] != r.BaselineVersion {
			return errs.New(errs.CodeConflict, "improvement.memory.promote", "active baseline changed")
		}
		base := s.versions[r.ArtifactID][r.BaselineVersion]
		base.Status = StatusDeprecated
		s.versions[r.ArtifactID][r.BaselineVersion] = base
	}
	candidate.Status = StatusPromoted
	s.versions[r.ArtifactID][r.CandidateVersion] = candidate
	s.active[r.ArtifactID] = r.CandidateVersion
	r.EvaluationRefs = append([]string(nil), r.EvaluationRefs...)
	s.promotions[VersionKey(r.ArtifactID, r.CandidateVersion)] = r
	return nil
}
func (s *MemoryStore) Promotion(_ context.Context, id ArtifactID, v uint32) (PromotionRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.promotions[VersionKey(id, v)]
	if !ok {
		return PromotionRecord{}, errs.New(errs.CodeNotFound, "improvement.memory.promotion", "promotion not found")
	}
	r.EvaluationRefs = append([]string(nil), r.EvaluationRefs...)
	return r, nil
}
func (s *MemoryStore) ActiveVersion(_ context.Context, id ArtifactID) (uint32, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.active[id]
	if !ok {
		return 0, errs.New(errs.CodeNotFound, "improvement.memory.active", "active artifact not found")
	}
	return v, nil
}
func (s *MemoryStore) Rollback(_ context.Context, id ArtifactID, from, to uint32, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active[id] != from {
		return errs.New(errs.CodeConflict, "improvement.memory.rollback", "active version changed")
	}
	current, ok := s.versions[id][from]
	if !ok {
		return errs.New(errs.CodeNotFound, "improvement.memory.rollback", "current version not found")
	}
	target, ok := s.versions[id][to]
	if !ok {
		return errs.New(errs.CodeNotFound, "improvement.memory.rollback", "target version not found")
	}
	current.Status = StatusRolledBack
	target.Status = StatusPromoted
	s.versions[id][from] = current
	s.versions[id][to] = target
	s.active[id] = to
	return nil
}
func (s *MemoryStore) BindInvocation(_ context.Context, m InvocationManifest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.invocations[m.InvocationID]; ok {
		return errs.New(errs.CodeConflict, "improvement.memory.bind_invocation", "invocation manifest is immutable")
	}
	m.Artifacts = cloneManifestMap(m.Artifacts)
	s.invocations[m.InvocationID] = m
	return nil
}
func (s *MemoryStore) InvocationManifest(_ context.Context, id string) (InvocationManifest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.invocations[id]
	if !ok {
		return InvocationManifest{}, errs.New(errs.CodeNotFound, "improvement.memory.invocation", "invocation manifest not found")
	}
	m.Artifacts = cloneManifestMap(m.Artifacts)
	return m, nil
}
func cloneManifestMap(in map[ArtifactID]uint32) map[ArtifactID]uint32 {
	out := make(map[ArtifactID]uint32, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
func (s *MemoryStore) KillSwitch(_ context.Context) (KillSwitch, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneKill(s.kill), nil
}
func (s *MemoryStore) SetKillSwitch(_ context.Context, k KillSwitch) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.kill = cloneKill(k)
	return nil
}
func cloneKill(k KillSwitch) KillSwitch {
	o := KillSwitch{All: k.All, Families: map[ArtifactKind]bool{}, Versions: map[string]bool{}}
	for x, v := range k.Families {
		o.Families[x] = v
	}
	for x, v := range k.Versions {
		o.Versions[x] = v
	}
	return o
}
func (s *MemoryStore) ReserveCanaryUse(_ context.Context, id ArtifactID, v uint32, cap uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := VersionKey(id, v)
	if cap == 0 || s.canary[key] >= cap {
		return errs.New(errs.CodeResourceExhausted, "improvement.memory.canary", "canary sample cap exhausted")
	}
	s.canary[key]++
	return nil
}

var _ Store = (*MemoryStore)(nil)
