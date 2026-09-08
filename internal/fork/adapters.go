package fork

import (
	"context"

	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/world"
)

type AuthorityProviderFunc func(context.Context, id.AgentID) (world.Intent, []world.Capability, error)

func (f AuthorityProviderFunc) CurrentAuthority(ctx context.Context, agentID id.AgentID) (world.Intent, []world.Capability, error) {
	return f(ctx, agentID)
}

type CognitivePromoterFunc func(context.Context, string, Checkpoint, Branch, OverlayEntry) error

func (f CognitivePromoterFunc) PromoteForkArtifact(ctx context.Context, operationKey string, checkpoint Checkpoint, branch Branch, entry OverlayEntry) error {
	return f(ctx, operationKey, checkpoint, branch, entry)
}
