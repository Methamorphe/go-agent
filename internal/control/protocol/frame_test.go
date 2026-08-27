package protocol

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
)

func TestFrameRoundTrip(t *testing.T) {
	var buffer bytes.Buffer

	want := Envelope{
		Version:   Version,
		Kind:      KindRequest,
		Type:      "ping",
		RequestID: id.RequestID("req_1"),
	}

	if err := NewEncoder(
		&buffer,
		1024,
	).Encode(want); err != nil {
		t.Fatal(err)
	}

	got, err := NewDecoder(
		&buffer,
		1024,
	).Decode()

	if err != nil {
		t.Fatal(err)
	}

	if got.Type != want.Type ||
		got.RequestID != want.RequestID {
		t.Fatalf(
			"got %#v, want %#v",
			got,
			want,
		)
	}
}

func TestDecoderRejectsOversizedFrameBeforeAllocation(
	t *testing.T,
) {
	var header [4]byte

	binary.BigEndian.PutUint32(
		header[:],
		4096,
	)

	_, err := NewDecoder(
		bytes.NewReader(header[:]),
		1024,
	).Decode()

	if !errs.IsCode(
		err,
		errs.CodeResourceExhausted,
	) {
		t.Fatalf(
			"err = %v",
			err,
		)
	}
}
