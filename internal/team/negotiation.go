package team

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
)

func (s *Service) StartNegotiation(ctx context.Context, teamID id.TeamID, subject string, participants []id.AgentID) (Negotiation, error) {
	current, err := s.store.Team(ctx, teamID)
	if err != nil { return Negotiation{}, err }
	if current.State != TeamActive && current.State != TeamEscalated {
		return Negotiation{}, errs.New(errs.CodeConflict, "team.negotiation.start", "team must be active")
	}
	participants = uniqueAgents(participants)
	if len(participants) < 2 {
		return Negotiation{}, errs.New(errs.CodeInvalidArgument, "team.negotiation.start", "at least two participants are required")
	}
	allowed := map[id.AgentID]struct{}{current.Proposal.LeadAgentID: {}}
	for _, member := range current.Members { allowed[member.AgentID] = struct{}{} }
	for _, participant := range participants {
		if _, ok := allowed[participant]; !ok {
			return Negotiation{}, errs.New(errs.CodePermissionDenied, "team.negotiation.start", "participant is not a member of the team")
		}
	}
	if strings.TrimSpace(subject) == "" {
		return Negotiation{}, errs.New(errs.CodeInvalidArgument, "team.negotiation.start", "subject is required")
	}
	negotiationID, err := s.ids.Negotiation()
	if err != nil { return Negotiation{}, errs.Wrap(errs.CodeInternal, "team.negotiation.start", "generate negotiation id", err) }
	now := s.clock.Now().UTC()
	n := Negotiation{
		ID: negotiationID,
		TeamID: teamID,
		Subject: strings.TrimSpace(subject),
		Participants: participants,
		State: NegotiationOpen,
		MaxRounds: current.Proposal.Limits.MaxRounds,
		MaxTurns: current.Proposal.Limits.MaxTurns,
		Deadline: current.Proposal.Limits.Deadline,
		Version: 1,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.store.PutNegotiation(ctx, n); err != nil { return Negotiation{}, err }
	return n, nil
}

func (s *Service) Turn(ctx context.Context, negotiationID id.NegotiationID, actor id.AgentID, kind TurnKind, statement string, evidenceRefs []string, replyTo uint64) (Negotiation, Turn, error) {
	n, err := s.store.Negotiation(ctx, negotiationID)
	if err != nil { return Negotiation{}, Turn{}, err }
	if n.State != NegotiationOpen { return Negotiation{}, Turn{}, errs.New(errs.CodeConflict, "team.negotiation.turn", "negotiation is terminal") }
	if !containsAgent(n.Participants, actor) { return Negotiation{}, Turn{}, errs.New(errs.CodePermissionDenied, "team.negotiation.turn", "actor is not a negotiation participant") }
	now := s.clock.Now().UTC()
	if n.Deadline != nil && !n.Deadline.After(now) { return s.escalate(ctx, n, actor, "negotiation deadline exceeded", NegotiationExpired) }
	turns, err := s.store.Turns(ctx, negotiationID, n.MaxTurns)
	if err != nil { return Negotiation{}, Turn{}, err }
	if len(turns) >= n.MaxTurns { return s.escalate(ctx, n, actor, "negotiation turn limit exceeded", NegotiationEscalated) }
	if err := validateTurn(turns, actor, kind, statement, evidenceRefs, replyTo); err != nil { return Negotiation{}, Turn{}, err }
	round := n.Round
	if kind == TurnChallenge {
		round++
		if round > n.MaxRounds { return s.escalate(ctx, n, actor, "negotiation round limit exceeded", NegotiationEscalated) }
	}
	turn := Turn{
		Sequence: uint64(len(turns) + 1),
		Round: round,
		ActorAgentID: actor,
		Kind: kind,
		Statement: strings.TrimSpace(statement),
		EvidenceRefs: canonicalRefs(evidenceRefs),
		ReplyTo: replyTo,
		CreatedAt: now,
	}
	state := NegotiationOpen
	resolution := ""
	escalatedTo := id.AgentID("")
	if kind == TurnConcede {
		state = NegotiationConverged
		resolution = turn.Statement
	}
	if kind == TurnEscalation {
		state = NegotiationEscalated
		resolution = turn.Statement
		escalatedTo = s.escalationTarget(ctx, n.TeamID)
	}
	next, err := s.store.AppendTurn(ctx, n.ID, n.Version, turn, state, round, resolution, escalatedTo, now)
	if err != nil { return Negotiation{}, Turn{}, err }
	if state == NegotiationEscalated { s.markTeamEscalated(ctx, n.TeamID, now) }
	return next, turn, nil
}

func (s *Service) escalate(ctx context.Context, n Negotiation, actor id.AgentID, reason string, state NegotiationState) (Negotiation, Turn, error) {
	turns, err := s.store.Turns(ctx, n.ID, n.MaxTurns)
	if err != nil { return Negotiation{}, Turn{}, err }
	now := s.clock.Now().UTC()
	turn := Turn{Sequence: uint64(len(turns) + 1), Round: n.Round, ActorAgentID: actor, Kind: TurnEscalation, Statement: reason, CreatedAt: now}
	target := s.escalationTarget(ctx, n.TeamID)
	next, err := s.store.AppendTurn(ctx, n.ID, n.Version, turn, state, n.Round, reason, target, now)
	if err != nil { return Negotiation{}, Turn{}, err }
	s.markTeamEscalated(ctx, n.TeamID, now)
	return next, turn, nil
}

func (s *Service) markTeamEscalated(ctx context.Context, teamID id.TeamID, now time.Time) {
	current, err := s.store.Team(ctx, teamID)
	if err != nil { return }
	current.State = TeamEscalated
	current.UpdatedAt = now
	_ = s.store.PutTeam(ctx, current)
}

func (s *Service) escalationTarget(ctx context.Context, teamID id.TeamID) id.AgentID {
	current, err := s.store.Team(ctx, teamID)
	if err != nil { return "" }
	return current.Proposal.LeadAgentID
}

func validateTurn(turns []Turn, actor id.AgentID, kind TurnKind, statement string, evidenceRefs []string, replyTo uint64) error {
	statement = strings.TrimSpace(statement)
	if len(turns) == 0 {
		if kind != TurnClaim || statement == "" || replyTo != 0 { return errs.New(errs.CodeInvalidArgument, "team.negotiation.turn", "first turn must be a root claim") }
		return nil
	}
	find := func(seq uint64) (Turn, bool) {
		for _, turn := range turns { if turn.Sequence == seq { return turn, true } }
		return Turn{}, false
	}
	if replyTo == 0 && kind != TurnEscalation { return errs.New(errs.CodeInvalidArgument, "team.negotiation.turn", "non-root turns must reference a previous turn") }
	parent, ok := find(replyTo)
	if kind != TurnEscalation && !ok { return errs.New(errs.CodeInvalidArgument, "team.negotiation.turn", "reply target does not exist") }
	switch kind {
	case TurnClaim:
		return errs.New(errs.CodeInvalidArgument, "team.negotiation.turn", "additional root claims are not allowed; use revision")
	case TurnChallenge:
		if actor == parent.ActorAgentID || (parent.Kind != TurnClaim && parent.Kind != TurnRevision) || statement == "" { return errs.New(errs.CodeInvalidArgument, "team.negotiation.turn", "challenge must contest another participant's claim or revision") }
	case TurnEvidence:
		if parent.Kind != TurnChallenge || len(canonicalRefs(evidenceRefs)) == 0 { return errs.New(errs.CodeInvalidArgument, "team.negotiation.turn", "evidence must answer a challenge with evidence references") }
	case TurnCounterexample:
		if len(canonicalRefs(evidenceRefs)) == 0 || (parent.Kind != TurnClaim && parent.Kind != TurnRevision && parent.Kind != TurnEvidence) { return errs.New(errs.CodeInvalidArgument, "team.negotiation.turn", "counterexample requires evidence and a claim/evidence target") }
	case TurnRevision:
		if statement == "" || (parent.Kind != TurnChallenge && parent.Kind != TurnEvidence && parent.Kind != TurnCounterexample) { return errs.New(errs.CodeInvalidArgument, "team.negotiation.turn", "revision must answer a challenge or evidence") }
	case TurnConcede:
		if statement == "" { return errs.New(errs.CodeInvalidArgument, "team.negotiation.turn", "concession requires an explicit resolution") }
	case TurnEscalation:
		if statement == "" { return errs.New(errs.CodeInvalidArgument, "team.negotiation.turn", "escalation requires a reason") }
	default:
		return errs.New(errs.CodeInvalidArgument, "team.negotiation.turn", "unknown negotiation turn kind")
	}
	return nil
}

func canonicalRefs(refs []string) []string {
	set := make(map[string]struct{}, len(refs))
	for _, ref := range refs { ref = strings.TrimSpace(ref); if ref != "" { set[ref] = struct{}{} } }
	out := make([]string, 0, len(set))
	for ref := range set { out = append(out, ref) }
	sort.Strings(out)
	return out
}

func uniqueAgents(in []id.AgentID) []id.AgentID {
	seen := make(map[id.AgentID]struct{}, len(in)); out := make([]id.AgentID, 0, len(in))
	for _, agent := range in { if agent == "" { continue }; if _, ok := seen[agent]; ok { continue }; seen[agent] = struct{}{}; out = append(out, agent) }
	return out
}

func containsAgent(values []id.AgentID, target id.AgentID) bool {
	for _, value := range values { if value == target { return true } }
	return false
}
