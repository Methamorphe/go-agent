package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/Methamorphe/go-agent/internal/control"
	controlapi "github.com/Methamorphe/go-agent/internal/control/api"
	"github.com/Methamorphe/go-agent/internal/control/protocol"
	"github.com/Methamorphe/go-agent/internal/diag"
	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/ledger"
	agentprocess "github.com/Methamorphe/go-agent/internal/process"
	"github.com/Methamorphe/go-agent/internal/storage"
)

type healthStore interface {
	Ping(context.Context) error
	Stats() storage.Stats
}

type processCommands interface {
	CreateRoot(
		context.Context,
		string,
		agentprocess.CommandMeta,
	) (agentprocess.State, error)

	Inspect(
		context.Context,
		id.AgentID,
	) (agentprocess.State, error)

	Suspend(
		context.Context,
		id.AgentID,
		*uint64,
		string,
		agentprocess.CommandMeta,
	) (agentprocess.State, error)

	Resume(
		context.Context,
		id.AgentID,
		*uint64,
		string,
		agentprocess.CommandMeta,
	) (agentprocess.State, error)

	Events(
		context.Context,
		id.AgentID,
		uint64,
		int,
	) ([]ledger.Event, error)
}

func newControlHandler(
	store healthStore,
	processes processCommands,
	now func() time.Time,
) control.Handler {
	return control.HandlerFunc(
		func(
			ctx context.Context,
			request protocol.Envelope,
		) (protocol.Envelope, error) {
			switch request.Type {
			case "ping":
				return response(
					request,
					map[string]any{
						"ok": true,
					},
				)

			case "health.get":
				if err := store.Ping(
					ctx,
				); err != nil {
					return protocol.Envelope{},
						err
				}

				return response(
					request,
					map[string]any{
						"ok": true,

						"runtime": diag.Snapshot(
							now(),
						),

						"storage": store.Stats(),
					},
				)

			case controlapi.TypeProcessCreate:
				payload, err :=
					decodeControlPayload[controlapi.ProcessCreateRequest](request.Payload)
				if err != nil {
					return protocol.Envelope{},
						err
				}

				state, err :=
					processes.CreateRoot(
						ctx,
						payload.Intent,
						commandMeta(
							request,
						),
					)
				if err != nil {
					return protocol.Envelope{},
						err
				}

				return response(
					request,
					controlapi.ProcessResponse{
						Process: state,
					},
				)

			case controlapi.TypeProcessInspect:
				payload, err :=
					decodeControlPayload[controlapi.ProcessIDRequest](request.Payload)
				if err != nil {
					return protocol.Envelope{},
						err
				}

				state, err :=
					processes.Inspect(
						ctx,
						payload.AgentID,
					)
				if err != nil {
					return protocol.Envelope{},
						err
				}

				return response(
					request,
					controlapi.ProcessResponse{
						Process: state,
					},
				)

			case controlapi.TypeProcessSuspend:
				payload, err :=
					decodeControlPayload[controlapi.ProcessTransitionRequest](request.Payload)
				if err != nil {
					return protocol.Envelope{},
						err
				}

				state, err :=
					processes.Suspend(
						ctx,
						payload.AgentID,
						payload.ExpectedVersion,
						payload.Reason,
						commandMeta(request),
					)
				if err != nil {
					return protocol.Envelope{},
						err
				}

				return response(
					request,
					controlapi.ProcessResponse{
						Process: state,
					},
				)

			case controlapi.TypeProcessResume:
				payload, err :=
					decodeControlPayload[controlapi.ProcessTransitionRequest](request.Payload)
				if err != nil {
					return protocol.Envelope{},
						err
				}

				state, err :=
					processes.Resume(
						ctx,
						payload.AgentID,
						payload.ExpectedVersion,
						payload.Reason,
						commandMeta(request),
					)
				if err != nil {
					return protocol.Envelope{},
						err
				}

				return response(
					request,
					controlapi.ProcessResponse{
						Process: state,
					},
				)

			case controlapi.TypeProcessEvents:
				payload, err :=
					decodeControlPayload[controlapi.ProcessEventsRequest](request.Payload)
				if err != nil {
					return protocol.Envelope{},
						err
				}

				events, err :=
					processes.Events(
						ctx,
						payload.AgentID,
						payload.AfterVersion,
						payload.Limit,
					)
				if err != nil {
					return protocol.Envelope{},
						err
				}

				return response(
					request,
					controlapi.ProcessEventsResponse{
						Events: events,
					},
				)

			default:
				return protocol.Envelope{},
					errs.New(
						errs.CodeNotFound,
						"control.handle",
						"unknown request type",
					)
			}
		},
	)
}

func commandMeta(
	request protocol.Envelope,
) agentprocess.CommandMeta {
	return agentprocess.CommandMeta{
		RequestID: request.RequestID,

		Actor: ledger.ActorRef{
			Kind: ledger.ActorUser,

			ID: "local-cli",
		},
	}
}

func decodeControlPayload[T any](
	body json.RawMessage,
) (T, error) {
	var value T

	decoder :=
		json.NewDecoder(
			bytes.NewReader(body),
		)

	decoder.DisallowUnknownFields()

	if err := decoder.Decode(
		&value,
	); err != nil {
		return value,
			errs.Wrap(
				errs.CodeInvalidArgument,
				"control.decode_payload",
				"decode request payload",
				err,
			)
	}

	var extra any

	if err := decoder.Decode(
		&extra,
	); err != io.EOF {
		if err == nil {
			return value,
				errs.New(
					errs.CodeInvalidArgument,
					"control.decode_payload",
					"multiple JSON values",
				)
		}

		return value,
			errs.Wrap(
				errs.CodeInvalidArgument,
				"control.decode_payload",
				"decode trailing payload",
				err,
			)
	}

	return value, nil
}

func response(
	request protocol.Envelope,
	payload any,
) (protocol.Envelope, error) {
	body, err :=
		json.Marshal(payload)

	if err != nil {
		return protocol.Envelope{},
			fmt.Errorf(
				"encode response payload: %w",
				err,
			)
	}

	return protocol.Envelope{
		Version: protocol.Version,

		Kind: protocol.KindResponse,

		Type: request.Type,

		RequestID: request.RequestID,

		Payload: body,
	}, nil
}
