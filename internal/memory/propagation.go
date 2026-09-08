package memory

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
)

func (s *Service) startPropagation(ctx context.Context, cause string, roots []PropagationItem) (PropagationJob, error) {
	if len(roots) == 0 {
		return PropagationJob{}, nil
	}
	jobID, err := s.ids.Propagation()
	if err != nil {
		return PropagationJob{}, errs.Wrap(errs.CodeInternal, "memory.propagation.start", "generate propagation id", err)
	}
	now := s.clock.Now().UTC()
	job := PropagationJob{
		ID: jobID, Cause: cause, State: PropagationPending,
		MaxDepth: s.cfg.MaxPropagationDepth, MaxWork: s.cfg.MaxPropagationWork,
		CreatedAt: now, UpdatedAt: now,
	}
	items := make([]PropagationItem, 0, len(roots))
	seen := make(map[string]struct{}, len(roots))
	for _, root := range roots {
		if root.Kind != PropagationEvidence && root.Kind != PropagationBelief {
			return PropagationJob{}, errs.New(errs.CodeInvalidArgument, "memory.propagation.start", "invalid propagation root kind")
		}
		if root.NodeID == "" {
			return PropagationJob{}, errs.New(errs.CodeInvalidArgument, "memory.propagation.start", "empty propagation root")
		}
		key := string(root.Kind) + ":" + root.NodeID
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		root.JobID = jobID
		root.Depth = 0
		root.Processed = false
		root.EnqueuedAt = now
		root.ProcessedAt = nil
		items = append(items, root)
	}
	if err := s.repo.CreatePropagationJob(ctx, job, items); err != nil {
		return PropagationJob{}, err
	}
	return job, nil
}

func (s *Service) RunPropagationBatch(ctx context.Context, jobID id.PropagationID) (PropagationJob, error) {
	job, err := s.repo.PropagationJob(ctx, jobID)
	if err != nil {
		return PropagationJob{}, err
	}
	if job.State == PropagationDone || job.State == PropagationLimited || job.State == PropagationFailed {
		return job, nil
	}
	remaining := job.MaxWork - job.ProcessedCount
	if remaining <= 0 {
		job.State = PropagationLimited
		job.UpdatedAt = s.clock.Now().UTC()
		job.LastError = ErrPropagationCap.Error()
		if err := s.repo.UpdatePropagationJob(ctx, job); err != nil {
			return PropagationJob{}, err
		}
		return job, ErrPropagationCap
	}
	limit := s.cfg.PropagationBatchSize
	if limit > remaining {
		limit = remaining
	}
	items, err := s.repo.PendingPropagationItems(ctx, jobID, limit)
	if err != nil {
		return PropagationJob{}, err
	}
	if len(items) == 0 {
		job.State = PropagationDone
		job.UpdatedAt = s.clock.Now().UTC()
		job.LastError = ""
		if err := s.repo.UpdatePropagationJob(ctx, job); err != nil {
			return PropagationJob{}, err
		}
		return job, nil
	}
	if job.State == PropagationPending {
		job.State = PropagationRunning
		job.UpdatedAt = s.clock.Now().UTC()
		if err := s.repo.UpdatePropagationJob(ctx, job); err != nil {
			return PropagationJob{}, err
		}
	}

	for _, item := range items {
		if item.Depth > job.MaxDepth {
			job.State = PropagationLimited
			job.LastError = fmt.Sprintf("propagation depth %d exceeds max %d", item.Depth, job.MaxDepth)
			job.UpdatedAt = s.clock.Now().UTC()
			_ = s.repo.UpdatePropagationJob(ctx, job)
			return job, ErrPropagationCap
		}
		children, processErr := s.processPropagationItem(ctx, job, item)
		if processErr != nil {
			job.State = PropagationFailed
			job.LastError = processErr.Error()
			job.UpdatedAt = s.clock.Now().UTC()
			_ = s.repo.UpdatePropagationJob(ctx, job)
			return job, processErr
		}
		if len(children) > 0 {
			if err := s.repo.EnqueuePropagationItems(ctx, children); err != nil {
				job.State = PropagationFailed
				job.LastError = err.Error()
				job.UpdatedAt = s.clock.Now().UTC()
				_ = s.repo.UpdatePropagationJob(ctx, job)
				return job, err
			}
		}
		now := s.clock.Now().UTC()
		if err := s.repo.MarkPropagationItemProcessed(ctx, jobID, item.Kind, item.NodeID, now); err != nil {
			job.State = PropagationFailed
			job.LastError = err.Error()
			job.UpdatedAt = now
			_ = s.repo.UpdatePropagationJob(ctx, job)
			return job, err
		}
		job.ProcessedCount++
		job.UpdatedAt = now
		if err := s.repo.UpdatePropagationJob(ctx, job); err != nil {
			return PropagationJob{}, err
		}
		if job.ProcessedCount >= job.MaxWork {
			more, pendingErr := s.repo.PendingPropagationItems(ctx, jobID, 1)
			if pendingErr != nil {
				return PropagationJob{}, pendingErr
			}
			if len(more) > 0 {
				job.State = PropagationLimited
				job.LastError = ErrPropagationCap.Error()
				job.UpdatedAt = s.clock.Now().UTC()
				_ = s.repo.UpdatePropagationJob(ctx, job)
				return job, ErrPropagationCap
			}
		}
	}

	more, err := s.repo.PendingPropagationItems(ctx, jobID, 1)
	if err != nil {
		return PropagationJob{}, err
	}
	if len(more) == 0 {
		job.State = PropagationDone
		job.LastError = ""
	} else {
		job.State = PropagationRunning
	}
	job.UpdatedAt = s.clock.Now().UTC()
	if err := s.repo.UpdatePropagationJob(ctx, job); err != nil {
		return PropagationJob{}, err
	}
	return job, nil
}

