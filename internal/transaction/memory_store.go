package transaction

import (
	"context"
	"encoding/json"
	"sort"
	"sync"
	"time"

	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/world"
)

type MemoryStore struct {
	mu            sync.Mutex
	transactions  map[id.TransactionID]Transaction
	effects       map[id.EffectRecordID]EffectRecord
	verifications map[id.VerificationID]Verification
	events        map[id.TransactionID][]Event
	sequence      uint64
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		transactions:  make(map[id.TransactionID]Transaction),
		effects:       make(map[id.EffectRecordID]EffectRecord),
		verifications: make(map[id.VerificationID]Verification),
		events:        make(map[id.TransactionID][]Event),
	}
}

func (s *MemoryStore) Create(_ context.Context, transaction Transaction, eventType string, eventPayload json.RawMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.transactions[transaction.ID]; exists {
		return ErrConflict
	}
	s.transactions[transaction.ID] = cloneTransaction(transaction)
	s.appendEventLocked(transaction.ID, transaction.Version, eventType, eventPayload, transaction.UpdatedAt)
	return nil
}

func (s *MemoryStore) Get(_ context.Context, transactionID id.TransactionID) (Transaction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	transaction, ok := s.transactions[transactionID]
	if !ok {
		return Transaction{}, ErrNotFound
	}
	return cloneTransaction(transaction), nil
}

func (s *MemoryStore) Transition(_ context.Context, transactionID id.TransactionID, expectedVersion uint64, expectedStates []State, newState State, plan *world.PromotionPlan, reconcileReason, eventType string, eventPayload json.RawMessage) (Transaction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	transaction, ok := s.transactions[transactionID]
	if !ok {
		return Transaction{}, ErrNotFound
	}
	if transaction.Version != expectedVersion || !containsState(expectedStates, transaction.State) {
		return Transaction{}, ErrConflict
	}
	transaction.State = newState
	transaction.Version++
	transaction.PreparedPlan = cloneWorldPlan(plan)
	transaction.ReconcileReason = reconcileReason
	transaction.UpdatedAt = time.Now().UTC()
	s.transactions[transactionID] = cloneTransaction(transaction)
	s.appendEventLocked(transactionID, transaction.Version, eventType, eventPayload, transaction.UpdatedAt)
	return cloneTransaction(transaction), nil
}

func (s *MemoryStore) CreateEffect(_ context.Context, effect EffectRecord, eventType string, eventPayload json.RawMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.transactions[effect.TransactionID]; !ok {
		return ErrNotFound
	}
	if _, exists := s.effects[effect.ID]; exists {
		return ErrConflict
	}
	s.effects[effect.ID] = effect
	transaction := s.transactions[effect.TransactionID]
	s.appendEventLocked(effect.TransactionID, transaction.Version, eventType, eventPayload, effect.UpdatedAt)
	return nil
}

func (s *MemoryStore) UpdateEffect(_ context.Context, effectID id.EffectRecordID, state EffectState, certainty OutcomeCertainty, effectError, eventType string, eventPayload json.RawMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	effect, ok := s.effects[effectID]
	if !ok {
		return ErrNotFound
	}
	effect.State = state
	effect.OutcomeCertainty = certainty
	effect.Error = effectError
	effect.UpdatedAt = time.Now().UTC()
	s.effects[effectID] = effect
	transaction := s.transactions[effect.TransactionID]
	s.appendEventLocked(effect.TransactionID, transaction.Version, eventType, eventPayload, effect.UpdatedAt)
	return nil
}

func (s *MemoryStore) CreateVerification(_ context.Context, verification Verification, eventType string, eventPayload json.RawMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	transaction, ok := s.transactions[verification.TransactionID]
	if !ok {
		return ErrNotFound
	}
	if _, exists := s.verifications[verification.ID]; exists {
		return ErrConflict
	}
	s.verifications[verification.ID] = cloneVerification(verification)
	at := verification.StartedAt
	if verification.CompletedAt != nil {
		at = *verification.CompletedAt
	}
	s.appendEventLocked(verification.TransactionID, transaction.Version, eventType, eventPayload, at)
	return nil
}

func (s *MemoryStore) Events(_ context.Context, transactionID id.TransactionID) ([]Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.transactions[transactionID]; !ok {
		return nil, ErrNotFound
	}
	events := s.events[transactionID]
	result := make([]Event, len(events))
	for i, event := range events {
		result[i] = event
		result[i].Payload = append(json.RawMessage(nil), event.Payload...)
	}
	return result, nil
}

func (s *MemoryStore) ListByStates(_ context.Context, states ...State) ([]Transaction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]Transaction, 0)
	for _, transaction := range s.transactions {
		if containsState(states, transaction.State) {
			result = append(result, cloneTransaction(transaction))
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID.String() < result[j].ID.String()
		}
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})
	return result, nil
}

func (s *MemoryStore) Effect(effectID id.EffectRecordID) (EffectRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	effect, ok := s.effects[effectID]
	return effect, ok
}

func (s *MemoryStore) appendEventLocked(transactionID id.TransactionID, version uint64, eventType string, eventPayload json.RawMessage, at time.Time) {
	s.sequence++
	event := Event{
		Sequence:           s.sequence,
		TransactionID:      transactionID,
		TransactionVersion: version,
		Type:               eventType,
		Payload:            append(json.RawMessage(nil), eventPayload...),
		CreatedAt:          at.UTC(),
	}
	s.events[transactionID] = append(s.events[transactionID], event)
}

func cloneTransaction(transaction Transaction) Transaction {
	copy := transaction
	copy.IsolatedWorldRef.Metadata = append(json.RawMessage(nil), transaction.IsolatedWorldRef.Metadata...)
	copy.PreparedPlan = cloneWorldPlan(transaction.PreparedPlan)
	return copy
}

func cloneWorldPlan(plan *world.PromotionPlan) *world.PromotionPlan {
	if plan == nil {
		return nil
	}
	copy := *plan
	copy.Metadata = append(json.RawMessage(nil), plan.Metadata...)
	return &copy
}

func cloneVerification(verification Verification) Verification {
	copy := verification
	copy.Checks = append([]CheckSpec(nil), verification.Checks...)
	copy.Results = append([]CheckResult(nil), verification.Results...)
	if verification.CompletedAt != nil {
		completed := *verification.CompletedAt
		copy.CompletedAt = &completed
	}
	return copy
}
