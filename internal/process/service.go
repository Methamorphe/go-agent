package process

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Methamorphe/go-agent/internal/clock"
	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/ledger"
)

const MaxEventPage = 1000

type IDGenerator interface {
	Agent() (id.AgentID, error)
	Event() (id.EventID, error)
	Correlation() (id.CorrelationID, error)
	Request() (id.RequestID, error)
	Intent() (id.IntentID, error)
	Sleep() (id.SleepID, error)
	Lease() (id.LeaseID, error)
	Snapshot() (id.SnapshotID, error)
	Checkpoint() (id.CheckpointID, error)
}

type CommandMeta struct {
	RequestID     id.RequestID
	CorrelationID id.CorrelationID
	CausationID   *id.EventID
	Actor         ledger.ActorRef
}

type Service struct {
	store Store
	ids   IDGenerator
	clock clock.Clock
}

func NewService(
	store Store,
	ids IDGenerator,
	clock clock.Clock,
) *Service {
	return &Service{
		store: store,
		ids:   ids,
		clock: clock,
	}
}

func (s *Service) CreateRoot(
	ctx context.Context,
	goal string,
	meta CommandMeta,
) (State, error) {
	meta, err := s.normalizeMeta(meta)
	if err != nil {
		return State{}, err
	}

	if state, ok, err :=
		s.receiptState(
			ctx,
			meta.RequestID,
			"process.create",
			"",
		); err != nil || ok {
		return state, err
	}

	goal = strings.TrimSpace(goal)

	if goal == "" {
		return State{},
			errs.New(
				errs.CodeInvalidArgument,
				"process.create",
				"root intent goal must not be empty",
			)
	}

	agentID, err := s.ids.Agent()
	if err != nil {
		return State{},
			wrapID(
				"process.create",
				err,
			)
	}

	intentID, err := s.ids.Intent()
	if err != nil {
		return State{},
			wrapID(
				"process.create",
				err,
			)
	}

	now := s.clock.Now().UTC()

	intent := Intent{
		ID:            intentID,
		SchemaVersion: IntentSchemaVersion,
		Goal:          goal,
		CreatedAt:     now,
	}

	return s.create(
		ctx,
		agentID,
		agentID,
		nil,
		0,
		intent,
		meta,
		"process.create",
	)
}

// G1 only establishes lineage.
// Delegated child Task Intent arrives in G5.
func (s *Service) CreateChild(
	ctx context.Context,
	parentID id.AgentID,
	meta CommandMeta,
) (State, error) {
	meta, err := s.normalizeMeta(meta)
	if err != nil {
		return State{}, err
	}

	if state, ok, err :=
		s.receiptState(
			ctx,
			meta.RequestID,
			"process.create_child",
			"",
		); err != nil || ok {
		return state, err
	}

	parent, err :=
		s.store.Current(
			ctx,
			parentID,
		)
	if err != nil {
		return State{}, err
	}

	if parent.Status.Terminal() {
		return State{},
			errs.New(
				errs.CodeConflict,
				"process.create_child",
				"cannot create child from terminal parent",
			)
	}

	if parent.RootIntent == nil {
		return State{},
			errs.New(
				errs.CodeCorruption,
				"process.create_child",
				"parent has no bound root intent",
			)
	}

	if meta.CausationID == nil &&
		parent.LastEventID != "" {
		meta.CausationID =
			eventPtr(
				parent.LastEventID,
			)
	}

	agentID, err := s.ids.Agent()
	if err != nil {
		return State{},
			wrapID(
				"process.create_child",
				err,
			)
	}

	intent := *parent.RootIntent

	return s.create(
		ctx,
		agentID,
		parent.RootAgentID,
		&parent.AgentID,
		parent.LineageDepth+1,
		intent,
		meta,
		"process.create_child",
	)
}

