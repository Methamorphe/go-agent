package scheduler

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/Methamorphe/go-agent/internal/id"
)

type BudgetSnapshot struct {
	Limit     Resources `json:"limit"`
	Spent     Resources `json:"spent"`
	Reserved  Resources `json:"reserved"`
	Available Resources `json:"available"`
}

type BudgetStore interface {
	SetLimit(id.AgentID, Resources) error
	Snapshot(id.AgentID) BudgetSnapshot
	Reserve(id.AgentID, Resources) (ReservationID, error)
	Reservation(ReservationID) (Resources, bool)
	Settle(ReservationID, Resources) error
	Release(ReservationID) error
}

type budgetAccount struct{ limit, spent, reserved Resources }
type reservation struct {
	id      ReservationID
	root    id.AgentID
	amount  Resources
	settled bool
}
type BudgetLedger struct {
	mu           sync.Mutex
	accounts     map[id.AgentID]*budgetAccount
	reservations map[ReservationID]*reservation
	seq          atomic.Uint64
}

var _ BudgetStore = (*BudgetLedger)(nil)

func NewBudgetLedger() *BudgetLedger {
	return &BudgetLedger{accounts: make(map[id.AgentID]*budgetAccount), reservations: make(map[ReservationID]*reservation)}
}
func (b *BudgetLedger) SetLimit(root id.AgentID, limit Resources) error {
	if root == "" || !limit.Valid() {
		return errors.New("invalid budget account")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	a := b.accounts[root]
	if a == nil {
		a = &budgetAccount{}
		b.accounts[root] = a
	}
	if !a.spent.Add(a.reserved).Fits(limit) {
		return errors.New("new limit below spent and reserved")
	}
	a.limit = limit
	return nil
}
func (b *BudgetLedger) Snapshot(root id.AgentID) BudgetSnapshot {
	b.mu.Lock()
	defer b.mu.Unlock()
	a := b.accounts[root]
	if a == nil {
		return BudgetSnapshot{}
	}
	used := a.spent.Add(a.reserved)
	return BudgetSnapshot{Limit: a.limit, Spent: a.spent, Reserved: a.reserved, Available: a.limit.Sub(used)}
}
func (b *BudgetLedger) Reserve(root id.AgentID, amount Resources) (ReservationID, error) {
	if root == "" || !amount.Valid() {
		return "", errors.New("invalid reservation")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	a := b.accounts[root]
	if a == nil {
		return "", ErrBudgetExhausted
	}
	available := a.limit.Sub(a.spent.Add(a.reserved))
	if !amount.Fits(available) {
		return "", ErrBudgetExhausted
	}
	rid := ReservationID(fmt.Sprintf("br-%016x", b.seq.Add(1)))
	a.reserved = a.reserved.Add(amount)
	b.reservations[rid] = &reservation{id: rid, root: root, amount: amount}
	return rid, nil
}
func (b *BudgetLedger) Reservation(id ReservationID) (Resources, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	r, ok := b.reservations[id]
	if !ok {
		return Resources{}, false
	}
	return r.amount, true
}
func (b *BudgetLedger) Settle(id ReservationID, actual Resources) error {
	if !actual.Valid() {
		return errors.New("invalid actual resources")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	r := b.reservations[id]
	if r == nil {
		return ErrReservationUnknown
	}
	if r.settled {
		return ErrReservationSettled
	}
	if !actual.Fits(r.amount) {
		return errors.New("actual usage exceeds reservation")
	}
	a := b.accounts[r.root]
	a.reserved = a.reserved.Sub(r.amount)
	a.spent = a.spent.Add(actual)
	r.settled = true
	return nil
}
func (b *BudgetLedger) Release(id ReservationID) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	r := b.reservations[id]
	if r == nil {
		return ErrReservationUnknown
	}
	if r.settled {
		return ErrReservationSettled
	}
	a := b.accounts[r.root]
	a.reserved = a.reserved.Sub(r.amount)
	r.settled = true
	return nil
}
