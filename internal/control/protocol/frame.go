package protocol

import (
	"encoding/binary"
	"encoding/json"
	"io"

	"github.com/Methamorphe/go-agent/internal/errs"
)

type Encoder struct {
	writer io.Writer
	max    uint32
}

type Decoder struct {
	reader io.Reader
	max    uint32
}

func NewEncoder(
	writer io.Writer,
	maxFrameBytes int,
) *Encoder {
	return &Encoder{
		writer: writer,
		max:    uint32(maxFrameBytes),
	}
}

func NewDecoder(
	reader io.Reader,
	maxFrameBytes int,
) *Decoder {
	return &Decoder{
		reader: reader,
		max:    uint32(maxFrameBytes),
	}
}

func (e *Encoder) Encode(
	envelope Envelope,
) error {
	if err := envelope.Validate(); err != nil {
		return err
	}

	body, err := json.Marshal(envelope)
	if err != nil {
		return errs.Wrap(
			errs.CodeInvalidArgument,
			"protocol.encode",
			"marshal envelope",
			err,
		)
	}

	if len(body) == 0 ||
		uint32(len(body)) > e.max {
		return errs.New(
			errs.CodeResourceExhausted,
			"protocol.encode",
			"frame exceeds maximum size",
		)
	}

	var header [4]byte

	binary.BigEndian.PutUint32(
		header[:],
		uint32(len(body)),
	)

	if err := writeFull(
		e.writer,
		header[:],
	); err != nil {
		return errs.Wrap(
			errs.CodeUnavailable,
			"protocol.encode",
			"write frame header",
			err,
		)
	}

	if err := writeFull(
		e.writer,
		body,
	); err != nil {
		return errs.Wrap(
			errs.CodeUnavailable,
			"protocol.encode",
			"write frame body",
			err,
		)
	}

	return nil
}

func (d *Decoder) Decode() (
	Envelope,
	error,
) {
	var header [4]byte

	if _, err := io.ReadFull(
		d.reader,
		header[:],
	); err != nil {
		return Envelope{}, err
	}

	size := binary.BigEndian.Uint32(
		header[:],
	)

	if size == 0 {
		return Envelope{}, errs.New(
			errs.CodeInvalidArgument,
			"protocol.decode",
			"empty frame",
		)
	}

	if size > d.max {
		return Envelope{}, errs.New(
			errs.CodeResourceExhausted,
			"protocol.decode",
			"frame exceeds maximum size",
		)
	}

	body := make(
		[]byte,
		int(size),
	)

	if _, err := io.ReadFull(
		d.reader,
		body,
	); err != nil {
		return Envelope{}, errs.Wrap(
			errs.CodeInvalidArgument,
			"protocol.decode",
			"truncated frame",
			err,
		)
	}

	var envelope Envelope

	if err := json.Unmarshal(
		body,
		&envelope,
	); err != nil {
		return Envelope{}, errs.Wrap(
			errs.CodeInvalidArgument,
			"protocol.decode",
			"invalid JSON envelope",
			err,
		)
	}

	if err := envelope.Validate(); err != nil {
		return Envelope{}, err
	}

	return envelope, nil
}

func writeFull(
	writer io.Writer,
	data []byte,
) error {
	for len(data) > 0 {
		n, err := writer.Write(data)

		if err != nil {
			return err
		}

		if n == 0 {
			return io.ErrShortWrite
		}

		data = data[n:]
	}

	return nil
}
