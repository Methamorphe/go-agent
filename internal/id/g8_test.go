package id

import (
	"bytes"
	"strings"
	"testing"
)

func TestG8ForkIDsHaveDedicatedNamespaces(t *testing.T) {
	for _, tc := range []struct {
		name string
		prefix string
		makeID func(*Generator) (string, error)
	}{
		{name: "fork_group", prefix: "fkg_", makeID: func(g *Generator) (string, error) { v, err := g.ForkGroup(); return v.String(), err }},
		{name: "fork", prefix: "frk_", makeID: func(g *Generator) (string, error) { v, err := g.Fork(); return v.String(), err }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := NewGeneratorWithReader(bytes.NewReader(bytes.Repeat([]byte{0x73}, 16)))
			got, err := tc.makeID(g)
			if err != nil { t.Fatal(err) }
			if !strings.HasPrefix(got, tc.prefix) { t.Fatalf("id=%q want prefix %q", got, tc.prefix) }
		})
	}
}
