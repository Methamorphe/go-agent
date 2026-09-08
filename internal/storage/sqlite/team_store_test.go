package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/team"
)

func TestG11NegotiationSurvivesSQLiteRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "g11.db")
	now := time.Date(2026, 9, 8, 13, 0, 0, 0, time.UTC)

	store, err := Open(ctx, Config{Path: path, MaxOpenConns: 1})
	if err != nil { t.Fatal(err) }
	teamValue := team.Team{
		Proposal: team.Proposal{ID: "tem_restart", RootAgentID: "agt_root", LeadAgentID: "agt_lead", Objective: "resolve disagreement", CreatedAt: now},
		State: team.TeamActive,
		Members: []team.Member{{Role: "reviewer", AgentID: "agt_reviewer"}, {Role: "implementer", AgentID: "agt_implementer"}},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := store.PutTeam(ctx, teamValue); err != nil { t.Fatal(err) }
	n := team.Negotiation{
		ID: "neg_restart", TeamID: teamValue.Proposal.ID, Subject: "race", Participants: []id.AgentID{"agt_reviewer", "agt_implementer"},
		State: team.NegotiationOpen, MaxRounds: 3, MaxTurns: 12, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	if err := store.PutNegotiation(ctx, n); err != nil { t.Fatal(err) }
	claim := team.Turn{Sequence: 1, ActorAgentID: "agt_reviewer", Kind: team.TurnClaim, Statement: "race exists", CreatedAt: now.Add(time.Second)}
	n, err = store.AppendTurn(ctx, n.ID, n.Version, claim, team.NegotiationOpen, 0, "", "", claim.CreatedAt)
	if err != nil { t.Fatal(err) }
	challenge := team.Turn{Sequence: 2, Round: 1, ActorAgentID: "agt_implementer", Kind: team.TurnChallenge, Statement: "show reproducer", ReplyTo: 1, CreatedAt: now.Add(2 * time.Second)}
	n, err = store.AppendTurn(ctx, n.ID, n.Version, challenge, team.NegotiationOpen, 1, "", "", challenge.CreatedAt)
	if err != nil { t.Fatal(err) }
	if err := store.Close(); err != nil { t.Fatal(err) }

	reopened, err := Open(ctx, Config{Path: path, MaxOpenConns: 1})
	if err != nil { t.Fatal(err) }
	defer reopened.Close()

	restored, err := reopened.Negotiation(ctx, n.ID)
	if err != nil { t.Fatal(err) }
	if restored.Version != 3 || restored.Round != 1 || restored.State != team.NegotiationOpen {
		t.Fatalf("unexpected restored negotiation: %#v", restored)
	}
	turns, err := reopened.Turns(ctx, n.ID, 12)
	if err != nil { t.Fatal(err) }
	if len(turns) != 2 || turns[0].Kind != team.TurnClaim || turns[1].Kind != team.TurnChallenge {
		t.Fatalf("unexpected restored turns: %#v", turns)
	}
	if turns[1].ReplyTo != turns[0].Sequence {
		t.Fatalf("causal reply link lost: %#v", turns)
	}
}
