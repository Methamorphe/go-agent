package invocation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/Methamorphe/go-agent/internal/bounded"
	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/objectstore"
	"github.com/Methamorphe/go-agent/internal/process"
	"github.com/Methamorphe/go-agent/internal/provider"
)

const DefaultPreviewBytes = 64 << 10

type IDGenerator interface {
	Invocation() (id.InvocationID, error)
}

type ProcessRecorder interface {
	ModelInvocationStarted(context.Context, id.AgentID, *uint64, process.ModelInvocationStartedPayload, process.CommandMeta) (process.State, error)
	ModelInvocationCompleted(context.Context, id.AgentID, *uint64, process.ModelInvocationCompletedPayload, process.CommandMeta) (process.State, error)
	ModelInvocationFailed(context.Context, id.AgentID, *uint64, process.ModelInvocationFailedPayload, process.CommandMeta) (process.State, error)
}

type ObjectStore interface {
	Put(context.Context, io.Reader) (objectstore.Meta, error)
}

type LivePublisher interface {
	Publish(id.AgentID, string)
}

type Service struct {
	processes ProcessRecorder
	objects   ObjectStore
	ids       IDGenerator
	live      LivePublisher
	preview   int
}

type Outcome struct {
	InvocationID  id.InvocationID `json:"invocation_id"`
	ResponseRef   string          `json:"response_ref"`
	TextPreview   string          `json:"text_preview"`
	TextTruncated bool            `json:"text_truncated"`
	Result        provider.Result `json:"result"`
}

func New(processes ProcessRecorder, objects ObjectStore, ids IDGenerator, liveBus LivePublisher) *Service {
	return &Service{
		processes: processes,
		objects:   objects,
		ids:       ids,
		live:      liveBus,
		preview:   DefaultPreviewBytes,
	}
}

func (s *Service) Invoke(
	ctx context.Context,
	model provider.Provider,
	agentID id.AgentID,
	modelName string,
	messages []provider.Message,
	tools []provider.ToolDefinition,
	expected *uint64,
	meta process.CommandMeta,
) (Outcome, process.State, error) {
	invocationID, err := s.ids.Invocation()
	if err != nil {
		return Outcome{}, process.State{}, errs.Wrap(errs.CodeInternal, "invocation.invoke", "generate invocation id", err)
	}

	req := provider.Request{
		AgentID:      agentID,
		InvocationID: invocationID,
		Model:        modelName,
		Messages:     messages,
		Tools:        tools,
	}
	if err := req.Validate(); err != nil {
		return Outcome{}, process.State{}, err
	}

	requestBody, err := json.Marshal(req)
	if err != nil {
		return Outcome{}, process.State{}, errs.Wrap(errs.CodeInvalidArgument, "invocation.invoke", "encode request artifact", err)
	}
	requestObject, err := s.objects.Put(ctx, bytes.NewReader(requestBody))
	if err != nil {
		return Outcome{}, process.State{}, err
	}

	started, err := s.processes.ModelInvocationStarted(
		ctx,
		agentID,
		expected,
		process.ModelInvocationStartedPayload{
			InvocationID: invocationID,
			Provider:     model.Name(),
			Model:        modelName,
			RequestRef:   string(requestObject.Ref),
		},
		meta,
	)
	if err != nil {
		return Outcome{}, process.State{}, err
	}

	reader, writer := io.Pipe()
	persistCtx, cancelPersist := context.WithCancel(context.WithoutCancel(ctx))
	defer cancelPersist()

	type persisted struct {
		meta objectstore.Meta
		err  error
	}
	persistedCh := make(chan persisted, 1)
	go func() {
		object, putErr := s.objects.Put(persistCtx, reader)
		_ = reader.CloseWithError(putErr)
		persistedCh <- persisted{meta: object, err: putErr}
	}()

	preview := bounded.NewPreview(s.preview)
	streamResult, streamErr := model.Stream(ctx, req, func(event provider.Event) error {
		if err := event.Validate(); err != nil {
			return err
		}
		if event.Kind != provider.EventTextDelta || event.Text == "" {
			return nil
		}
		if _, err := preview.Write([]byte(event.Text)); err != nil {
			return err
		}
		if s.live != nil {
			s.live.Publish(agentID, event.Text)
		}
		_, err := writer.Write([]byte(event.Text))
		return err
	})

	closeErr := writer.Close()
	stored := <-persistedCh
	_ = reader.Close()

	if streamErr == nil && closeErr != nil {
		streamErr = closeErr
	}
	if streamErr == nil && stored.err != nil {
		streamErr = stored.err
	}

	outcome := Outcome{
		InvocationID:  invocationID,
		TextPreview:   preview.String(),
		TextTruncated: preview.Truncated(),
		Result:        streamResult,
	}
	if stored.err == nil {
		outcome.ResponseRef = string(stored.meta.Ref)
	}

	if streamErr != nil {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		failure := streamErr.Error()
		if len(failure) > 2048 {
			failure = failure[:2048]
		}
		expectedVersion := started.Version
		failed, recordErr := s.processes.ModelInvocationFailed(
			cleanupCtx,
			agentID,
			&expectedVersion,
			process.ModelInvocationFailedPayload{
				InvocationID:       invocationID,
				PartialResponseRef: outcome.ResponseRef,
				Failure:            failure,
			},
			meta,
		)
		if recordErr != nil {
			return outcome, started, errors.Join(streamErr, recordErr)
		}
		return outcome, failed, streamErr
	}

	if outcome.ResponseRef == "" {
		return outcome, started, errs.New(errs.CodeUnavailable, "invocation.invoke", "response artifact was not persisted")
	}

	expectedVersion := started.Version
	completed, err := s.processes.ModelInvocationCompleted(
		ctx,
		agentID,
		&expectedVersion,
		process.ModelInvocationCompletedPayload{
			InvocationID:     invocationID,
			ResponseRef:      outcome.ResponseRef,
			ProviderResultID: streamResult.ProviderResult,
			FinishReason:     string(streamResult.FinishReason),
			Usage: process.ModelUsage{
				InputTokens:  streamResult.Usage.InputTokens,
				OutputTokens: streamResult.Usage.OutputTokens,
				TotalTokens:  streamResult.Usage.TotalTokens,
			},
			ToolCallCount: len(streamResult.ToolCalls),
		},
		meta,
	)
	if err != nil {
		return outcome, started, err
	}
	return outcome, completed, nil
}