func (s *Service) create(
	ctx context.Context,
	agentID id.AgentID,
	rootID id.AgentID,
	parentID *id.AgentID,
	depth uint32,
	intent Intent,
	meta CommandMeta,
	command string,
) (State, error) {
	now := s.clock.Now().UTC()

	state := State{}

	created, err := s.buildEvent(
		agentID,
		rootID,
		1,
		EventAgentCreated,
		AgentCreatedPayload{
			ParentAgentID: parentID,
			LineageDepth:  depth,
		},
		now,
		meta,
		meta.CausationID,
	)
	if err != nil {
		return State{}, err
	}

	state, err = Apply(
		state,
		created,
	)
	if err != nil {
		return State{}, err
	}

	intentEvent, err := s.buildEvent(
		agentID,
		rootID,
		2,
		EventRootIntentBound,
		RootIntentBoundPayload{
			Intent: intent,
		},
		now,
		meta,
		eventPtr(created.ID),
	)
	if err != nil {
		return State{}, err
	}

	state, err = Apply(
		state,
		intentEvent,
	)
	if err != nil {
		return State{}, err
	}

	ready, err := s.buildEvent(
		agentID,
		rootID,
		3,
		EventAgentReadied,
		AgentReadiedPayload{
			Reason: "creation_committed",
		},
		now,
		meta,
		eventPtr(intentEvent.ID),
	)
	if err != nil {
		return State{}, err
	}

	state, err = Apply(
		state,
		ready,
	)
	if err != nil {
		return State{}, err
	}

	return s.commit(
		ctx,
		command,
		meta.RequestID,
		0,
		[]ledger.Event{
			created,
			intentEvent,
			ready,
		},
		state,
	)
}

func (s *Service) Inspect(
	ctx context.Context,
	agentID id.AgentID,
) (State, error) {
	return s.store.Current(
		ctx,
		agentID,
	)
}

func (s *Service) Events(
	ctx context.Context,
	agentID id.AgentID,
	afterVersion uint64,
	limit int,
) ([]ledger.Event, error) {
	if limit <= 0 {
		limit = 100
	}

	if limit > MaxEventPage {
		return nil,
			errs.New(
				errs.CodeInvalidArgument,
				"process.events",
				"limit exceeds maximum page size",
			)
	}

	return s.store.Events(
		ctx,
		agentID,
		afterVersion,
		limit,
	)
}

func (s *Service) Activate(
	ctx context.Context,
	agentID id.AgentID,
	runtimeID id.RuntimeInstanceID,
	expected *uint64,
	meta CommandMeta,
) (State, error) {
	if runtimeID == "" {
		return State{},
			errs.New(
				errs.CodeInvalidArgument,
				"process.activate",
				"runtime instance id is required",
			)
	}

	leaseID, err := s.ids.Lease()
	if err != nil {
		return State{},
			wrapID(
				"process.activate",
				err,
			)
	}

	return s.transition(
		ctx,
		"process.activate",
		agentID,
		expected,
		EventAgentActivated,
		AgentActivatedPayload{
			LeaseID:           leaseID,
			RuntimeInstanceID: runtimeID,
		},
		meta,
	)
}

func (s *Service) Yield(
	ctx context.Context,
	agentID id.AgentID,
	expected *uint64,
	reason string,
	meta CommandMeta,
) (State, error) {
	return s.transition(
		ctx,
		"process.yield",
		agentID,
		expected,
		EventAgentYielded,
		AgentYieldedPayload{
			Reason: reason,
		},
		meta,
	)
}

func (s *Service) Wait(
	ctx context.Context,
	agentID id.AgentID,
	expected *uint64,
	reason WaitingReason,
	reference string,
	meta CommandMeta,
) (State, error) {
	return s.transition(
		ctx,
		"process.wait",
		agentID,
		expected,
		EventAgentWaitStarted,
		AgentWaitStartedPayload{
			Reason:    reason,
			Reference: reference,
		},
		meta,
	)
}

func (s *Service) SatisfyWait(
	ctx context.Context,
	agentID id.AgentID,
	expected *uint64,
	reference string,
	meta CommandMeta,
) (State, error) {
	return s.transition(
		ctx,
		"process.wait_satisfy",
		agentID,
		expected,
		EventAgentWaitSatisfied,
		AgentWaitSatisfiedPayload{
			Reference: reference,
		},
		meta,
	)
}

