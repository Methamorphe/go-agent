package id

import (
	"bytes"
	"strings"
	"testing"
)

func TestGeneratorCreatesTypedAgentID(t *testing.T) {
	random := bytes.NewReader(
		bytes.Repeat([]byte{0xab}, 16),
	)

	generator := NewGeneratorWithReader(random)

	got, err := generator.Agent()
	if err != nil {
		t.Fatalf("Agent() returned an error: %v", err)
	}

	want := AgentID(
		"agt_abababababababababababababababab",
	)

	if got != want {
		t.Fatalf(
			"Agent() = %q, want %q",
			got,
			want,
		)
	}
}

func TestGeneratedIDsHaveExpectedPrefix(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		create func(*Generator) (string, error)
	}{
		{
			name:   "agent",
			prefix: "agt_",
			create: func(g *Generator) (string, error) {
				value, err := g.Agent()
				return value.String(), err
			},
		},
		{
			name:   "event",
			prefix: "evt_",
			create: func(g *Generator) (string, error) {
				value, err := g.Event()
				return value.String(), err
			},
		},
		{
			name:   "world",
			prefix: "wld_",
			create: func(g *Generator) (string, error) {
				value, err := g.World()
				return value.String(), err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			random := bytes.NewReader(
				bytes.Repeat([]byte{0x42}, 16),
			)

			generator := NewGeneratorWithReader(random)

			value, err := test.create(generator)
			if err != nil {
				t.Fatalf("generate id: %v", err)
			}

			if !strings.HasPrefix(value, test.prefix) {
				t.Fatalf(
					"id %q does not start with %q",
					value,
					test.prefix,
				)
			}
		})
	}
}
