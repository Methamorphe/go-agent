package id

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
)

type Generator struct {
	random io.Reader
}

func NewGenerator() *Generator {
	return &Generator{
		random: rand.Reader,
	}
}

func NewGeneratorWithReader(random io.Reader) *Generator {
	return &Generator{
		random: random,
	}
}

func (g *Generator) Agent() (AgentID, error) {
	value, err := g.generate("agt")
	if err != nil {
		return "", err
	}

	return AgentID(value), nil
}

func (g *Generator) Event() (EventID, error) {
	value, err := g.generate("evt")
	if err != nil {
		return "", err
	}

	return EventID(value), nil
}

func (g *Generator) Invocation() (InvocationID, error) {
	value, err := g.generate("inv")
	if err != nil {
		return "", err
	}

	return InvocationID(value), nil
}

func (g *Generator) Action() (ActionID, error) {
	value, err := g.generate("act")
	if err != nil {
		return "", err
	}

	return ActionID(value), nil
}

func (g *Generator) Transaction() (TransactionID, error) {
	value, err := g.generate("tx")
	if err != nil {
		return "", err
	}

	return TransactionID(value), nil
}

func (g *Generator) World() (WorldID, error) {
	value, err := g.generate("wld")
	if err != nil {
		return "", err
	}

	return WorldID(value), nil
}

func (g *Generator) Correlation() (CorrelationID, error) {
	value, err := g.generate("cor")
	if err != nil {
		return "", err
	}

	return CorrelationID(value), nil
}

func (g *Generator) generate(prefix string) (string, error) {
	var bytes [16]byte

	if _, err := io.ReadFull(g.random, bytes[:]); err != nil {
		return "", fmt.Errorf("generate %s id: %w", prefix, err)
	}

	return prefix + "_" + hex.EncodeToString(bytes[:]), nil
}