func (s *Service) Sleep(
	ctx context.Context,
	agentID id.AgentID,
	expected *uint64,
	wakeAt time.Time,
	meta CommandMeta,
) (State, error) {
	if wakeAt.IsZero() {
		return State{},
			errs.New(
				errs.CodeInvalidArgument,
				"process.sleep",
				"wake time is required",
			)
	}

	meta, err := s.normalizeMeta(meta)
	if err != nil {
		return State{}, err
	}

	if state, ok, err :=
		s.receiptState(
			ctx,
			meta.RequestID,
			"process.sleep",
			agentID,
		); err != nil || ok {
		return state, err
	}

	current, err :=
		s.store.Current(
			ctx,
			agentID,
		)
	if err != nil {
		return State{}, err
	}

	if expected != nil &&
		*expected != current.Version {
		return State{},
			versionConflict(
				"process.sleep",
				*expected,
				current.Version,
			)
	}

	sleepID, err := s.ids.Sleep()
	if err != nil {
		return State{},
			wrapID(
				"process.sleep",
				err,
			)
	}

	generation := uint64(1)

	if current.Sleep != nil {
		generation =
			current.Sleep.Generation + 1
	}

	return s.transitionFromState(
		ctx,
		"process.sleep",
		current,
		EventAgentSleepStarted,
		AgentSleepStartedPayload{
			SleepID:    sleepID,
			WakeAt:     wakeAt.UTC(),
			Generation: generation,
		},
		meta,
	)
}

func (s *Service) Wake(
	ctx context.Context,
	due SleepDue,
	meta CommandMeta,
) (State, error) {
	expected := due.Version

	return s.transition(
		ctx,
		"process.wake",
		due.AgentID,
		&expected,
		EventAgentWoken,
		AgentWokenPayload{
			SleepID:    due.SleepID,
			Generation: due.Generation,
		},
		meta,
	)
}

func (s *Service) Suspend(
	ctx context.Context,
	agentID id.AgentID,
	expected *uint64,
	reason string,
	meta CommandMeta,
) (State, error) {
	return s.transition(
		ctx,
		"process.suspend",
		agentID,
		expected,
		EventAgentSuspended,
		AgentSuspendedPayload{
			Reason: reason,
		},
		meta,
	)
}

func (s *Service) Resume(
	ctx context.Context,
	agentID id.AgentID,
	expected *uint64,
	reason string,
	meta CommandMeta,
) (State, error) {
	return s.transition(
		ctx,
		"process.resume",
		agentID,
		expected,
		EventAgentResumed,
		AgentResumedPayload{
			Reason: reason,
		},
		meta,
	)
}

func (s *Service) Cancel(
	ctx context.Context,
	agentID id.AgentID,
	expected *uint64,
	scope string,
	reason string,
	meta CommandMeta,
) (State, error) {
	meta, err := s.normalizeMeta(meta)
	if err != nil {
		return State{}, err
	}

	if state, ok, err :=
		s.receiptState(
			ctx,
			meta.RequestID,
			"process.cancel",
			agentID,
		); err != nil || ok {
		return state, err
	}

	current, err :=
		s.store.Current(
			ctx,
			agentID,
		)
	if err != nil {
		return State{}, err
	}

	if expected != nil &&
		*expected != current.Version {
		return State{},
			versionConflict(
				"process.cancel",
				*expected,
				current.Version,
			)
	}

	now := s.clock.Now().UTC()

	causation := meta.CausationID

	if causation == nil &&
		current.LastEventID != "" {
		causation =
			eventPtr(
				current.LastEventID,
			)
	}

	request, err := s.buildEvent(
		current.AgentID,
		current.RootAgentID,
		current.Version+1,
		EventCancelRequested,
		CancelRequestedPayload{
			Scope: scope,
		},
		now,
		meta,
		causation,
	)
	if err != nil {
		return State{}, err
	}

	next, err := Apply(
		current,
		request,
	)
	if err != nil {
		return State{}, err
	}

	cancelled, err := s.buildEvent(
		current.AgentID,
		current.RootAgentID,
		next.Version+1,
		EventAgentCancelled,
		AgentCancelledPayload{
			Reason: reason,
		},
		now,
		meta,
		eventPtr(request.ID),
	)
	if err != nil {
		return State{}, err
	}

	next, err = Apply(
		next,
		cancelled,
	)
	if err != nil {
		return State{}, err
	}

	return s.commit(
		ctx,
		"process.cancel",
		meta.RequestID,
		current.Version,
		[]ledger.Event{
			request,
			cancelled,
		},
		next,
	)
}

