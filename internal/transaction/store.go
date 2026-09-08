package transaction

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/world"
)

var (
	ErrNotFound               = errors.New("transaction not found")
	ErrConflict               = errors.New("transaction version conflict")
	ErrInvalidState           = errors.New("invalid transaction state")
	ErrVerificationFailed     = errors.New("transaction verification failed")
	ErrIrreversibleDeferred   = errors.New("irreversible effect deferred until after commit")
	ErrReconciliationRequired = errors.New("transaction requires reconciliation")
)

type Store interface {
	Create(context.Context, Transaction, string, json.RawMessage) error
	Get(context.Context, id.TransactionID) (Transaction, error)
	Transition(context.Context, id.TransactionID, uint64, []State, State, *world.PromotionPlan, string, string, json.RawMessage) (Transaction, error)
	CreateEffect(context.Context, EffectRecord, string, json.RawMessage) error
	UpdateEffect(context.Context, id.EffectRecordID, EffectState, OutcomeCertainty, string, string, json.RawMessage) error
	CreateVerification(context.Context, Verification, string, json.RawMessage) error
	Events(context.Context, id.TransactionID) ([]Event, error)
	ListByStates(context.Context, ...State) ([]Transaction, error)
}

func payload(value any) json.RawMessage {
	if value == nil {
		return nil
	}
	body, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return body
}

func containsState(states []State, state State) bool {
	for _, candidate := range states {
		if candidate == state {
			return true
		}
	}
	return false
}
