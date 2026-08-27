package control

import (
	"context"

	"github.com/Methamorphe/go-agent/internal/control/protocol"
)

type Handler interface {
	Handle(
		context.Context,
		protocol.Envelope,
	) (protocol.Envelope, error)
}

type HandlerFunc func(
	context.Context,
	protocol.Envelope,
) (protocol.Envelope, error)

func (f HandlerFunc) Handle(
	ctx context.Context,
	envelope protocol.Envelope,
) (protocol.Envelope, error) {
	return f(ctx, envelope)
}
