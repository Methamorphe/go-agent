package app

import (
	"context"

	"github.com/Methamorphe/go-agent/internal/agent"
	"github.com/Methamorphe/go-agent/internal/control"
	controlapi "github.com/Methamorphe/go-agent/internal/control/api"
	"github.com/Methamorphe/go-agent/internal/control/protocol"
	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/live"
)

type g2AgentCommands interface {
	Run(context.Context, agent.RunRequest) (agent.RunResult, error)
	Live(id.AgentID) (live.Snapshot, bool)
}

func newG2ControlHandler(
	base control.Handler,
	agents g2AgentCommands,
) control.Handler {
	return control.HandlerFunc(
		func(
			ctx context.Context,
			request protocol.Envelope,
		) (protocol.Envelope, error) {
			switch request.Type {
			case controlapi.TypeAgentRun:
				payload, err := decodeControlPayload[controlapi.AgentRunRequest](request.Payload)
				if err != nil {
					return protocol.Envelope{}, err
				}

				result, err := agents.Run(
					ctx,
					agent.RunRequest{
						AgentID:   payload.AgentID,
						Provider:  payload.Provider,
						Model:     payload.Model,
						BaseURL:   payload.BaseURL,
						APIKeyEnv: payload.APIKeyEnv,
						Workspace: payload.Workspace,
						MaxSteps:  payload.MaxSteps,
					},
				)
				if err != nil {
					return protocol.Envelope{}, err
				}

				return response(
					request,
					controlapi.AgentRunResponse{Result: result},
				)

			case controlapi.TypeAgentLive:
				payload, err := decodeControlPayload[controlapi.AgentLiveRequest](request.Payload)
				if err != nil {
					return protocol.Envelope{}, err
				}

				stream, ok := agents.Live(payload.AgentID)
				if !ok {
					return protocol.Envelope{}, errs.New(
						errs.CodeNotFound,
						"agent.live",
						"live stream not found",
					)
				}

				return response(
					request,
					controlapi.AgentLiveResponse{Stream: stream},
				)

			default:
				return base.Handle(ctx, request)
			}
		},
	)
}
