package team

import (
	"context"
	"fmt"
	"strings"

	"github.com/Methamorphe/go-agent/internal/clock"
	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/orchestration"
)

const (
	defaultMaxMembers = 8
	defaultMaxRounds  = 4
	defaultMaxTurns   = 32
	hardMaxMembers    = 16
	hardMaxRounds     = 16
	hardMaxTurns      = 128
)

type IDGenerator interface {
	Team() (id.TeamID, error)
	Negotiation() (id.NegotiationID, error)
}

type TeamAdmitter interface {
	AdmitTeam(context.Context, Proposal) (AdmissionDecision, error)
}

type Spawner interface {
	Spawn(context.Context, orchestration.SpawnSpec) (orchestration.SpawnOutcome, error)
}

type Service struct {
	store Store
	spawner Spawner
	admitter TeamAdmitter
	ids IDGenerator
	clock clock.Clock
}

func New(store Store, spawner Spawner, admitter TeamAdmitter, ids IDGenerator, source clock.Clock) (*Service, error) {
	if store == nil || spawner == nil || admitter == nil || ids == nil {
		return nil, errs.New(errs.CodeInvalidArgument, "team.new", "store, spawner, admitter and ids are required")
	}
	if source == nil { source = clock.NewSystemClock() }
	return &Service{store: store, spawner: spawner, admitter: admitter, ids: ids, clock: source}, nil
}

func (s *Service) Propose(ctx context.Context, proposal Proposal) (Team, error) {
	if proposal.RootAgentID == "" || proposal.LeadAgentID == "" || strings.TrimSpace(proposal.Objective) == "" {
		return Team{}, errs.New(errs.CodeInvalidArgument, "team.propose", "root, lead and objective are required")
	}
	if len(proposal.Specialists) == 0 {
		return Team{}, errs.New(errs.CodeInvalidArgument, "team.propose", "at least one specialist is required")
	}
	proposal.Limits = normalizeLimits(proposal.Limits)
	if err := validateLimits(proposal.Limits, len(proposal.Specialists)); err != nil { return Team{}, err }
	if proposal.Limits.Deadline != nil && !proposal.Limits.Deadline.After(s.clock.Now()) {
		return Team{}, errs.New(errs.CodeInvalidArgument, "team.propose", "deadline must be in the future")
	}
	seen := make(map[string]struct{}, len(proposal.Specialists))
	for i := range proposal.Specialists {
		profile := &proposal.Specialists[i]
		profile.Role = strings.TrimSpace(profile.Role)
		profile.TaskIntent = strings.TrimSpace(profile.TaskIntent)
		if profile.Role == "" || profile.TaskIntent == "" || !profile.Budget.Valid() || profile.Budget.IsZero() {
			return Team{}, errs.New(errs.CodeInvalidArgument, "team.propose", "each specialist needs a role, task and non-zero valid budget")
		}
		key := strings.ToLower(profile.Role)
		if _, ok := seen[key]; ok { return Team{}, errs.New(errs.CodeInvalidArgument, "team.propose", "specialist roles must be unique") }
		seen[key] = struct{}{}
	}
	if proposal.ID == "" {
		generated, err := s.ids.Team()
		if err != nil { return Team{}, errs.Wrap(errs.CodeInternal, "team.propose", "generate team id", err) }
		proposal.ID = generated
	}
	now := s.clock.Now().UTC()
	if proposal.CreatedAt.IsZero() { proposal.CreatedAt = now }
	result := Team{Proposal: proposal, State: TeamProposed, CreatedAt: now, UpdatedAt: now}
	if err := s.store.PutTeam(ctx, result); err != nil { return Team{}, err }
	return result, nil
}

func (s *Service) Form(ctx context.Context, teamID id.TeamID) (Team, error) {
	current, err := s.store.Team(ctx, teamID)
	if err != nil { return Team{}, err }
	if current.State != TeamProposed { return Team{}, errs.New(errs.CodeConflict, "team.form", "team is not in PROPOSED state") }
	decision, err := s.admitter.AdmitTeam(ctx, current.Proposal)
	if err != nil { return Team{}, err }
	current.Admission = decision
	current.UpdatedAt = s.clock.Now().UTC()
	if !decision.Admitted {
		current.State = TeamRejected
		current.Failure = strings.TrimSpace(decision.Reason)
		if err := s.store.PutTeam(ctx, current); err != nil { return Team{}, err }
		return current, nil
	}
	current.State = TeamAdmitted
	if err := s.store.PutTeam(ctx, current); err != nil { return Team{}, err }

	members := make([]Member, 0, len(current.Proposal.Specialists))
	for _, specialist := range current.Proposal.Specialists {
		outcome, spawnErr := s.spawner.Spawn(ctx, orchestration.SpawnSpec{
			ParentAgentID: current.Proposal.LeadAgentID,
			TaskIntent: specialist.TaskIntent,
			Authority: specialist.Authority,
			Budget: specialist.Budget,
			Result: specialist.Result,
			Deadline: current.Proposal.Limits.Deadline,
			IndependentVerification: specialist.IndependentVerification,
		})
		if spawnErr != nil {
			current.State = TeamFormationFailed
			current.Members = members
			current.Failure = fmt.Sprintf("specialist %q: %v", specialist.Role, spawnErr)
			current.UpdatedAt = s.clock.Now().UTC()
			_ = s.store.PutTeam(ctx, current)
			return current, spawnErr
		}
		members = append(members, Member{Role: specialist.Role, AgentID: outcome.Child.AgentID, SpawnID: outcome.Spawn.ID})
	}
	current.Members = members
	current.State = TeamActive
	current.UpdatedAt = s.clock.Now().UTC()
	if err := s.store.PutTeam(ctx, current); err != nil { return Team{}, err }
	return current, nil
}

func normalizeLimits(in Limits) Limits {
	if in.MaxMembers <= 0 { in.MaxMembers = defaultMaxMembers }
	if in.MaxParallelism <= 0 { in.MaxParallelism = in.MaxMembers }
	if in.MaxRounds <= 0 { in.MaxRounds = defaultMaxRounds }
	if in.MaxTurns <= 0 { in.MaxTurns = defaultMaxTurns }
	return in
}

func validateLimits(l Limits, members int) error {
	if l.MaxMembers > hardMaxMembers || l.MaxRounds > hardMaxRounds || l.MaxTurns > hardMaxTurns {
		return errs.New(errs.CodeResourceExhausted, "team.propose", "team limits exceed G11 hard bounds")
	}
	if members > l.MaxMembers || l.MaxParallelism > l.MaxMembers || l.MaxParallelism <= 0 {
		return errs.New(errs.CodeResourceExhausted, "team.propose", "team topology exceeds configured bounds")
	}
	return nil
}
