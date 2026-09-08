package team

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Methamorphe/go-agent/internal/clock"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/orchestration"
	"github.com/Methamorphe/go-agent/internal/process"
)

type fakeIDs struct { teams, negotiations int }
func (f *fakeIDs) Team() (id.TeamID, error) { f.teams++; return id.TeamID(fmt.Sprintf("tem_%d", f.teams)), nil }
func (f *fakeIDs) Negotiation() (id.NegotiationID, error) { f.negotiations++; return id.NegotiationID(fmt.Sprintf("neg_%d", f.negotiations)), nil }

type fakeAdmitter struct { decision AdmissionDecision }
func (f fakeAdmitter) AdmitTeam(context.Context, Proposal) (AdmissionDecision, error) { return f.decision, nil }

type fakeSpawner struct { calls int }
func (f *fakeSpawner) Spawn(_ context.Context, spec orchestration.SpawnSpec) (orchestration.SpawnOutcome, error) {
	f.calls++
	agentID := id.AgentID(fmt.Sprintf("agt_member_%d", f.calls))
	spawnID := id.SpawnID(fmt.Sprintf("spn_%d", f.calls))
	return orchestration.SpawnOutcome{
		Spawn: orchestration.SpawnRecord{ID: spawnID, ParentAgentID: spec.ParentAgentID, ChildAgentID: agentID},
		Child: process.State{AgentID: agentID, RootAgentID: "agt_root", Status: process.StatusReady},
	}, nil
}

func TestKillerReviewerEvidenceConverges(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 8, 11, 0, 0, 0, time.UTC)
	store := NewMemoryStore()
	spawner := &fakeSpawner{}
	service, err := New(store, spawner, fakeAdmitter{AdmissionDecision{Admitted: true}}, &fakeIDs{}, clock.NewFakeClock(now))
	if err != nil { t.Fatal(err) }

	proposed, err := service.Propose(ctx, Proposal{
		RootAgentID: "agt_root", LeadAgentID: "agt_lead", Objective: "resolve cache race",
		Specialists: []SpecialistProfile{
			{Role: "reviewer", TaskIntent: "review cache concurrency", Budget: orchestration.Budget{Tokens: 1000}},
			{Role: "implementer", TaskIntent: "fix verified cache races", Budget: orchestration.Budget{Tokens: 1000}},
		},
		Limits: Limits{MaxMembers: 2, MaxParallelism: 2, MaxRounds: 3, MaxTurns: 12},
	})
	if err != nil { t.Fatal(err) }
	formed, err := service.Form(ctx, proposed.Proposal.ID)
	if err != nil { t.Fatal(err) }
	if formed.State != TeamActive || len(formed.Members) != 2 { t.Fatalf("unexpected formed team: %#v", formed) }

	reviewer := formed.Members[0].AgentID
	implementer := formed.Members[1].AgentID
	negotiation, err := service.StartNegotiation(ctx, formed.Proposal.ID, "cache invalidation overlaps refresh", []id.AgentID{reviewer, implementer})
	if err != nil { t.Fatal(err) }

	negotiation, claim, err := service.Turn(ctx, negotiation.ID, reviewer, TurnClaim, "refresh and invalidation can race", nil, 0)
	if err != nil { t.Fatal(err) }
	negotiation, challenge, err := service.Turn(ctx, negotiation.ID, implementer, TurnChallenge, "provide a deterministic reproducer", nil, claim.Sequence)
	if err != nil { t.Fatal(err) }
	negotiation, evidence, err := service.Turn(ctx, negotiation.ID, reviewer, TurnEvidence, "race reproduces under overlap", []string{"evidence://race-reproducer-42"}, challenge.Sequence)
	if err != nil { t.Fatal(err) }
	negotiation, revision, err := service.Turn(ctx, negotiation.ID, implementer, TurnRevision, "confirmed; serialize publish with invalidation", nil, evidence.Sequence)
	if err != nil { t.Fatal(err) }
	negotiation, _, err = service.Turn(ctx, negotiation.ID, reviewer, TurnConcede, "race confirmed and remediation agreed", nil, revision.Sequence)
	if err != nil { t.Fatal(err) }
	if negotiation.State != NegotiationConverged { t.Fatalf("want CONVERGED, got %s", negotiation.State) }
	if negotiation.Resolution == "" { t.Fatal("expected explicit resolution") }
	turns, err := store.Turns(ctx, negotiation.ID, 32)
	if err != nil { t.Fatal(err) }
	if len(turns) != 5 { t.Fatalf("want 5 durable turns, got %d", len(turns)) }
	if len(turns[2].EvidenceRefs) != 1 || turns[2].EvidenceRefs[0] != "evidence://race-reproducer-42" { t.Fatalf("evidence not preserved: %#v", turns[2]) }
}

