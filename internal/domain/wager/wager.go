package wager

import (
	"time"

	domerr "github.com/diogo/golangbet/internal/domain/errors"
	"github.com/diogo/golangbet/internal/domain/money"
	"github.com/diogo/golangbet/internal/domain/wallet"
)

type ID string
type Origin string
type Kind string
type Status string

const (
	OriginInternal Origin = "INTERNAL"
	OriginExternal Origin = "EXTERNAL"
	KindOpening  Kind = "OPENING"
	KindBet      Kind = "BET"
	KindWin      Kind = "WIN"
	KindLoss     Kind = "LOSS"
	KindRefund   Kind = "REFUND"
	KindRollback Kind = "ROLLBACK"
	StatusPending          Status = "PENDING"
	StatusPendingReference Status = "PENDING_REFERENCE"
	StatusProcessed        Status = "PROCESSED"
	StatusRejected         Status = "REJECTED"
	StatusFailed           Status = "FAILED"
)

func (s Status) Terminal() bool {
	return s == StatusProcessed || s == StatusRejected || s == StatusFailed
}

type Transaction struct {
	id, providerID, externalTransactionID, idempotencyKey, payloadHash, roundID, gameID, referenceExternalID, resultBalanceCurrency string
	origin Origin
	kind Kind
	status Status
	walletID wallet.ID
	playerID wallet.PlayerID
	money money.Money
	referenceInternalID ID
	failureCode domerr.FailureCode
	resultBalanceMinor *int64
	createdAt, updatedAt time.Time
}

type ExternalCreateParams struct {
	ID ID
	Kind Kind
	WalletID wallet.ID
	PlayerID wallet.PlayerID
	ProviderID, ExternalTransactionID, IdempotencyKey, PayloadHash, RoundID, GameID, ReferenceExternalID string
	Money money.Money
	Now time.Time
}

func CreateExternal(p ExternalCreateParams) (*Transaction, error) {
	if p.Kind == KindOpening {
		return nil, domerr.New(domerr.FailureOpeningNotAllowed, domerr.ErrOpeningNotAllowed, "OPENING not allowed externally")
	}
	switch p.Kind {
	case KindBet, KindWin, KindLoss, KindRefund, KindRollback:
	default:
		return nil, domerr.New(domerr.FailureInvalidKind, domerr.ErrInvalidKind, "invalid kind %s", p.Kind)
	}
	if p.ID == "" || p.WalletID == "" || p.PlayerID == "" || p.ProviderID == "" || p.ExternalTransactionID == "" || p.IdempotencyKey == "" || p.PayloadHash == "" || p.RoundID == "" || p.GameID == "" {
		return nil, domerr.New(domerr.FailureValidation, domerr.ErrValidation, "missing external transaction fields")
	}
	if p.Money.Currency() == "" {
		return nil, domerr.New(domerr.FailureInvalidAmount, domerr.ErrInvalidMoney, "money required")
	}
	switch p.Kind {
	case KindLoss:
		if !p.Money.IsZero() {
			return nil, domerr.New(domerr.FailureInvalidAmount, domerr.ErrInvalidMoney, "LOSS requires 0.00")
		}
	case KindBet, KindWin, KindRefund, KindRollback:
		if !p.Money.IsPositive() {
			return nil, domerr.New(domerr.FailureInvalidAmount, domerr.ErrInvalidMoney, "%s requires positive amount", p.Kind)
		}
	}
	if (p.Kind == KindRefund || p.Kind == KindRollback) && p.ReferenceExternalID == "" {
		return nil, domerr.New(domerr.FailureReferenceRequired, domerr.ErrReferenceRequired, "reference required")
	}
	now := p.Now.UTC()
	return &Transaction{
		id: string(p.ID), origin: OriginExternal, kind: p.Kind, status: StatusPending,
		walletID: p.WalletID, playerID: p.PlayerID, providerID: p.ProviderID,
		externalTransactionID: p.ExternalTransactionID, idempotencyKey: p.IdempotencyKey, payloadHash: p.PayloadHash,
		roundID: p.RoundID, gameID: p.GameID, money: p.Money, referenceExternalID: p.ReferenceExternalID,
		createdAt: now, updatedAt: now,
	}, nil
}

type OpeningCreateParams struct {
	ID ID
	WalletID wallet.ID
	PlayerID wallet.PlayerID
	Money money.Money
	Now time.Time
}

func CreateOpening(p OpeningCreateParams) (*Transaction, error) {
	if p.ID == "" || p.WalletID == "" || p.PlayerID == "" {
		return nil, domerr.New(domerr.FailureValidation, domerr.ErrValidation, "opening fields required")
	}
	if !p.Money.IsPositive() {
		return nil, domerr.New(domerr.FailureInvalidAmount, domerr.ErrInvalidMoney, "opening requires positive amount")
	}
	now := p.Now.UTC()
	return &Transaction{id: string(p.ID), origin: OriginInternal, kind: KindOpening, status: StatusPending, walletID: p.WalletID, playerID: p.PlayerID, money: p.Money, createdAt: now, updatedAt: now}, nil
}

