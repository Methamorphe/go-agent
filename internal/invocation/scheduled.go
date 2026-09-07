package invocation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/process"
	"github.com/Methamorphe/go-agent/internal/provider"
	"github.com/Methamorphe/go-agent/internal/scheduler"
)

type BackendResolver interface {
	Resolve(scheduler.ModelProfile) (provider.Provider, error)
}

type RoutingRecorder interface {
	CognitiveRoutingDecided(context.Context, id.AgentID, *uint64, process.CognitiveRoutingDecidedPayload, process.CommandMeta) (process.State, error)
}

type ScheduledService struct {
	base     *Service
	runtime  *scheduler.Runtime
	resolver BackendResolver
}

func NewScheduled(base *Service, runtime *scheduler.Runtime, resolver BackendResolver) (*ScheduledService, error) {
	if base == nil || runtime == nil || resolver == nil {
		return nil, errs.New(errs.CodeInvalidArgument, "invocation.scheduled.new", "base service, scheduler runtime and backend resolver are required")
	}
	if _, ok := base.processes.(RoutingRecorder); !ok {
		return nil, errs.New(errs.CodeUnsupported, "invocation.scheduled.new", "process recorder does not support cognitive routing decisions")
	}
	return &ScheduledService{base: base, runtime: runtime, resolver: resolver}, nil
}

func (s *ScheduledService) Invoke(
	ctx context.Context,
	task scheduler.CognitiveTask,
	messages []provider.Message,
	tools []provider.ToolDefinition,
	expected *uint64,
	meta process.CommandMeta,
) (Outcome, process.State, scheduler.RoutingDecision, error) {
	if err := task.Validate(); err != nil {
		return Outcome{}, process.State{}, scheduler.RoutingDecision{}, errs.Wrap(errs.CodeInvalidArgument, "invocation.scheduled.invoke", "validate cognitive task", err)
	}
	if task.AgentID == "" {
		return Outcome{}, process.State{}, scheduler.RoutingDecision{}, errs.New(errs.CodeInvalidArgument, "invocation.scheduled.invoke", "agent id is required")
	}

	recorder := s.base.processes.(RoutingRecorder)
	excluded := make(map[string]struct{})
	maxAttempts := len(s.runtime.Router.Registry().Snapshot())
	if maxAttempts == 0 {
		maxAttempts = 1
	}

	var state process.State
	var lastDecision scheduler.RoutingDecision
	var lastErr error
	currentExpected := expected

	for attemptNumber := 0; attemptNumber < maxAttempts; attemptNumber++ {
		attempt, err := s.runtime.Prepare(task, excluded)
		if err != nil {
			if lastErr != nil {
				return Outcome{}, state, lastDecision, errors.Join(lastErr, err)
			}
			return Outcome{}, state, lastDecision, err
		}
		lastDecision = attempt.Decision

		decisionBody, err := json.Marshal(attempt.Decision)
		if err != nil {
			s.abort(attempt)
			return Outcome{}, state, lastDecision, errs.Wrap(errs.CodeInternal, "invocation.scheduled.invoke", "encode routing decision", err)
		}
		decisionObject, err := s.base.objects.Put(ctx, bytes.NewReader(decisionBody))
		if err != nil {
			s.abort(attempt)
			return Outcome{}, state, lastDecision, err
		}

		routed, err := recorder.CognitiveRoutingDecided(ctx, task.AgentID, currentExpected, process.CognitiveRoutingDecidedPayload{
			DecisionID:           string(attempt.Decision.ID),
			TaskID:               string(task.ID),
			Provider:             string(attempt.Profile.ProviderID),
			Model:                string(attempt.Profile.ModelID),
			ProfileVersion:       attempt.Profile.ProfileVersion,
			DecisionRef:          string(decisionObject.Ref),
			ReservationID:        string(attempt.ReservationID),
			EstimatedMoneyMicros: attempt.Reserved.MoneyMicros,
			EstimatedTokens:      attempt.Reserved.Tokens,
		}, meta)
		if err != nil {
			s.abort(attempt)
			return Outcome{}, state, lastDecision, err
		}
		state = routed

		model, err := s.resolver.Resolve(attempt.Profile)
		if err != nil {
			finishErr := s.runtime.Release(attempt, scheduler.ObservationPermanentFailure)
			if finishErr != nil {
				return Outcome{}, state, lastDecision, errors.Join(err, finishErr)
			}
			excluded[attempt.Profile.Key()] = struct{}{}
			lastErr = err
			nextExpected := state.Version
			currentExpected = &nextExpected
			continue
		}

		nextExpected := state.Version
		outcome, nextState, invokeErr := s.base.Invoke(ctx, model, task.AgentID, string(attempt.Profile.ModelID), messages, tools, &nextExpected, meta)
		state = nextState
		observation := observationFor(invokeErr, ctx.Err())
		actual := actualResources(attempt, outcome.Result.Usage, invokeErr == nil)
		finishErr := s.runtime.Finish(attempt, actual, observation)
		if finishErr != nil {
			if invokeErr != nil {
				return outcome, state, lastDecision, errors.Join(invokeErr, finishErr)
			}
			return outcome, state, lastDecision, finishErr
		}
		if invokeErr == nil {
			return outcome, state, lastDecision, nil
		}
		if ctx.Err() != nil || observation != scheduler.ObservationTransientFailure {
			return outcome, state, lastDecision, invokeErr
		}

		excluded[attempt.Profile.Key()] = struct{}{}
		lastErr = invokeErr
		nextExpected = state.Version
		currentExpected = &nextExpected
	}

	if lastErr == nil {
		lastErr = scheduler.ErrNoEligibleModel
	}
	return Outcome{}, state, lastDecision, lastErr
}

