package wallet

import (
	"time"

	domerr "github.com/diogo/golangbet/internal/domain/errors"
	"github.com/diogo/golangbet/internal/domain/money"
)

type ID string
type PlayerID string

type Wallet struct {
	id        ID
	playerID  PlayerID
	currency  string
	balance   money.Money
	version   int64
	createdAt time.Time
	updatedAt time.Time
}

type CreateParams struct {
	ID             ID
	PlayerID       PlayerID
	InitialBalance money.Money
	Now            time.Time
}

func Create(p CreateParams) (*Wallet, error) {
	if p.ID == "" || p.PlayerID == "" {
		return nil, domerr.New(domerr.FailureValidation, domerr.ErrInvalidWallet, "wallet id and player id required")
	}
	if p.InitialBalance.Currency() == "" {
		return nil, domerr.New(domerr.FailureInvalidAmount, domerr.ErrInvalidMoney, "initial balance required")
	}
	if p.InitialBalance.IsNegative() {
		return nil, domerr.New(domerr.FailureInvalidAmount, domerr.ErrInvalidMoney, "initial balance cannot be negative")
	}
	now := p.Now.UTC()
	return &Wallet{
		id: p.ID, playerID: p.PlayerID, currency: p.InitialBalance.Currency(),
		balance: p.InitialBalance, version: 1, createdAt: now, updatedAt: now,
	}, nil
}

func Rehydrate(id ID, playerID PlayerID, balance money.Money, version int64, createdAt, updatedAt time.Time) (*Wallet, error) {
	if id == "" || playerID == "" || balance.Currency() == "" || version < 1 {
		return nil, domerr.New(domerr.FailureValidation, domerr.ErrInvalidWallet, "invalid wallet rehydration")
	}
	if balance.IsNegative() {
		return nil, domerr.New(domerr.FailureInvalidAmount, domerr.ErrInvalidMoney, "balance cannot be negative")
	}
	return &Wallet{id: id, playerID: playerID, currency: balance.Currency(), balance: balance, version: version, createdAt: createdAt.UTC(), updatedAt: updatedAt.UTC()}, nil
}

func (w *Wallet) ID() ID               { return w.id }
func (w *Wallet) PlayerID() PlayerID   { return w.playerID }
func (w *Wallet) Currency() string     { return w.currency }
func (w *Wallet) Balance() money.Money { return w.balance }
func (w *Wallet) Version() int64       { return w.version }
func (w *Wallet) CreatedAt() time.Time { return w.createdAt }
func (w *Wallet) UpdatedAt() time.Time { return w.updatedAt }

type BalanceChange struct {
	Before money.Money
	After  money.Money
}

func (w *Wallet) Debit(amount money.Money, now time.Time) (BalanceChange, error) {
	if err := w.ensureCurrency(amount); err != nil {
		return BalanceChange{}, err
	}
	if !amount.IsPositive() {
		return BalanceChange{}, domerr.New(domerr.FailureInvalidAmount, domerr.ErrInvalidMoney, "debit requires positive amount")
	}
	cmp, err := w.balance.Cmp(amount)
	if err != nil {
		return BalanceChange{}, err
	}
	if cmp < 0 {
		return BalanceChange{}, domerr.New(domerr.FailureInsufficientFunds, domerr.ErrInsufficientFunds, "insufficient funds")
	}
	after, err := w.balance.Sub(amount)
	if err != nil {
		return BalanceChange{}, err
	}
	before := w.balance
	w.balance = after
	w.version++
	w.updatedAt = now.UTC()
	return BalanceChange{Before: before, After: after}, nil
}

func (w *Wallet) Credit(amount money.Money, now time.Time) (BalanceChange, error) {
	if err := w.ensureCurrency(amount); err != nil {
		return BalanceChange{}, err
	}
	if !amount.IsPositive() {
		return BalanceChange{}, domerr.New(domerr.FailureInvalidAmount, domerr.ErrInvalidMoney, "credit requires positive amount")
	}
	after, err := w.balance.Add(amount)
	if err != nil {
		return BalanceChange{}, err
	}
	before := w.balance
	w.balance = after
	w.version++
	w.updatedAt = now.UTC()
	return BalanceChange{Before: before, After: after}, nil
}

func (w *Wallet) DebitReversal(amount money.Money, now time.Time) (BalanceChange, error) {
	if err := w.ensureCurrency(amount); err != nil {
		return BalanceChange{}, err
	}
	if !amount.IsPositive() {
		return BalanceChange{}, domerr.New(domerr.FailureInvalidAmount, domerr.ErrInvalidMoney, "reversal debit requires positive amount")
	}
	cmp, err := w.balance.Cmp(amount)
	if err != nil {
		return BalanceChange{}, err
	}
	if cmp < 0 {
		return BalanceChange{}, domerr.New(domerr.FailureInsufficientFundsReversal, domerr.ErrInsufficientFundsReversal, "insufficient funds for reversal")
	}
	after, err := w.balance.Sub(amount)
	if err != nil {
		return BalanceChange{}, err
	}
	before := w.balance
	w.balance = after
	w.version++
	w.updatedAt = now.UTC()
	return BalanceChange{Before: before, After: after}, nil
}

func (w *Wallet) ensureCurrency(amount money.Money) error {
	if amount.Currency() != w.currency {
		return domerr.New(domerr.FailureCurrencyMismatch, domerr.ErrCurrencyMismatch, "movement currency must match wallet")
	}
	return nil
}