type RehydrateParams struct {
	ID ID
	Origin Origin
	Kind Kind
	Status Status
	WalletID wallet.ID
	PlayerID wallet.PlayerID
	ProviderID, ExternalTransactionID, IdempotencyKey, PayloadHash, RoundID, GameID, ReferenceExternalID, ResultBalanceCurrency string
	ReferenceInternalID ID
	FailureCode domerr.FailureCode
	Money money.Money
	ResultBalanceMinor *int64
	CreatedAt, UpdatedAt time.Time
}

func Rehydrate(p RehydrateParams) (*Transaction, error) {
	if p.ID == "" || p.WalletID == "" || p.Status == "" {
		return nil, domerr.New(domerr.FailureValidation, domerr.ErrValidation, "invalid rehydration")
	}
	return &Transaction{
		id: string(p.ID), origin: p.Origin, kind: p.Kind, status: p.Status, walletID: p.WalletID, playerID: p.PlayerID,
		providerID: p.ProviderID, externalTransactionID: p.ExternalTransactionID, idempotencyKey: p.IdempotencyKey,
		payloadHash: p.PayloadHash, roundID: p.RoundID, gameID: p.GameID, money: p.Money,
		referenceExternalID: p.ReferenceExternalID, referenceInternalID: p.ReferenceInternalID, failureCode: p.FailureCode,
		resultBalanceMinor: p.ResultBalanceMinor, resultBalanceCurrency: p.ResultBalanceCurrency,
		createdAt: p.CreatedAt.UTC(), updatedAt: p.UpdatedAt.UTC(),
	}, nil
}

func (t *Transaction) ID() ID { return ID(t.id) }
func (t *Transaction) Origin() Origin { return t.origin }
func (t *Transaction) Kind() Kind { return t.kind }
func (t *Transaction) Status() Status { return t.status }
func (t *Transaction) WalletID() wallet.ID { return t.walletID }
func (t *Transaction) PlayerID() wallet.PlayerID { return t.playerID }
func (t *Transaction) ProviderID() string { return t.providerID }
func (t *Transaction) ExternalTransactionID() string { return t.externalTransactionID }
func (t *Transaction) IdempotencyKey() string { return t.idempotencyKey }
func (t *Transaction) PayloadHash() string { return t.payloadHash }
func (t *Transaction) RoundID() string { return t.roundID }
func (t *Transaction) GameID() string { return t.gameID }
func (t *Transaction) Money() money.Money { return t.money }
func (t *Transaction) ReferenceExternalID() string { return t.referenceExternalID }
func (t *Transaction) ReferenceInternalID() ID { return t.referenceInternalID }
func (t *Transaction) FailureCode() domerr.FailureCode { return t.failureCode }
func (t *Transaction) ResultBalanceMinor() *int64 { return t.resultBalanceMinor }
func (t *Transaction) ResultBalanceCurrency() string { return t.resultBalanceCurrency }
func (t *Transaction) CreatedAt() time.Time { return t.createdAt }
func (t *Transaction) UpdatedAt() time.Time { return t.updatedAt }

func (t *Transaction) ensureTransition(to Status) error {
	if t.status.Terminal() {
		return domerr.New(domerr.FailureValidation, domerr.ErrTerminalState, "terminal state")
	}
	switch t.status {
	case StatusPending:
		if to == StatusProcessed || to == StatusRejected || to == StatusFailed || to == StatusPendingReference {
			return nil
		}
	case StatusPendingReference:
		if to == StatusProcessed || to == StatusRejected || to == StatusFailed {
			return nil
		}
	}
	return domerr.New(domerr.FailureValidation, domerr.ErrInvalidTransition, "cannot transition %s -> %s", t.status, to)
}

func (t *Transaction) MarkProcessed(balance money.Money, now time.Time) error {
	if err := t.ensureTransition(StatusProcessed); err != nil { return err }
	minor := balance.AmountMinor()
	t.status = StatusProcessed
	t.resultBalanceMinor = &minor
	t.resultBalanceCurrency = balance.Currency()
	t.updatedAt = now.UTC()
	return nil
}

func (t *Transaction) MarkRejected(code domerr.FailureCode, balance money.Money, now time.Time) error {
	if err := t.ensureTransition(StatusRejected); err != nil { return err }
	t.status = StatusRejected
	t.failureCode = code
	if balance.Currency() != "" {
		minor := balance.AmountMinor()
		t.resultBalanceMinor = &minor
		t.resultBalanceCurrency = balance.Currency()
	}
	t.updatedAt = now.UTC()
	return nil
}

func (t *Transaction) MarkFailed(code domerr.FailureCode, now time.Time) error {
	if err := t.ensureTransition(StatusFailed); err != nil { return err }
	t.status = StatusFailed
	t.failureCode = code
	t.updatedAt = now.UTC()
	return nil
}

func (t *Transaction) MarkPendingReference(now time.Time) error {
	if err := t.ensureTransition(StatusPendingReference); err != nil { return err }
	t.status = StatusPendingReference
	t.updatedAt = now.UTC()
	return nil
}

func (t *Transaction) SetReferenceInternal(id ID) { t.referenceInternalID = id }