func (s *Service) Complete(
	ctx context.Context,
	agentID id.AgentID,
	expected *uint64,
	summaryRef string,
	meta CommandMeta,
) (State, error) {
	return s.transition(
		ctx,
		"process.complete",
		agentID,
		expected,
		EventAgentCompleted,
		AgentCompletedPayload{
			SummaryRef: summaryRef,
		},
		meta,
	)
}

func (s *Service) Fail(
	ctx context.Context,
	agentID id.AgentID,
	expected *uint64,
	failure Failure,
	meta CommandMeta,
) (State, error) {
	return s.transition(
		ctx,
		"process.fail",
		agentID,
		expected,
		EventAgentFailed,
		AgentFailedPayload{
			Failure: failure,
		},
		meta,
	)
}

func (s *Service) Checkpoint(
	ctx context.Context,
	agentID id.AgentID,
	expected *uint64,
	meta CommandMeta,
) (State, error) {
	checkpointID, err :=
		s.ids.Checkpoint()
	if err != nil {
		return State{},
			wrapID(
				"process.checkpoint",
				err,
			)
	}

	return s.transition(
		ctx,
		"process.checkpoint",
		agentID,
		expected,
		EventCheckpointCreated,
		CheckpointCreatedPayload{
			CheckpointID: checkpointID,
		},
		meta,
	)
}

func (s *Service) RecoverRunning(
	ctx context.Context,
	state State,
	meta CommandMeta,
) (State, error) {
	expected := state.Version

	return s.transition(
		ctx,
		"process.recover_running",
		state.AgentID,
		&expected,
		EventRecoveryActionApplied,
		RecoveryActionAppliedPayload{
			Action: "stale_running_to_ready",

			PreviousStatus: StatusRunning,
		},
		meta,
	)
}

func (s *Service) Snapshot(
	ctx context.Context,
	agentID id.AgentID,
) (Snapshot, error) {
	state, err :=
		s.store.Current(
			ctx,
			agentID,
		)
	if err != nil {
		return Snapshot{}, err
	}

	sequence, err :=
		s.store.LedgerSequenceAtVersion(
			ctx,
			agentID,
			state.Version,
		)
	if err != nil {
		return Snapshot{}, err
	}

	body, err := json.Marshal(state)
	if err != nil {
		return Snapshot{},
			errs.Wrap(
				errs.CodeInternal,
				"process.snapshot",
				"marshal state",
				err,
			)
	}

	sum := sha256.Sum256(body)

	snapshotID, err :=
		s.ids.Snapshot()
	if err != nil {
		return Snapshot{},
			wrapID(
				"process.snapshot",
				err,
			)
	}

	snapshot := Snapshot{
		ID: snapshotID,

		AgentID: agentID,

		ThroughProcessVersion: state.Version,

		ThroughLedgerSequence: sequence,

		SchemaVersion: SnapshotSchemaVersion,

		CreatedAt: s.clock.Now().UTC(),

		StateJSON: body,

		SHA256: hex.EncodeToString(
			sum[:],
		),
	}

	if err :=
		s.store.SaveSnapshot(
			ctx,
			snapshot,
		); err != nil {
		return Snapshot{}, err
	}

	return snapshot, nil
}

