package world

import (
	"context"
	"testing"

	"github.com/Methamorphe/go-agent/internal/id"
)

func TestG8SpeculativeWorldDeniesIrreversibleEffectBeforeInnerWorld(t *testing.T) {
	inner := &secureTransactionalSpy{}
	wrapped := NewSpeculativeTransactionalWorld(inner)
	action := Action{ID: id.ActionID("act_g8_network"), AgentID: id.AgentID("agt_g8"), Kind: "network.request", Purpose: "must not escape fork", Effect: CanonicalEffect("network.request")}
	result, err := wrapped.Execute(context.Background(), action)
	if err == nil || result.Status != ResultDenied { t.Fatalf("result=%+v err=%v", result, err) }
	if inner.calls != 0 { t.Fatalf("irreversible action reached inner world: calls=%d", inner.calls) }
}
