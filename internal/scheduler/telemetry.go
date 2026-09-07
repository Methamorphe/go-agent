package scheduler

import (
	"sync"
	"time"
)

type ObservationKind string

const (
	ObservationSuccess          ObservationKind = "success"
	ObservationTransientFailure ObservationKind = "transient_failure"
	ObservationRateLimited      ObservationKind = "rate_limited"
	ObservationPermanentFailure ObservationKind = "permanent_failure"
)

type TelemetrySnapshot struct {
	State               HealthState   `json:"state"`
	LatencyEWMA         time.Duration `json:"latency_ewma"`
	ErrorEWMA           float64       `json:"error_ewma"`
	QueueDepth          int           `json:"queue_depth"`
	ConsecutiveFailures int           `json:"consecutive_failures"`
	OpenUntil           time.Time     `json:"open_until,omitempty"`
	Version             uint64        `json:"version"`
}
type telemetryEntry struct{ snap TelemetrySnapshot }
type Telemetry struct {
	mu               sync.Mutex
	entries          map[string]telemetryEntry
	max              int
	clock            Clock
	failureThreshold int
	cooldown         time.Duration
}

func NewTelemetry(max int, clock Clock) *Telemetry {
	if max <= 0 {
		max = DefaultMaxProfiles
	}
	if clock == nil {
		clock = systemClock{}
	}
	return &Telemetry{entries: make(map[string]telemetryEntry), max: max, clock: clock, failureThreshold: 3, cooldown: 30 * time.Second}
}
func (t *Telemetry) ensure(key string) telemetryEntry {
	e, ok := t.entries[key]
	if !ok {
		e.snap.State = HealthHealthy
	}
	return e
}
func (t *Telemetry) Snapshot(ref ModelRef) TelemetrySnapshot {
	t.mu.Lock()
	defer t.mu.Unlock()
	e := t.ensure(ref.Key())
	now := t.clock.Now().UTC()
	if (e.snap.State == HealthUnavailable || e.snap.State == HealthRateLimited) && !e.snap.OpenUntil.IsZero() && !now.Before(e.snap.OpenUntil) {
		e.snap.State = HealthRecovering
		e.snap.Version++
		t.entries[ref.Key()] = e
	}
	return e.snap
}
func (t *Telemetry) SetQueueDepth(ref ModelRef, depth int) {
	if depth < 0 {
		depth = 0
	}
	t.mu.Lock()
	key := ref.Key()
	_, exists := t.entries[key]
	if !exists && len(t.entries) >= t.max {
		t.mu.Unlock()
		return
	}
	e := t.ensure(key)
	e.snap.QueueDepth = depth
	e.snap.Version++
	t.entries[key] = e
	t.mu.Unlock()
}
func (t *Telemetry) Observe(ref ModelRef, kind ObservationKind, latency time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	key := ref.Key()
	e := t.ensure(key)
	if len(t.entries) >= t.max {
		if _, ok := t.entries[key]; !ok {
			return
		}
	}
	if latency > 0 {
		if e.snap.LatencyEWMA == 0 {
			e.snap.LatencyEWMA = latency
		} else {
			e.snap.LatencyEWMA = time.Duration(float64(e.snap.LatencyEWMA)*0.8 + float64(latency)*0.2)
		}
	}
	errSample := 0.0
	if kind != ObservationSuccess {
		errSample = 1
	}
	e.snap.ErrorEWMA = e.snap.ErrorEWMA*0.8 + errSample*0.2
	now := t.clock.Now().UTC()
	switch kind {
	case ObservationSuccess:
		e.snap.ConsecutiveFailures = 0
		if e.snap.State == HealthRecovering || e.snap.State == HealthDegraded {
			e.snap.State = HealthHealthy
		}
		e.snap.OpenUntil = time.Time{}
	case ObservationRateLimited:
		e.snap.ConsecutiveFailures++
		e.snap.State = HealthRateLimited
		e.snap.OpenUntil = now.Add(t.cooldown)
	case ObservationPermanentFailure:
		e.snap.ConsecutiveFailures++
		e.snap.State = HealthUnavailable
		e.snap.OpenUntil = now.Add(t.cooldown)
	case ObservationTransientFailure:
		e.snap.ConsecutiveFailures++
		if e.snap.ConsecutiveFailures >= t.failureThreshold {
			e.snap.State = HealthUnavailable
			e.snap.OpenUntil = now.Add(t.cooldown)
		} else {
			e.snap.State = HealthDegraded
		}
	}
	e.snap.Version++
	t.entries[key] = e
}
