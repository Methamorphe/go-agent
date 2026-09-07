package sqlite

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/scheduler"
)

var _ scheduler.BudgetStore = (*Store)(nil)

func (s *Store) SetLimit(root id.AgentID, limit scheduler.Resources) error {
	if root == "" || !limit.Valid() {
		return errors.New("invalid scheduler budget account")
	}
	result, err := s.db.ExecContext(context.Background(), `
INSERT INTO scheduler_budget_accounts(root_agent_id, limit_money_micros, limit_tokens)
VALUES(?, ?, ?)
ON CONFLICT(root_agent_id) DO UPDATE SET
    limit_money_micros = excluded.limit_money_micros,
    limit_tokens = excluded.limit_tokens
WHERE scheduler_budget_accounts.spent_money_micros + scheduler_budget_accounts.reserved_money_micros <= excluded.limit_money_micros
  AND scheduler_budget_accounts.spent_tokens + scheduler_budget_accounts.reserved_tokens <= excluded.limit_tokens`,
		root.String(), limit.MoneyMicros, limit.Tokens)
	if err != nil {
		return fmt.Errorf("set scheduler budget limit: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read scheduler budget update result: %w", err)
	}
	if rows == 0 {
		return errors.New("new scheduler budget limit below spent and reserved")
	}
	return nil
}

func (s *Store) Snapshot(root id.AgentID) scheduler.BudgetSnapshot {
	var snapshot scheduler.BudgetSnapshot
	if root == "" {
		return snapshot
	}
	var limitMoney, limitTokens, spentMoney, spentTokens, reservedMoney, reservedTokens int64
	err := s.db.QueryRowContext(context.Background(), `
SELECT limit_money_micros, limit_tokens, spent_money_micros, spent_tokens,
       reserved_money_micros, reserved_tokens
FROM scheduler_budget_accounts WHERE root_agent_id = ?`, root.String()).Scan(
		&limitMoney, &limitTokens, &spentMoney, &spentTokens, &reservedMoney, &reservedTokens,
	)
	if err != nil {
		return snapshot
	}
	snapshot.Limit = scheduler.Resources{MoneyMicros: limitMoney, Tokens: limitTokens}
	snapshot.Spent = scheduler.Resources{MoneyMicros: spentMoney, Tokens: spentTokens}
	snapshot.Reserved = scheduler.Resources{MoneyMicros: reservedMoney, Tokens: reservedTokens}
	snapshot.Available = snapshot.Limit.Sub(snapshot.Spent.Add(snapshot.Reserved))
	return snapshot
}

func (s *Store) Reserve(root id.AgentID, amount scheduler.Resources) (scheduler.ReservationID, error) {
	if root == "" || !amount.Valid() {
		return "", errors.New("invalid scheduler reservation")
	}
	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return "", fmt.Errorf("begin scheduler reservation: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `
UPDATE scheduler_budget_accounts
SET reserved_money_micros = reserved_money_micros + ?,
    reserved_tokens = reserved_tokens + ?
WHERE root_agent_id = ?
  AND spent_money_micros + reserved_money_micros + ? <= limit_money_micros
  AND spent_tokens + reserved_tokens + ? <= limit_tokens`,
		amount.MoneyMicros, amount.Tokens, root.String(), amount.MoneyMicros, amount.Tokens)
	if err != nil {
		return "", fmt.Errorf("reserve scheduler budget: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return "", fmt.Errorf("read scheduler reservation result: %w", err)
	}
	if rows == 0 {
		return "", scheduler.ErrBudgetExhausted
	}

	reservationID, err := newSchedulerReservationID()
	if err != nil {
		return "", err
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO scheduler_budget_reservations(
    reservation_id, root_agent_id, amount_money_micros, amount_tokens, settled
) VALUES(?, ?, ?, ?, 0)`, reservationID, root.String(), amount.MoneyMicros, amount.Tokens)
	if err != nil {
		return "", fmt.Errorf("record scheduler reservation: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit scheduler reservation: %w", err)
	}
	return scheduler.ReservationID(reservationID), nil
}

func (s *Store) Reservation(reservationID scheduler.ReservationID) (scheduler.Resources, bool) {
	var amount scheduler.Resources
	var settled int
	err := s.db.QueryRowContext(context.Background(), `
SELECT amount_money_micros, amount_tokens, settled
FROM scheduler_budget_reservations WHERE reservation_id = ?`, string(reservationID)).Scan(
		&amount.MoneyMicros, &amount.Tokens, &settled,
	)
	if err != nil || settled != 0 {
		return scheduler.Resources{}, false
	}
	return amount, true
}

func (s *Store) Settle(reservationID scheduler.ReservationID, actual scheduler.Resources) error {
	if !actual.Valid() {
		return errors.New("invalid scheduler actual resources")
	}
	return s.finishSchedulerReservation(reservationID, actual, true)
}

func (s *Store) Release(reservationID scheduler.ReservationID) error {
	return s.finishSchedulerReservation(reservationID, scheduler.Resources{}, false)
}

func (s *Store) finishSchedulerReservation(reservationID scheduler.ReservationID, actual scheduler.Resources, spend bool) error {
	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return fmt.Errorf("begin scheduler settlement: %w", err)
	}
	defer tx.Rollback()

	var root string
	var amount scheduler.Resources
	var settled int
	err = tx.QueryRowContext(ctx, `
SELECT root_agent_id, amount_money_micros, amount_tokens, settled
FROM scheduler_budget_reservations WHERE reservation_id = ?`, string(reservationID)).Scan(
		&root, &amount.MoneyMicros, &amount.Tokens, &settled,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return scheduler.ErrReservationUnknown
	}
	if err != nil {
		return fmt.Errorf("load scheduler reservation: %w", err)
	}
	if settled != 0 {
		return scheduler.ErrReservationSettled
	}
	if spend && !actual.Fits(amount) {
		return errors.New("actual scheduler usage exceeds reservation")
	}

	result, err := tx.ExecContext(ctx, `
UPDATE scheduler_budget_reservations SET settled = 1
WHERE reservation_id = ? AND settled = 0`, string(reservationID))
	if err != nil {
		return fmt.Errorf("settle scheduler reservation: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read scheduler settlement result: %w", err)
	}
	if rows != 1 {
		return scheduler.ErrReservationSettled
	}

	spentMoney, spentTokens := int64(0), int64(0)
	if spend {
		spentMoney, spentTokens = actual.MoneyMicros, actual.Tokens
	}
	_, err = tx.ExecContext(ctx, `
UPDATE scheduler_budget_accounts
SET reserved_money_micros = reserved_money_micros - ?,
    reserved_tokens = reserved_tokens - ?,
    spent_money_micros = spent_money_micros + ?,
    spent_tokens = spent_tokens + ?
WHERE root_agent_id = ?`, amount.MoneyMicros, amount.Tokens, spentMoney, spentTokens, root)
	if err != nil {
		return fmt.Errorf("update scheduler budget account: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit scheduler settlement: %w", err)
	}
	return nil
}

func newSchedulerReservationID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate scheduler reservation id: %w", err)
	}
	return "br-" + hex.EncodeToString(raw[:]), nil
}
