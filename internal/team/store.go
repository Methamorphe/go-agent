package team

import (
	"context"
	"time"

	"github.com/Methamorphe/go-agent/internal/id"
)

type Store interface {
	PutTeam(context.Context, Team) error
	Team(context.Context, id.TeamID) (Team, error)
	PutNegotiation(context.Context, Negotiation) error
	Negotiation(context.Context, id.NegotiationID) (Negotiation, error)
	Turns(context.Context, id.NegotiationID, int) ([]Turn, error)
	AppendTurn(context.Context, id.NegotiationID, uint64, Turn, NegotiationState, int, string, id.AgentID, time.Time) (Negotiation, error)
}
