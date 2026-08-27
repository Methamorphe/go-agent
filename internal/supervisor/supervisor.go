package supervisor

import (
	"context"
	"log/slog"
	"time"

	"github.com/Methamorphe/go-agent/internal/clock"
	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/ledger"
	agentprocess "github.com/Methamorphe/go-agent/internal/process"
)

const recoveryBatch = 256

type ProcessService interface {
	Activate(
		context.Context,
		id.AgentID,
		id.RuntimeInstanceID,
		*uint64,
		agentprocess.CommandMeta,
	) (agentprocess.State, error)

	ListByStatus(
		context.Context,
		agentprocess.Status,
		int,
	) ([]agentprocess.State, error)

	RecoverRunning(
		context.Context,
		agentprocess.State,
		agentprocess.CommandMeta,
	) (agentprocess.State, error)

	DueSleeps(
		context.Context,
		time.Time,
		int,
	) ([]agentprocess.SleepDue, error)

	Wake(
		context.Context,
		agentprocess.SleepDue,
		agentprocess.CommandMeta,
	) (agentprocess.State, error)
}

type Supervisor struct {
	logger    *slog.Logger
	processes ProcessService
	clock     clock.Clock

	runtimeID id.RuntimeInstanceID

	tick time.Duration
}

func New(
	logger *slog.Logger,
	processes ProcessService,
	clock clock.Clock,
	runtimeID id.RuntimeInstanceID,
) *Supervisor {
	return &Supervisor{
		logger:    logger,
		processes: processes,
		clock:     clock,
		runtimeID: runtimeID,
		tick:      250 * time.Millisecond,
	}
}

func (s *Supervisor) Recover(
	ctx context.Context,
) error {
	for {
		running, err :=
			s.processes.ListByStatus(
				ctx,
				agentprocess.StatusRunning,
				recoveryBatch,
			)
		if err != nil {
			return err
		}

		if len(running) == 0 {
			break
		}

		for _, state := range running {
			_, err :=
				s.processes.RecoverRunning(
					ctx,
					state,
					agentprocess.CommandMeta{
						Actor: ledger.ActorRef{
							Kind: ledger.ActorSupervisor,

							ID: s.runtimeID.String(),
						},
					},
				)

			if err != nil &&
				!errs.IsCode(
					err,
					errs.CodeConflict,
				) {
				return err
			}
		}

		if len(running) <
			recoveryBatch {
			break
		}
	}

	return s.wakeDue(ctx)
}

func (s *Supervisor) Activate(
	ctx context.Context,
	agentID id.AgentID,
	expected *uint64,
) (agentprocess.State, error) {
	return s.processes.Activate(
		ctx,
		agentID,
		s.runtimeID,
		expected,
		agentprocess.CommandMeta{
			Actor: ledger.ActorRef{
				Kind: ledger.ActorSupervisor,

				ID: s.runtimeID.String(),
			},
		},
	)
}

func (s *Supervisor) Run(
	ctx context.Context,
) error {
	ticker := time.NewTicker(s.tick)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil

		case <-ticker.C:
			if err := s.wakeDue(
				ctx,
			); err != nil {
				if ctx.Err() != nil {
					return nil
				}

				s.logger.Error(
					"durable wake scan failed",
					"error",
					err,
				)
			}
		}
	}
}

func (s *Supervisor) wakeDue(
	ctx context.Context,
) error {
	for {
		due, err :=
			s.processes.DueSleeps(
				ctx,
				s.clock.Now(),
				recoveryBatch,
			)
		if err != nil {
			return err
		}

		if len(due) == 0 {
			return nil
		}

		for _, wake := range due {
			_, err :=
				s.processes.Wake(
					ctx,
					wake,
					agentprocess.CommandMeta{
						Actor: ledger.ActorRef{
							Kind: ledger.ActorSupervisor,

							ID: s.runtimeID.String(),
						},
					},
				)

			if err != nil &&
				!errs.IsCode(
					err,
					errs.CodeConflict,
				) {
				return err
			}
		}

		if len(due) <
			recoveryBatch {
			return nil
		}
	}
}
