package ledger

import (
	"time"

	domerr "github.com/diogo/golangbet/internal/domain/errors"
	"github.com/diogo/golangbet/internal/domain/money"
	"github.com/diogo/golangbet/internal/domain/wallet"
)

type Direction string

const (
	Debit  Direction = "DEBIT"
	Credit Direction = "CREDIT"
)

type EntryID string
type TransactionID string

type Entry struct {
	id            EntryID
	walletID      wallet.ID
	transactionID TransactionID
	direction     Direction
	money         money.Money
	balanceBefore money.Money
	balanceAfter  money.Money
	createdAt     time.Time
}

type CreateParams struct {
	ID            EntryID
	WalletID      wallet.ID
	TransactionID TransactionID
	Direction     Direction
	Money         money.Money
	BalanceBefore money.Money
	BalanceAfter  money.Money
	Now           time.Time
}

func Create(p CreateParams) (*Entry, error) {
	if p.ID == "" || p.WalletID == "" || p.TransactionID == "" {
		return nil, domerr.New(domerr.FailureValidation, domerr.ErrValidation, "ledger ids required")
	}
	if p.Direction != Debit && p.Direction != Credit {
		return nil, domerr.New(domerr.FailureValidation, domerr.ErrValidation, "invalid direction")
	}
	if !p.Money.IsPositive() {
		return nil, domerr.New(domerr.FailureInvalidAmount, domerr.ErrInvalidMoney, "ledger money must be positive")
	}
	var expected money.Money
	var err error
	switch p.Direction {
	case Debit:
		expected, err = p.BalanceBefore.Sub(p.Money)
	case Credit:
		expected, err = p.BalanceBefore.Add(p.Money)
	}
	if err != nil {
		return nil, err
	}
	if !expected.Equal(p.BalanceAfter) {
		return nil, domerr.New(domerr.FailureValidation, domerr.ErrValidation, "balanceAfter mismatch")
	}
	return &Entry{
		id: p.ID, walletID: p.WalletID, transactionID: p.TransactionID, direction: p.Direction,
		money: p.Money, balanceBefore: p.BalanceBefore, balanceAfter: p.BalanceAfter, createdAt: p.Now.UTC(),
	}, nil
}

func Rehydrate(id EntryID, walletID wallet.ID, txID TransactionID, dir Direction, m, before, after money.Money, createdAt time.Time) (*Entry, error) {
	return Create(CreateParams{ID: id, WalletID: walletID, TransactionID: txID, Direction: dir, Money: m, BalanceBefore: before, BalanceAfter: after, Now: createdAt})
}

func (e *Entry) ID() EntryID                   { return e.id }
func (e *Entry) WalletID() wallet.ID           { return e.walletID }
func (e *Entry) TransactionID() TransactionID { return e.transactionID }
func (e *Entry) Direction() Direction          { return e.direction }
func (e *Entry) Money() money.Money            { return e.money }
func (e *Entry) BalanceBefore() money.Money    { return e.balanceBefore }
func (e *Entry) BalanceAfter() money.Money     { return e.balanceAfter }
func (e *Entry) CreatedAt() time.Time          { return e.createdAt }