func (s *ScheduledService) abort(attempt *scheduler.Attempt) {
	if attempt == nil {
		return
	}
	if attempt.Lease != nil {
		attempt.Lease.Release()
	}
	_ = s.runtime.Budgets.Release(attempt.ReservationID)
}

func observationFor(invokeErr, contextErr error) scheduler.ObservationKind {
	if invokeErr == nil {
		return scheduler.ObservationSuccess
	}
	if contextErr != nil || errors.Is(invokeErr, context.Canceled) || errors.Is(invokeErr, context.DeadlineExceeded) {
		return scheduler.ObservationCancelled
	}
	if errs.IsCode(invokeErr, errs.CodeUnavailable) {
		return scheduler.ObservationTransientFailure
	}
	return scheduler.ObservationPermanentFailure
}

func actualResources(attempt *scheduler.Attempt, usage provider.Usage, successful bool) scheduler.Resources {
	if attempt == nil {
		return scheduler.Resources{}
	}
	input, inputKnown := optionalValue(usage.InputTokens)
	output, outputKnown := optionalValue(usage.OutputTokens)
	total, totalKnown := optionalValue(usage.TotalTokens)
	if !inputKnown && !outputKnown && !totalKnown {
		if successful {
			return attempt.Reserved
		}
		return scheduler.Resources{}
	}
	if !totalKnown {
		total = input + output
	}
	money := int64(0)
	if attempt.Profile.InputCostMicrosPerMillion >= 0 && attempt.Profile.OutputCostMicrosPerMillion >= 0 {
		money = ceilMillion(input*attempt.Profile.InputCostMicrosPerMillion + output*attempt.Profile.OutputCostMicrosPerMillion)
	}
	if total < 0 || money < 0 {
		return scheduler.Resources{}
	}
	return scheduler.Resources{MoneyMicros: money, Tokens: total}
}

func optionalValue(value *int64) (int64, bool) {
	if value == nil {
		return 0, false
	}
	if *value < 0 {
		return 0, false
	}
	return *value, true
}

func ceilMillion(value int64) int64 {
	if value <= 0 {
		return 0
	}
	return (value + 999999) / 1000000
}

func (s *ScheduledService) String() string {
	return fmt.Sprintf("scheduled-invocation[%d profiles]", len(s.runtime.Router.Registry().Snapshot()))
}