func TestNegotiationEscalatesAtRoundLimit(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	spawner := &fakeSpawner{}
	service, err := New(store, spawner, fakeAdmitter{AdmissionDecision{Admitted: true}}, &fakeIDs{}, clock.NewFakeClock(time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)))
	if err != nil { t.Fatal(err) }
	proposed, err := service.Propose(ctx, Proposal{
		RootAgentID: "agt_root", LeadAgentID: "agt_lead", Objective: "resolve disagreement",
		Specialists: []SpecialistProfile{
			{Role: "reviewer", TaskIntent: "review", Budget: orchestration.Budget{Tokens: 100}},
			{Role: "implementer", TaskIntent: "implement", Budget: orchestration.Budget{Tokens: 100}},
		}, Limits: Limits{MaxMembers: 2, MaxRounds: 1, MaxTurns: 8},
	})
	if err != nil { t.Fatal(err) }
	formed, err := service.Form(ctx, proposed.Proposal.ID); if err != nil { t.Fatal(err) }
	a, b := formed.Members[0].AgentID, formed.Members[1].AgentID
	n, err := service.StartNegotiation(ctx, formed.Proposal.ID, "bounded dispute", []id.AgentID{a, b}); if err != nil { t.Fatal(err) }
	_, claim, err := service.Turn(ctx, n.ID, a, TurnClaim, "claim", nil, 0); if err != nil { t.Fatal(err) }
	_, challenge, err := service.Turn(ctx, n.ID, b, TurnChallenge, "challenge", nil, claim.Sequence); if err != nil { t.Fatal(err) }
	_, evidence, err := service.Turn(ctx, n.ID, a, TurnEvidence, "proof", []string{"evidence://1"}, challenge.Sequence); if err != nil { t.Fatal(err) }
	_, revision, err := service.Turn(ctx, n.ID, a, TurnRevision, "revised claim", nil, evidence.Sequence); if err != nil { t.Fatal(err) }
	n, escalation, err := service.Turn(ctx, n.ID, b, TurnChallenge, "still disputed", nil, revision.Sequence); if err != nil { t.Fatal(err) }
	if n.State != NegotiationEscalated { t.Fatalf("want ESCALATED, got %s", n.State) }
	if escalation.Kind != TurnEscalation || n.EscalatedTo != "agt_lead" { t.Fatalf("unexpected escalation: %#v / %#v", escalation, n) }
	currentTeam, err := store.Team(ctx, formed.Proposal.ID); if err != nil { t.Fatal(err) }
	if currentTeam.State != TeamEscalated { t.Fatalf("want team ESCALATED, got %s", currentTeam.State) }
}

func TestRejectedTeamNeverSpawns(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	spawner := &fakeSpawner{}
	service, err := New(store, spawner, fakeAdmitter{AdmissionDecision{Admitted: false, Reason: "scheduler capacity exhausted"}}, &fakeIDs{}, clock.NewFakeClock(time.Now()))
	if err != nil { t.Fatal(err) }
	proposed, err := service.Propose(ctx, Proposal{RootAgentID: "agt_root", LeadAgentID: "agt_lead", Objective: "work", Specialists: []SpecialistProfile{{Role: "one", TaskIntent: "work", Budget: orchestration.Budget{Tokens: 1}}}})
	if err != nil { t.Fatal(err) }
	formed, err := service.Form(ctx, proposed.Proposal.ID)
	if err != nil { t.Fatal(err) }
	if formed.State != TeamRejected { t.Fatalf("want REJECTED, got %s", formed.State) }
	if spawner.calls != 0 { t.Fatalf("scheduler rejection must happen before spawn, got %d calls", spawner.calls) }
}
