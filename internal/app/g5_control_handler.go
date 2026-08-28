package app

import (
	"context"

	"github.com/Methamorphe/go-agent/internal/control"
	controlapi "github.com/Methamorphe/go-agent/internal/control/api"
	"github.com/Methamorphe/go-agent/internal/control/protocol"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/orchestration"
	"github.com/Methamorphe/go-agent/internal/process"
)

type g5OrchestrationCommands interface {
	BootstrapRoot(context.Context, id.AgentID, orchestration.RootPolicy) error
	Spawn(context.Context, orchestration.SpawnSpec) (orchestration.SpawnOutcome, error)
	WaitFor(context.Context, id.AgentID, []id.AgentID, orchestration.WaitMode, int, orchestration.FailurePolicy) (orchestration.WaitSpec, process.State, error)
	Send(context.Context, orchestration.AgentMessage) (orchestration.AgentMessage, bool, error)
	Mailbox(context.Context, id.AgentID, int) ([]orchestration.AgentMessage, error)
	Consume(context.Context, id.AgentID, id.MessageID) (bool, error)
	SettleAndNotify(context.Context, id.SpawnID, orchestration.Budget, orchestration.ChildResult) (int, error)
	ChildTerminal(context.Context, id.AgentID) (int, error)
	CancelTree(context.Context, id.AgentID, string) error
	AdmitFair(context.Context, int) ([]orchestration.Admission, error)
}

func newG5ControlHandler(base control.Handler, service g5OrchestrationCommands) control.Handler {
	return control.HandlerFunc(func(ctx context.Context, request protocol.Envelope) (protocol.Envelope, error) {
		switch request.Type {
		case controlapi.TypeOrchestrationBootstrap:
			payload, err := decodeControlPayload[controlapi.OrchestrationBootstrapRequest](request.Payload)
			if err != nil { return protocol.Envelope{}, err }
			if err := service.BootstrapRoot(ctx, payload.AgentID, payload.Policy); err != nil { return protocol.Envelope{}, err }
			return response(request, controlapi.OrchestrationOKResponse{OK:true})
		case controlapi.TypeOrchestrationSpawn:
			payload, err := decodeControlPayload[controlapi.OrchestrationSpawnRequest](request.Payload)
			if err != nil { return protocol.Envelope{}, err }
			outcome, err := service.Spawn(ctx, payload.Spec)
			if err != nil { return protocol.Envelope{}, err }
			return response(request, controlapi.OrchestrationSpawnResponse{Outcome:outcome})
		case controlapi.TypeOrchestrationWait:
			payload, err := decodeControlPayload[controlapi.OrchestrationWaitRequest](request.Payload)
			if err != nil { return protocol.Envelope{}, err }
			wait, _, err := service.WaitFor(ctx, payload.ParentID, payload.Children, payload.Mode, payload.Quorum, payload.Failure)
			if err != nil { return protocol.Envelope{}, err }
			return response(request, controlapi.OrchestrationWaitResponse{Wait:wait})
		case controlapi.TypeOrchestrationSend:
			payload, err := decodeControlPayload[controlapi.OrchestrationSendRequest](request.Payload)
			if err != nil { return protocol.Envelope{}, err }
			message, inserted, err := service.Send(ctx, payload.Message)
			if err != nil { return protocol.Envelope{}, err }
			return response(request, controlapi.OrchestrationSendResponse{Message:message, Inserted:inserted})
		case controlapi.TypeOrchestrationMailbox:
			payload, err := decodeControlPayload[controlapi.OrchestrationMailboxRequest](request.Payload)
			if err != nil { return protocol.Envelope{}, err }
			messages, err := service.Mailbox(ctx, payload.AgentID, payload.Limit)
			if err != nil { return protocol.Envelope{}, err }
			return response(request, controlapi.OrchestrationMailboxResponse{Messages:messages})
		case controlapi.TypeOrchestrationConsume:
			payload, err := decodeControlPayload[controlapi.OrchestrationConsumeRequest](request.Payload)
			if err != nil { return protocol.Envelope{}, err }
			consumed, err := service.Consume(ctx, payload.AgentID, payload.MessageID)
			if err != nil { return protocol.Envelope{}, err }
			return response(request, controlapi.OrchestrationConsumeResponse{Consumed:consumed})
		case controlapi.TypeOrchestrationSettle:
			payload, err := decodeControlPayload[controlapi.OrchestrationSettleRequest](request.Payload)
			if err != nil { return protocol.Envelope{}, err }
			woken, err := service.SettleAndNotify(ctx, payload.SpawnID, payload.Actual, payload.Result)
			if err != nil { return protocol.Envelope{}, err }
			return response(request, controlapi.OrchestrationChildTerminalResponse{Woken:woken})
		case controlapi.TypeOrchestrationChildTerminal:
			payload, err := decodeControlPayload[controlapi.OrchestrationChildTerminalRequest](request.Payload)
			if err != nil { return protocol.Envelope{}, err }
			woken, err := service.ChildTerminal(ctx, payload.ChildID)
			if err != nil { return protocol.Envelope{}, err }
			return response(request, controlapi.OrchestrationChildTerminalResponse{Woken:woken})
		case controlapi.TypeOrchestrationCancelTree:
			payload, err := decodeControlPayload[controlapi.OrchestrationCancelTreeRequest](request.Payload)
			if err != nil { return protocol.Envelope{}, err }
			if err := service.CancelTree(ctx, payload.AgentID, payload.Reason); err != nil { return protocol.Envelope{}, err }
			return response(request, controlapi.OrchestrationOKResponse{OK:true})
		case controlapi.TypeOrchestrationAdmit:
			payload, err := decodeControlPayload[controlapi.OrchestrationAdmitRequest](request.Payload)
			if err != nil { return protocol.Envelope{}, err }
			admissions, err := service.AdmitFair(ctx, payload.GlobalSlots)
			if err != nil { return protocol.Envelope{}, err }
			return response(request, controlapi.OrchestrationAdmitResponse{Admissions:admissions})
		default:
			return base.Handle(ctx, request)
		}
	})
}