func (s *Service) Reconstruct(
	ctx context.Context,
	agentID id.AgentID,
) (State, error) {
	var state State
	var after uint64

	snapshot, ok, err :=
		s.store.LatestSnapshot(
			ctx,
			agentID,
		)
	if err != nil {
		return State{}, err
	}

	if ok {
		if snapshot.SchemaVersion !=
			SnapshotSchemaVersion {
			return State{},
				errs.New(
					errs.CodeUnsupported,
					"process.reconstruct",
					"unsupported snapshot schema version",
				)
		}

		sum :=
			sha256.Sum256(
				snapshot.StateJSON,
			)

		if hex.EncodeToString(sum[:]) !=
			snapshot.SHA256 {
			return State{},
				errs.New(
					errs.CodeCorruption,
					"process.reconstruct",
					"snapshot integrity hash mismatch",
				)
		}

		if err := json.Unmarshal(
			snapshot.StateJSON,
			&state,
		); err != nil {
			return State{},
				errs.Wrap(
					errs.CodeCorruption,
					"process.reconstruct",
					"decode snapshot state",
					err,
				)
		}

		if state.AgentID != agentID ||
			state.Version !=
				snapshot.ThroughProcessVersion {
			return State{},
				errs.New(
					errs.CodeCorruption,
					"process.reconstruct",
					"snapshot identity/version mismatch",
				)
		}

		after =
			snapshot.ThroughProcessVersion
	}

	found := ok

	for {
		events, err :=
			s.store.Events(
				ctx,
				agentID,
				after,
				256,
			)
		if err != nil {
			return State{}, err
		}

		if len(events) == 0 {
			break
		}

		found = true

		for _, event := range events {
			state, err =
				Apply(
					state,
					event,
				)
			if err != nil {
				return State{}, err
			}

			after =
				event.ProcessVersion
		}

		if len(events) < 256 {
			break
		}
	}

	if !found {
		return State{},
			errs.New(
				errs.CodeNotFound,
				"process.reconstruct",
				"agent process not found",
			)
	}

	return state, nil
}

func (s *Service) ListByStatus(
	ctx context.Context,
	status Status,
	limit int,
) ([]State, error) {
	if limit <= 0 ||
		limit > MaxEventPage {
		limit = MaxEventPage
	}

	return s.store.ListByStatus(
		ctx,
		status,
		limit,
	)
}

func (s *Service) DueSleeps(
	ctx context.Context,
	now time.Time,
	limit int,
) ([]SleepDue, error) {
	if limit <= 0 ||
		limit > MaxEventPage {
		limit = MaxEventPage
	}

	return s.store.DueSleeps(
		ctx,
		now.UTC(),
		limit,
	)
}

func (s *Service) transition(
	ctx context.Context,
	command string,
	agentID id.AgentID,
	expected *uint64,
	eventType ledger.EventType,
	payload any,
	meta CommandMeta,
) (State, error) {
	meta, err := s.normalizeMeta(meta)
	if err != nil {
		return State{}, err
	}

	if state, ok, err :=
		s.receiptState(
			ctx,
			meta.RequestID,
			command,
			agentID,
		); err != nil || ok {
		return state, err
	}

	current, err :=
		s.store.Current(
			ctx,
			agentID,
		)
	if err != nil {
		return State{}, err
	}

	if expected != nil &&
		*expected != current.Version {
		return State{},
			versionConflict(
				command,
				*expected,
				current.Version,
			)
	}

	return s.transitionFromState(
		ctx,
		command,
		current,
		eventType,
		payload,
		meta,
	)
}

func (s *Service) transitionFromState(
	ctx context.Context,
	command string,
	current State,
	eventType ledger.EventType,
	payload any,
	meta CommandMeta,
) (State, error) {
	meta, err := s.normalizeMeta(meta)
	if err != nil {
		return State{}, err
	}

	causation := meta.CausationID

	if causation == nil &&
		current.LastEventID != "" {
		causation =
			eventPtr(
				current.LastEventID,
			)
	}

	event, err := s.buildEvent(
		current.AgentID,
		current.RootAgentID,
		current.Version+1,
		eventType,
		payload,
		s.clock.Now().UTC(),
		meta,
		causation,
	)
	if err != nil {
		return State{}, err
	}

	next, err := Apply(
		current,
		event,
	)
	if err != nil {
		return State{}, err
	}

	return s.commit(
		ctx,
		command,
		meta.RequestID,
		current.Version,
		[]ledger.Event{
			event,
		},
		next,
	)
}

