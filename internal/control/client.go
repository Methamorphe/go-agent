package control

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Methamorphe/go-agent/internal/control/protocol"
	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
)

type RequestIDGenerator interface {
	Request() (id.RequestID, error)
}

type Client struct {
	address  string
	maxFrame int
	ids      RequestIDGenerator
}

func NewClient(
	address string,
	maxFrameBytes int,
	ids RequestIDGenerator,
) *Client {
	return &Client{
		address:  address,
		maxFrame: maxFrameBytes,
		ids:      ids,
	}
}

func (c *Client) Call(
	ctx context.Context,
	messageType string,
	payload any,
	out any,
) error {
	requestID, err :=
		c.ids.Request()
	if err != nil {
		return fmt.Errorf(
			"generate request id: %w",
			err,
		)
	}

	return c.CallWithRequestID(
		ctx,
		requestID,
		messageType,
		payload,
		out,
	)
}

func (c *Client) CallWithRequestID(
	ctx context.Context,
	requestID id.RequestID,
	messageType string,
	payload any,
	out any,
) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf(
			"encode control request: %w",
			err,
		)
	}

	conn, err :=
		dial(
			ctx,
			c.address,
		)
	if err != nil {
		return fmt.Errorf(
			"connect to runtime: %w",
			err,
		)
	}

	defer conn.Close()

	if deadline, ok :=
		ctx.Deadline(); ok {
		_ = conn.SetDeadline(
			deadline,
		)
	}

	encoder :=
		protocol.NewEncoder(
			conn,
			c.maxFrame,
		)

	decoder :=
		protocol.NewDecoder(
			conn,
			c.maxFrame,
		)

	request := protocol.Envelope{
		Version: protocol.Version,
		Kind:    protocol.KindRequest,
		Type:    messageType,

		RequestID: requestID,

		Payload: body,
	}

	if err := encoder.Encode(
		request,
	); err != nil {
		return err
	}

	response, err :=
		decoder.Decode()
	if err != nil {
		return err
	}

	if response.RequestID !=
		requestID ||
		response.Type != messageType {
		return errs.New(
			errs.CodeCorruption,
			"control.client.call",
			"response does not match request",
		)
	}

	if response.Kind ==
		protocol.KindError {
		if response.Error == nil {
			return errs.New(
				errs.CodeCorruption,
				"control.client.call",
				"error response has no payload",
			)
		}

		return errs.New(
			response.Error.Code,
			"control.client.call",
			response.Error.Message,
		)
	}

	if response.Kind !=
		protocol.KindResponse {
		return errs.New(
			errs.CodeCorruption,
			"control.client.call",
			"unexpected response kind",
		)
	}

	if out == nil ||
		len(response.Payload) == 0 {
		return nil
	}

	if err := json.Unmarshal(
		response.Payload,
		out,
	); err != nil {
		return errs.Wrap(
			errs.CodeCorruption,
			"control.client.call",
			"decode response payload",
			err,
		)
	}

	return nil
}

func CallTimeout(
	parent context.Context,
) (
	context.Context,
	context.CancelFunc,
) {
	return context.WithTimeout(
		parent,
		10*time.Second,
	)
}
