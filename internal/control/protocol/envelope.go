package protocol

import (
	"encoding/json"
	"fmt"

	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
)

const Version uint16 = 1

type Kind string

const (
	KindRequest  Kind = "request"
	KindResponse Kind = "response"
	KindError    Kind = "error"
	KindEvent    Kind = "event"
)

type Envelope struct {
	Version   uint16          `json:"version"`
	Kind      Kind            `json:"kind"`
	Type      string          `json:"type"`
	RequestID id.RequestID    `json:"request_id,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	Error     *ErrorPayload   `json:"error,omitempty"`
}

type ErrorPayload struct {
	Code    errs.Code `json:"code"`
	Message string    `json:"message"`
}

func (e Envelope) Validate() error {
	if e.Version != Version {
		return errs.New(
			errs.CodeUnsupported,
			"protocol.validate",
			fmt.Sprintf(
				"unsupported protocol version %d",
				e.Version,
			),
		)
	}

	switch e.Kind {
	case KindRequest,
		KindResponse,
		KindError,
		KindEvent:
	default:
		return errs.New(
			errs.CodeInvalidArgument,
			"protocol.validate",
			"invalid envelope kind",
		)
	}

	if e.Type == "" {
		return errs.New(
			errs.CodeInvalidArgument,
			"protocol.validate",
			"message type is empty",
		)
	}

	if (e.Kind == KindRequest ||
		e.Kind == KindResponse ||
		e.Kind == KindError) &&
		e.RequestID == "" {
		return errs.New(
			errs.CodeInvalidArgument,
			"protocol.validate",
			"request_id is required",
		)
	}

	if e.Kind == KindError &&
		e.Error == nil {
		return errs.New(
			errs.CodeInvalidArgument,
			"protocol.validate",
			"error payload is required",
		)
	}

	return nil
}

func ErrorResponse(
	request Envelope,
	err error,
) Envelope {
	return Envelope{
		Version:   Version,
		Kind:      KindError,
		Type:      request.Type,
		RequestID: request.RequestID,

		Error: &ErrorPayload{
			Code:    errs.CodeOf(err),
			Message: err.Error(),
		},
	}
}