func (s *Service) DrainPropagation(ctx context.Context, jobID id.PropagationID) (PropagationJob, error) {
	for {
		job, err := s.RunPropagationBatch(ctx, jobID)
		if err != nil {
			return job, err
		}
		if job.State == PropagationDone || job.State == PropagationLimited || job.State == PropagationFailed {
			return job, nil
		}
	}
}

func (s *Service) processPropagationItem(ctx context.Context, job PropagationJob, item PropagationItem) ([]PropagationItem, error) {
	switch item.Kind {
	case PropagationEvidence:
		return s.propagateEvidence(ctx, job, item)
	case PropagationBelief:
		return s.propagateBelief(ctx, job, item)
	default:
		return nil, errs.New(errs.CodeCorruption, "memory.propagation.process", "unknown node kind")
	}
}

func (s *Service) propagateEvidence(ctx context.Context, job PropagationJob, item PropagationItem) ([]PropagationItem, error) {
	evidenceID := id.EvidenceID(item.NodeID)
	invalidation, err := s.repo.EvidenceInvalidation(ctx, evidenceID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	if invalidation == nil {
		return nil, nil
	}
	links, err := s.repo.EvidenceDependents(ctx, evidenceID)
	if err != nil {
		return nil, err
	}
	children := make([]PropagationItem, 0, len(links))
	for _, link := range links {
		belief, err := s.repo.Belief(ctx, link.BeliefID)
		if err != nil {
			return nil, err
		}
		if belief.Status.HistoricalOnly() {
			continue
		}
		target := belief.Status
		profile := belief.Confidence
		profile.Freshness = FreshnessStale
		switch link.Relation {
		case EvidenceSupports, EvidenceVerifies, EvidenceSupersedesSource:
			if belief.Status == BeliefActive {
				target = BeliefStale
			} else {
				target = BeliefNeedsReview
			}
		case EvidenceContradicts:
			if belief.Status == BeliefContested {
				target = BeliefNeedsReview
			}
			profile.ConflictLevel = ConflictPossible
		case EvidenceContextualizes, EvidenceWeaklySuggests:
			if belief.Status == BeliefActive {
				target = BeliefNeedsReview
			}
		}
		changed, err := s.transitionWithProfile(ctx, belief, target, profile, "upstream evidence invalidated: "+invalidation.Reason, job.ID)
		if err != nil {
			return nil, err
		}
		if changed {
			children = append(children, PropagationItem{JobID: job.ID, Kind: PropagationBelief, NodeID: belief.ID.String(), Depth: item.Depth + 1, EnqueuedAt: s.clock.Now().UTC()})
		}
	}
	return children, nil
}

func (s *Service) propagateBelief(ctx context.Context, job PropagationJob, item PropagationItem) ([]PropagationItem, error) {
	sourceID := id.BeliefID(item.NodeID)
	source, err := s.repo.Belief(ctx, sourceID)
	if err != nil {
		return nil, err
	}
	edges, err := s.repo.BeliefOutgoing(ctx, sourceID)
	if err != nil {
		return nil, err
	}
	children := make([]PropagationItem, 0, len(edges))
	for _, edge := range edges {
		if edge.Relation == BeliefSupersedes {
			continue
		}
		target, err := s.repo.Belief(ctx, edge.To)
		if err != nil {
			return nil, err
		}
		if target.Status.HistoricalOnly() {
			continue
		}
		next := target.Status
		profile := target.Confidence
		reason := "upstream belief " + source.ID.String() + " changed to " + string(source.Status)
		switch edge.Relation {
		case BeliefDerivedFrom, BeliefRequires:
			if source.Status != BeliefActive {
				next = BeliefNeedsReview
				profile.Freshness = FreshnessStale
			}
		case BeliefSupports, BeliefRefines:
			if source.Status == BeliefInvalidated || source.Status == BeliefSuperseded || source.Status == BeliefRejected {
				next = BeliefNeedsReview
				profile.Freshness = FreshnessStale
			} else if source.Status == BeliefStale || source.Status == BeliefNeedsReview {
				next = BeliefStale
				profile.Freshness = FreshnessStale
			} else if source.Status == BeliefContested {
				next = BeliefNeedsReview
				profile.ConflictLevel = ConflictPossible
			}
		case BeliefContradicts:
			if source.Status == BeliefInvalidated || source.Status == BeliefSuperseded || source.Status == BeliefRejected {
				if target.Status == BeliefContested {
					next = BeliefNeedsReview
					profile.ConflictLevel = ConflictPossible
				}
			} else if source.Status == BeliefActive || source.Status == BeliefContested {
				next = BeliefContested
				profile.ConflictLevel = ConflictVerified
			}
		}
		changed, err := s.transitionWithProfile(ctx, target, next, profile, reason, job.ID)
		if err != nil {
			return nil, err
		}
		if changed {
			children = append(children, PropagationItem{JobID: job.ID, Kind: PropagationBelief, NodeID: target.ID.String(), Depth: item.Depth + 1, EnqueuedAt: s.clock.Now().UTC()})
		}
	}
	return children, nil
}

func (s *Service) transitionWithProfile(ctx context.Context, belief Belief, target BeliefStatus, profile ConfidenceProfile, reason string, jobID id.PropagationID) (bool, error) {
	if belief.Status == target && belief.Confidence == profile {
		return false, nil
	}
	if belief.Status != target && !allowedTransition(belief.Status, target) {
		return false, nil
	}
	now := s.clock.Now().UTC()
	_, err := s.repo.TransitionBelief(ctx, belief.ID, belief.Status, target, profile, nil, reason, jobID, now)
	if err != nil {
		return false, err
	}
	return true, nil
}

func propagationKey(kind PropagationNodeKind, nodeID string) string {
	return string(kind) + ":" + nodeID
}

func parsePropagationDepth(value string) (int, error) {
	depth, err := strconv.Atoi(value)
	if err != nil {
		return 0, err
	}
	if depth < 0 {
		return 0, errors.New("negative propagation depth")
	}
	return depth, nil
}

var _ = time.RFC3339Nano
