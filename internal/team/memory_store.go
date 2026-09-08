package team

import (
	"context"
	"sync"
	"time"

	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
)

type MemoryStore struct {
	mu           sync.RWMutex
	teams        map[id.TeamID]Team
	negotiations map[id.NegotiationID]Negotiation
	turns        map[id.NegotiationID][]Turn
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		teams: make(map[id.TeamID]Team),
		negotiations: make(map[id.NegotiationID]Negotiation),
		turns: make(map[id.NegotiationID][]Turn),
	}
}

func (s *MemoryStore) PutTeam(_ context.Context, value Team) error {
	if value.Proposal.ID == "" { return errs.New(errs.CodeInvalidArgument, "team.memory.put_team", "team id is required") }
	s.mu.Lock(); defer s.mu.Unlock()
	value.Members = append([]Member(nil), value.Members...)
	value.Proposal.Specialists = append([]SpecialistProfile(nil), value.Proposal.Specialists...)
	s.teams[value.Proposal.ID] = value
	return nil
}

func (s *MemoryStore) Team(_ context.Context, teamID id.TeamID) (Team, error) {
	s.mu.RLock(); defer s.mu.RUnlock()
	value, ok := s.teams[teamID]
	if !ok { return Team{}, errs.New(errs.CodeNotFound, "team.memory.team", "team not found") }
	value.Members = append([]Member(nil), value.Members...)
	value.Proposal.Specialists = append([]SpecialistProfile(nil), value.Proposal.Specialists...)
	return value, nil
}

func (s *MemoryStore) PutNegotiation(_ context.Context, value Negotiation) error {
	if value.ID == "" { return errs.New(errs.CodeInvalidArgument, "team.memory.put_negotiation", "negotiation id is required") }
	s.mu.Lock(); defer s.mu.Unlock()
	if _, exists := s.negotiations[value.ID]; exists { return errs.New(errs.CodeConflict, "team.memory.put_negotiation", "negotiation already exists") }
	value.Participants = append([]id.AgentID(nil), value.Participants...)
	s.negotiations[value.ID] = value
	return nil
}

func (s *MemoryStore) Negotiation(_ context.Context, negotiationID id.NegotiationID) (Negotiation, error) {
	s.mu.RLock(); defer s.mu.RUnlock()
	value, ok := s.negotiations[negotiationID]
	if !ok { return Negotiation{}, errs.New(errs.CodeNotFound, "team.memory.negotiation", "negotiation not found") }
	value.Participants = append([]id.AgentID(nil), value.Participants...)
	return value, nil
}

func (s *MemoryStore) Turns(_ context.Context, negotiationID id.NegotiationID, limit int) ([]Turn, error) {
	s.mu.RLock(); defer s.mu.RUnlock()
	if _, ok := s.negotiations[negotiationID]; !ok { return nil, errs.New(errs.CodeNotFound, "team.memory.turns", "negotiation not found") }
	values := s.turns[negotiationID]
	if limit <= 0 || limit > len(values) { limit = len(values) }
	out := make([]Turn, limit)
	copy(out, values[:limit])
	for i := range out { out[i].EvidenceRefs = append([]string(nil), out[i].EvidenceRefs...) }
	return out, nil
}

func (s *MemoryStore) AppendTurn(_ context.Context, negotiationID id.NegotiationID, expectedVersion uint64, turn Turn, state NegotiationState, round int, resolution string, escalatedTo id.AgentID, now time.Time) (Negotiation, error) {
	s.mu.Lock(); defer s.mu.Unlock()
	current, ok := s.negotiations[negotiationID]
	if !ok { return Negotiation{}, errs.New(errs.CodeNotFound, "team.memory.append_turn", "negotiation not found") }
	if current.Version != expectedVersion { return Negotiation{}, errs.New(errs.CodeConflict, "team.memory.append_turn", "negotiation version changed") }
	if turn.Sequence != uint64(len(s.turns[negotiationID])+1) { return Negotiation{}, errs.New(errs.CodeConflict, "team.memory.append_turn", "turn sequence changed") }
	turn.EvidenceRefs = append([]string(nil), turn.EvidenceRefs...)
	s.turns[negotiationID] = append(s.turns[negotiationID], turn)
	current.State = state
	current.Round = round
	current.Resolution = resolution
	current.EscalatedTo = escalatedTo
	current.Version++
	current.UpdatedAt = now
	s.negotiations[negotiationID] = current
	return current, nil
}

var _ Store = (*MemoryStore)(nil)
