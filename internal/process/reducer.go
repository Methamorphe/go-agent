package process

import (
	"fmt"

	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/ledger"
)

func Apply(
	state State,
	event ledger.Event,
) (State, error) {
	if err := event.Validate(); err != nil {
		return State{}, err
	}

	if state.AgentID == "" {
		if event.Type != EventAgentCreated ||
			event.ProcessVersion != 1 {
			return State{},
				impossible(
					event,
					"first event must be AgentCreated@1",
				)
		}
	} else {
		if event.AgentID != state.AgentID ||
			event.RootAgentID != state.RootAgentID {
			return State{},
				impossible(
					event,
					"event lineage does not match process",
				)
		}

		if event.ProcessVersion != state.Version+1 {
			return State{},
				impossible(
					event,
					fmt.Sprintf(
						"expected process version %d",
						state.Version+1,
					),
				)
		}

		if state.Status.Terminal() {
			return State{},
				impossible(
					event,
					"terminal process cannot accept more process events",
				)
		}
	}

	next := state

	switch event.Type {
	case EventAgentCreated:
		payload, err := decodeAgentCreated(event)
		if err != nil {
			return State{}, err
		}

		next = State{
			AgentID:         event.AgentID,
			RootAgentID:     event.RootAgentID,
			ParentAgentID:   payload.ParentAgentID,
			CreationEventID: event.ID,
			CreatedAt:       event.Timestamp.UTC(),
			LineageDepth:    payload.LineageDepth,
			Status:          StatusNew,
		}

	case EventRootIntentBound:
		if next.Status != StatusNew ||
			next.RootIntent != nil {
			return State{},
				impossible(
					event,
					"root intent can only be bound once while NEW",
				)
		}

		payload, err :=
			decodePayload[RootIntentBoundPayload](
				event,
			)
		if err != nil {
			return State{}, err
		}

		if payload.Intent.ID == "" ||
			payload.Intent.SchemaVersion == 0 ||
			payload.Intent.Goal == "" ||
			payload.Intent.CreatedAt.IsZero() {
			return State{},
				impossible(
					event,
					"root intent is incomplete",
				)
		}

		intent := payload.Intent
		intent.CreatedAt =
			intent.CreatedAt.UTC()

		next.RootIntent = &intent

	case EventAgentReadied:
		if next.Status != StatusNew {
			return State{},
				impossible(
					event,
					"AgentReadied requires NEW",
				)
		}

		next.Status = StatusReady

	case EventAgentActivated:
		if next.Status != StatusReady {
			return State{},
				impossible(
					event,
					"AgentActivated requires READY",
				)
		}

		payload, err :=
			decodePayload[AgentActivatedPayload](
				event,
			)
		if err != nil {
			return State{}, err
		}

		if payload.LeaseID == "" ||
			payload.RuntimeInstanceID == "" {
			return State{},
				impossible(
					event,
					"activation lease is incomplete",
				)
		}

		next.Status = StatusRunning

		next.Lease = &ExecutionLease{
			ID: payload.LeaseID,

			RuntimeInstanceID: payload.RuntimeInstanceID,

			AcquiredAt: event.Timestamp.UTC(),

			ProcessVersion: event.ProcessVersion,
		}

	case EventAgentYielded:
		if next.Status != StatusRunning {
			return State{},
				impossible(
					event,
					"AgentYielded requires RUNNING",
				)
		}

		next.Status = StatusReady
		next.Lease = nil

	case EventAgentWaitStarted:
		if next.Status != StatusRunning {
			return State{},
				impossible(
					event,
					"AgentWaitStarted requires RUNNING",
				)
		}

		payload, err :=
			decodePayload[AgentWaitStartedPayload](
				event,
			)
		if err != nil {
			return State{}, err
		}

		if !validWaitingReason(
			payload.Reason,
		) {
			return State{},
				impossible(
					event,
					"unknown waiting reason",
				)
		}

		next.Status = StatusWaiting
		next.Lease = nil

		next.Wait = &WaitState{
			Reason:    payload.Reason,
			Reference: payload.Reference,
			Since:     event.Timestamp.UTC(),
		}

	case EventAgentWaitSatisfied:
		if next.Status != StatusWaiting ||
			next.Wait == nil {
			return State{},
				impossible(
					event,
					"AgentWaitSatisfied requires WAITING",
				)
		}

		payload, err :=
			decodePayload[AgentWaitSatisfiedPayload](
				event,
			)
		if err != nil {
			return State{}, err
		}

		if payload.Reference != "" &&
			next.Wait.Reference != "" &&
			payload.Reference !=
				next.Wait.Reference {
			return State{},
				impossible(
					event,
					"wait reference does not match active wait",
				)
		}

		next.Status = StatusReady
		next.Wait = nil

	case EventAgentSleepStarted:
		if next.Status != StatusRunning {
			return State{},
				impossible(
					event,
					"AgentSleepStarted requires RUNNING",
				)
		}

		payload, err :=
			decodePayload[AgentSleepStartedPayload](
				event,
			)
		if err != nil {
			return State{}, err
		}

		if payload.SleepID == "" ||
			payload.WakeAt.IsZero() ||
			payload.Generation == 0 {
			return State{},
				impossible(
					event,
					"sleep payload is incomplete",
				)
		}

		next.Status = StatusSleeping
		next.Lease = nil

		next.Sleep = &SleepState{
			ID: payload.SleepID,

			WakeAt: payload.WakeAt.UTC(),

			Generation: payload.Generation,

			CreatedAt: event.Timestamp.UTC(),
		}

	case EventAgentWoken:
		if next.Status != StatusSleeping ||
			next.Sleep == nil {
			return State{},
				impossible(
					event,
					"AgentWoken requires SLEEPING",
				)
		}

		payload, err :=
			decodePayload[AgentWokenPayload](
				event,
			)
		if err != nil {
			return State{}, err
		}

		if payload.SleepID !=
			next.Sleep.ID ||
			payload.Generation !=
				next.Sleep.Generation {
			return State{},
				impossible(
					event,
					"stale wake does not match active sleep",
				)
		}

		next.Status = StatusReady
		next.Sleep = nil

	case EventCancelRequested:
		payload, err :=
			decodePayload[CancelRequestedPayload](
				event,
			)
		if err != nil {
			return State{}, err
		}

		next.Cancel = CancelState{
			Requested: true,
			Scope:     payload.Scope,
			EventID:   event.ID,
		}

	case EventAgentCancelled:
		next.Status = StatusCancelled
		next.Wait = nil
		next.Sleep = nil
		next.Lease = nil

	case EventAgentSuspended:
		if next.Status != StatusReady &&
			next.Status != StatusRunning &&
			next.Status != StatusWaiting &&
			next.Status != StatusSleeping {
			return State{},
				impossible(
					event,
					"AgentSuspended requires READY/RUNNING/WAITING/SLEEPING",
				)
		}

		next.Status = StatusSuspended
		next.Wait = nil
		next.Sleep = nil
		next.Lease = nil

	case EventAgentResumed:
		if next.Status != StatusSuspended {
			return State{},
				impossible(
					event,
					"AgentResumed requires SUSPENDED",
				)
		}

		next.Status = StatusReady

	case EventAgentCompleted:
		if next.Status != StatusRunning {
			return State{},
				impossible(
					event,
					"AgentCompleted requires RUNNING",
				)
		}

		payload, err :=
			decodePayload[AgentCompletedPayload](
				event,
			)
		if err != nil {
			return State{}, err
		}

		if next.Cancel.Requested {
			return State{},
				impossible(
					event,
					"cancel-requested process cannot complete successfully",
				)
		}

		next.Status = StatusCompleted
		next.CompletionRef =
			payload.SummaryRef
		next.Lease = nil

	case EventAgentFailed:
		payload, err :=
			decodePayload[AgentFailedPayload](
				event,
			)
		if err != nil {
			return State{}, err
		}

		if payload.Failure.Code == "" ||
			payload.Failure.Message == "" {
			return State{},
				impossible(
					event,
					"failure must have code and message",
				)
		}

		failure := payload.Failure

		next.Status = StatusFailed
		next.Failure = &failure
		next.Wait = nil
		next.Sleep = nil
		next.Lease = nil

	case EventCheckpointCreated:
		payload, err :=
			decodePayload[CheckpointCreatedPayload](
				event,
			)
		if err != nil {
			return State{}, err
		}

		if payload.CheckpointID == "" {
			return State{},
				impossible(
					event,
					"checkpoint id is required",
				)
		}

		next.LastCheckpointID =
			payload.CheckpointID

	case EventRecoveryActionApplied:
		payload, err :=
			decodePayload[RecoveryActionAppliedPayload](event)
		if err != nil {
			return State{}, err
		}

		if payload.Action !=
			"stale_running_to_ready" ||
			payload.PreviousStatus !=
				StatusRunning ||
			next.Status != StatusRunning {
			return State{},
				impossible(
					event,
					"unsupported recovery action",
				)
		}

		next.Status = StatusReady
		next.Lease = nil

	default:
		return State{},
			impossible(
				event,
				"unknown event type",
			)
	}

	next.Version =
		event.ProcessVersion

	next.LastEventID =
		event.ID

	next.UpdatedAt =
		event.Timestamp.UTC()

	return next, nil
}

func validWaitingReason(
	reason WaitingReason,
) bool {
	switch reason {
	case WaitingModel,
		WaitingTool,
		WaitingChild,
		WaitingApproval,
		WaitingResource,
		WaitingReconciliation:
		return true

	default:
		return false
	}
}

func impossible(
	event ledger.Event,
	message string,
) error {
	return errs.New(
		errs.CodeCorruption,
		"process.reducer",
		fmt.Sprintf(
			"%s v%d %s: %s",
			event.AgentID,
			event.ProcessVersion,
			event.Type,
			message,
		),
	)
}
