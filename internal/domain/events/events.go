package events

import (
	"time"

	"github.com/diogo/golangbet/internal/domain/ledger"
	"github.com/diogo/golangbet/internal/domain/money"
	"github.com/diogo/golangbet/internal/domain/wager"
	"github.com/diogo/golangbet/internal/domain/wallet"
)

const (
	TypeProcessed = "WagerTransactionProcessed"
	TypeRejected = "WagerTransactionRejected"
	TypeBalanceChanged = "WalletBalanceChanged"
	TypePendingReference = "WagerTransactionPendingReference"
	EnvelopeVersion = 1
)

type Envelope struct {
	EventID string `json:"eventId"`
	EventType string `json:"eventType"`
	AggregateID string `json:"aggregateId"`
	CorrelationID string `json:"correlationId"`
	CausationID string `json:"causationId,omitempty"`
	OccurredAt time.Time `json:"occurredAt"`
	Version int `json:"version"`
	Data any `json:"data"`
}

type MoneyDTO struct {
	Amount string `json:"amount"`
	Currency string `json:"currency"`
}

func MoneyOf(m money.Money) MoneyDTO { return MoneyDTO{Amount: m.DecimalString(), Currency: m.Currency()} }

type WagerTransactionProcessed struct {
	TransactionID, ProviderID, ExternalTransactionID, WalletID, PlayerID, Kind string
	Money, Balance MoneyDTO
}

type WagerTransactionRejected struct {
	TransactionID, ProviderID, ExternalTransactionID, WalletID, Kind, FailureCode string
	Money MoneyDTO
}

type WalletBalanceChanged struct {
	WalletID, TransactionID, Direction string
	Money, BalanceBefore, BalanceAfter MoneyDTO
	WalletVersion int64
}

type WagerTransactionPendingReference struct {
	TransactionID, ProviderID, ExternalTransactionID, ReferenceExternalID string
}

func NewProcessed(eventID, correlationID string, tx *wager.Transaction, balance money.Money, at time.Time) Envelope {
	return Envelope{EventID: eventID, EventType: TypeProcessed, AggregateID: string(tx.ID()), CorrelationID: correlationID, OccurredAt: at.UTC(), Version: EnvelopeVersion,
		Data: WagerTransactionProcessed{TransactionID: string(tx.ID()), ProviderID: tx.ProviderID(), ExternalTransactionID: tx.ExternalTransactionID(), WalletID: string(tx.WalletID()), PlayerID: string(tx.PlayerID()), Kind: string(tx.Kind()), Money: MoneyOf(tx.Money()), Balance: MoneyOf(balance)}}
}

func NewRejected(eventID, correlationID string, tx *wager.Transaction, at time.Time) Envelope {
	return Envelope{EventID: eventID, EventType: TypeRejected, AggregateID: string(tx.ID()), CorrelationID: correlationID, OccurredAt: at.UTC(), Version: EnvelopeVersion,
		Data: WagerTransactionRejected{TransactionID: string(tx.ID()), ProviderID: tx.ProviderID(), ExternalTransactionID: tx.ExternalTransactionID(), WalletID: string(tx.WalletID()), Kind: string(tx.Kind()), FailureCode: string(tx.FailureCode()), Money: MoneyOf(tx.Money())}}
}

func NewBalanceChanged(eventID, correlationID string, walletID wallet.ID, txID wager.ID, dir ledger.Direction, m, before, after money.Money, version int64, at time.Time) Envelope {
	return Envelope{EventID: eventID, EventType: TypeBalanceChanged, AggregateID: string(walletID), CorrelationID: correlationID, OccurredAt: at.UTC(), Version: EnvelopeVersion,
		Data: WalletBalanceChanged{WalletID: string(walletID), TransactionID: string(txID), Direction: string(dir), Money: MoneyOf(m), BalanceBefore: MoneyOf(before), BalanceAfter: MoneyOf(after), WalletVersion: version}}
}

func NewPendingReference(eventID, correlationID string, tx *wager.Transaction, at time.Time) Envelope {
	return Envelope{EventID: eventID, EventType: TypePendingReference, AggregateID: string(tx.ID()), CorrelationID: correlationID, OccurredAt: at.UTC(), Version: EnvelopeVersion,
		Data: WagerTransactionPendingReference{TransactionID: string(tx.ID()), ProviderID: tx.ProviderID(), ExternalTransactionID: tx.ExternalTransactionID(), ReferenceExternalID: tx.ReferenceExternalID()}}
}
