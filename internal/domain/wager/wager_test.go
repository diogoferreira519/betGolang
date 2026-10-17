package wager_test

import (
	"errors"
	"testing"
	"time"

	domerr "github.com/diogo/golangbet/internal/domain/errors"
	"github.com/diogo/golangbet/internal/domain/money"
	"github.com/diogo/golangbet/internal/domain/wager"
)

func TestExternalOpeningRejected(t *testing.T) {
	m, _ := money.ParseExternal("10.00", "BRL")
	_, err := wager.CreateExternal(wager.ExternalCreateParams{
		ID: "t1", Kind: wager.KindOpening, WalletID: "w", PlayerID: "p",
		ProviderID: "a", ExternalTransactionID: "x", IdempotencyKey: "k",
		PayloadHash: "h", RoundID: "r", GameID: "g", Money: m, Now: time.Now(),
	})
	if !errors.Is(err, domerr.ErrOpeningNotAllowed) {
		t.Fatalf("got %v", err)
	}
}

func TestLossRequiresZero(t *testing.T) {
	z, _ := money.ParseExternal("0.00", "BRL")
	tx, err := wager.CreateExternal(wager.ExternalCreateParams{
		ID: "t1", Kind: wager.KindLoss, WalletID: "w", PlayerID: "p",
		ProviderID: "a", ExternalTransactionID: "x", IdempotencyKey: "k",
		PayloadHash: "h", RoundID: "r", GameID: "g", Money: z, Now: time.Now(),
	})
	if err != nil || tx.Status() != wager.StatusPending {
		t.Fatal(err)
	}
}

func TestTerminalNoTransition(t *testing.T) {
	m, _ := money.ParseExternal("10.00", "BRL")
	tx, _ := wager.CreateExternal(wager.ExternalCreateParams{
		ID: "t1", Kind: wager.KindBet, WalletID: "w", PlayerID: "p",
		ProviderID: "a", ExternalTransactionID: "x", IdempotencyKey: "k",
		PayloadHash: "h", RoundID: "r", GameID: "g", Money: m, Now: time.Now(),
	})
	_ = tx.MarkProcessed(m, time.Now())
	if err := tx.MarkRejected(domerr.FailureValidation, m, time.Now()); !errors.Is(err, domerr.ErrTerminalState) {
		t.Fatalf("got %v", err)
	}
}
