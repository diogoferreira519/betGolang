package hash_test

import (
	"testing"

	"github.com/diogo/golangbet/internal/platform/hash"
)

func TestPayloadHashStable(t *testing.T) {
	a := hash.BusinessPayload{
		ProviderID: "provider-a", ExternalTransactionID: "t1", PlayerID: "p", WalletID: "w",
		RoundID: "r", GameID: "g", Kind: "BET", MoneyAmount: "25.00", MoneyCurrency: "BRL",
	}
	b := a
	ha, err := hash.PayloadHash(a)
	if err != nil {
		t.Fatal(err)
	}
	hb, err := hash.PayloadHash(b)
	if err != nil || ha != hb || ha == "" {
		t.Fatalf("unstable hash %q %q %v", ha, hb, err)
	}
	b.Kind = "WIN"
	hc, _ := hash.PayloadHash(b)
	if hc == ha {
		t.Fatal("expected different hash")
	}
}
