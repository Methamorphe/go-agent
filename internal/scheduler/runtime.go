package scheduler

import (
	"errors"
	"sync/atomic"
	"time"
)

type MetricsSnapshot struct {
	Decisions      uint64 `json:"decisions"`
	NoEligible     uint64 `json:"no_eligible"`
	BudgetRejected uint64 `json:"budget_rejected"`
	SlotRejected   uint64 `json:"slot_rejected"`
	FallbackRoutes uint64 `json:"fallback_routes"`
	Completed      uint64 `json:"completed"`
	Failed         uint64 `json:"failed"`
}

type Metrics struct {
	decisions      atomic.Uint64
	noEligible     atomic.Uint64
	budgetRejected atomic.Uint64
	slotRejected   atomic.Uint64
	fallbackRoutes atomic.Uint64
	completed      atomic.Uint64
	failed         atomic.Uint64
}

func (m *Metrics) Snapshot() MetricsSnapshot {
	if m == nil {
		return MetricsSnapshot{}
	}
	return MetricsSnapshot{
		Decisions:      m.decisions.Load(),
		NoEligible:     m.noEligible.Load(),
		BudgetRejected: m.budgetRejected.Load(),
		SlotRejected:   m.slotRejected.Load(),
		FallbackRoutes: m.fallbackRoutes.Load(),
		Completed:      m.completed.Load(),
		Failed:         m.failed.Load(),
	}
}

type Runtime struct {
	Router  *Router
	Budgets *BudgetLedger
	Slots   *Slots
	Metrics *Metrics
}

func NewRuntime(router *Router, budgets *BudgetLedger, slots *Slots) *Runtime {
	if router == nil {
		router = NewRouter(nil, nil, nil)
	}
	if budgets == nil {
		budgets = NewBudgetLedger()
	}
	if slots == nil {
		slots = NewSlots(SlotConfig{})
	}
	return &Runtime{Router: router, Budgets: budgets, Slots: slots, Metrics: &Metrics{}}
}

type Attempt struct {
	Decision      RoutingDecision
	Profile       ModelProfile
	ReservationID ReservationID
	Reserved      Resources
	Lease         *SlotLease
	startedAt     time.Time
	finished      atomic.Bool
}

func (r *Runtime) Prepare(task CognitiveTask, excluded map[string]struct{}) (*Attempt, error) {
	if r == nil || r.Router == nil || r.Budgets == nil || r.Slots == nil {
		return nil, errors.New("scheduler runtime is not configured")
	}
	if excluded == nil {
		excluded = make(map[string]struct{})
	}
	workExcluded := make(map[string]struct{}, len(excluded)+4)
	for key := range excluded {
		workExcluded[key] = struct{}{}
	}
	maxAttempts := len(r.Router.Registry().Snapshot())
	if maxAttempts == 0 {
		maxAttempts = 1
	}
	for i := 0; i < maxAttempts; i++ {
		decision, err := r.Router.Route(task, workExcluded)
		if err != nil {
			if errors.Is(err, ErrNoEligibleModel) {
				r.Metrics.noEligible.Add(1)
			}
			return nil, err
		}
		r.Metrics.decisions.Add(1)
		profile, ok := r.Router.Registry().Get(decision.Selected)
		if !ok {
			workExcluded[decision.Selected.Key()] = struct{}{}
			continue
		}
		reservationID, err := r.Budgets.Reserve(task.RootAgentID, decision.EstimatedCost)
		if err != nil {
			if errors.Is(err, ErrBudgetExhausted) {
				r.Metrics.budgetRejected.Add(1)
				workExcluded[profile.Key()] = struct{}{}
				continue
			}
			return nil, err
		}
		lease, err := r.Slots.Acquire(task.RootAgentID, profile.ProviderID, profile.MaxConcurrency)
		if err != nil {
			_ = r.Budgets.Release(reservationID)
			if errors.Is(err, ErrSlotsExhausted) {
				r.Metrics.slotRejected.Add(1)
				workExcluded[profile.Key()] = struct{}{}
				continue
			}
			return nil, err
		}
		if len(excluded) > 0 || len(workExcluded) > len(excluded) {
			r.Metrics.fallbackRoutes.Add(1)
		}
		return &Attempt{
			Decision: decision, Profile: profile, ReservationID: reservationID,
			Reserved: decision.EstimatedCost, Lease: lease, startedAt: r.Router.clock.Now().UTC(),
		}, nil
	}
	r.Metrics.noEligible.Add(1)
	return nil, &NoEligibleError{}
}

func (r *Runtime) Finish(attempt *Attempt, actual Resources, observation ObservationKind) error {
	if attempt == nil {
		return errors.New("attempt is nil")
	}
	if !attempt.finished.CompareAndSwap(false, true) {
		return ErrReservationSettled
	}
	if attempt.Lease != nil {
		attempt.Lease.Release()
	}
	latency := r.Router.clock.Now().UTC().Sub(attempt.startedAt)
	r.Router.Telemetry().Observe(attempt.Profile.Ref(), observation, latency)
	if observation == ObservationSuccess {
		r.Metrics.completed.Add(1)
	} else {
		r.Metrics.failed.Add(1)
	}
	return r.Budgets.Settle(attempt.ReservationID, actual)
}

func (r *Runtime) Release(attempt *Attempt, observation ObservationKind) error {
	if attempt == nil {
		return errors.New("attempt is nil")
	}
	if !attempt.finished.CompareAndSwap(false, true) {
		return ErrReservationSettled
	}
	if attempt.Lease != nil {
		attempt.Lease.Release()
	}
	latency := r.Router.clock.Now().UTC().Sub(attempt.startedAt)
	r.Router.Telemetry().Observe(attempt.Profile.Ref(), observation, latency)
	r.Metrics.failed.Add(1)
	return r.Budgets.Release(attempt.ReservationID)
}