func (s *Service) buildEvent(
	agentID id.AgentID,
	rootID id.AgentID,
	version uint64,
	eventType ledger.EventType,
	payload any,
	timestamp time.Time,
	meta CommandMeta,
	causation *id.EventID,
) (ledger.Event, error) {
	eventID, err := s.ids.Event()
	if err != nil {
		return ledger.Event{},
			wrapID(
				"process.build_event",
				err,
			)
	}

	body, err :=
		encodePayload(payload)
	if err != nil {
		return ledger.Event{}, err
	}

	event := ledger.Event{
		ID:             eventID,
		AgentID:        agentID,
		RootAgentID:    rootID,
		ProcessVersion: version,
		Type:           eventType,

		SchemaVersion: EventSchemaVersion,

		Timestamp: timestamp.UTC(),

		CausationID: causation,

		CorrelationID: meta.CorrelationID,

		Actor: meta.Actor,

		Payload: body,
	}

	if err := event.Validate(); err != nil {
		return ledger.Event{}, err
	}

	return event, nil
}

func (s *Service) commit(
	ctx context.Context,
	command string,
	requestID id.RequestID,
	expected uint64,
	events []ledger.Event,
	next State,
) (State, error) {
	result, err := json.Marshal(next)
	if err != nil {
		return State{},
			errs.Wrap(
				errs.CodeInternal,
				command,
				"marshal command result",
				err,
			)
	}

	receipt := Receipt{
		RequestID:     requestID,
		CommandType:   command,
		AgentID:       next.AgentID,
		ResultVersion: next.Version,
		ResultJSON:    result,
		LastEventID:   next.LastEventID,
		CreatedAt:     s.clock.Now().UTC(),
	}

	if err := s.store.Append(
		ctx,
		expected,
		events,
		next,
		receipt,
	); err != nil {
		if errs.IsCode(
			err,
			errs.CodeConflict,
		) {
			state, ok, lookupErr :=
				s.receiptState(
					ctx,
					requestID,
					command,
					next.AgentID,
				)

			if lookupErr != nil {
				return State{},
					lookupErr
			}

			if ok {
				return state, nil
			}
		}

		return State{}, err
	}

	return next, nil
}

func (s *Service) receiptState(
	ctx context.Context,
	requestID id.RequestID,
	command string,
	agentID id.AgentID,
) (State, bool, error) {
	receipt, ok, err :=
		s.store.Receipt(
			ctx,
			requestID,
		)

	if err != nil || !ok {
		return State{},
			false,
			err
	}

	if receipt.CommandType != command ||
		(agentID != "" &&
			receipt.AgentID != agentID) {
		return State{},
			false,
			errs.New(
				errs.CodeConflict,
				command,
				"request_id was already used for a different command",
			)
	}

	var state State

	if err := json.Unmarshal(
		receipt.ResultJSON,
		&state,
	); err != nil {
		return State{},
			false,
			errs.Wrap(
				errs.CodeCorruption,
				command,
				"decode stored command receipt",
				err,
			)
	}

	return state, true, nil
}

func (s *Service) normalizeMeta(
	meta CommandMeta,
) (CommandMeta, error) {
	var err error

	if meta.RequestID == "" {
		meta.RequestID, err =
			s.ids.Request()

		if err != nil {
			return CommandMeta{},
				wrapID(
					"process.command_meta",
					err,
				)
		}
	}

	if meta.CorrelationID == "" {
		meta.CorrelationID, err =
			s.ids.Correlation()

		if err != nil {
			return CommandMeta{},
				wrapID(
					"process.command_meta",
					err,
				)
		}
	}

	if meta.Actor.Kind == "" {
		meta.Actor =
			ledger.ActorRef{
				Kind: ledger.ActorSystem,
			}
	}

	return meta, nil
}

func eventPtr(
	value id.EventID,
) *id.EventID {
	return &value
}

func versionConflict(
	op string,
	expected uint64,
	actual uint64,
) error {
	return errs.New(
		errs.CodeConflict,
		op,
		fmt.Sprintf(
			"expected version %d, actual %d",
			expected,
			actual,
		),
	)
}

func wrapID(
	op string,
	err error,
) error {
	return errs.Wrap(
		errs.CodeInternal,
		op,
		"generate identifier",
		err,
	)
}
