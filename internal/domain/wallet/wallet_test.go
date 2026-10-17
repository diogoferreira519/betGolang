package wallet_test

import (
	"errors"
	"testing"
	"time"

	domerr "github.com/diogo/golangbet/internal/domain/errors"
	"github.com/diogo/golangbet/internal/domain/money"
	"github.com/diogo/golangbet/internal/domain/wallet"
)

func TestDebitCredit(t *testing.T) {
	bal, _ := money.ParseExternal("100.00", "BRL")
	w, err := wallet.Create(wallet.CreateParams{ID: "w1", PlayerID: "p1", InitialBalance: bal, Now: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	amt, _ := money.ParseExternal("40.00", "BRL")
	ch, err := w.Debit(amt, time.Now())
	if err != nil || ch.After.DecimalString() != "60.00" || w.Version() != 2 {
		t.Fatalf("debit %v %s v=%d", err, ch.After.DecimalString(), w.Version())
	}
}

func TestInsufficientFunds(t *testing.T) {
	bal, _ := money.ParseExternal("10.00", "BRL")
	w, _ := wallet.Create(wallet.CreateParams{ID: "w1", PlayerID: "p1", InitialBalance: bal, Now: time.Now()})
	amt, _ := money.ParseExternal("20.00", "BRL")
	_, err := w.Debit(amt, time.Now())
	if !errors.Is(err, domerr.ErrInsufficientFunds) {
		t.Fatalf("got %v", err)
	}
}
