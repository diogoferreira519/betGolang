package money_test

import (
	"math"
	"testing"

	"github.com/diogo/golangbet/internal/domain/money"
)

func TestParseExternalValid(t *testing.T) {
	m, err := money.ParseExternal("25.00", "brl")
	if err != nil {
		t.Fatal(err)
	}
	if m.AmountMinor() != 2500 || m.Currency() != "BRL" || m.DecimalString() != "25.00" {
		t.Fatalf("got %d %s", m.AmountMinor(), m.DecimalString())
	}
}

func TestParseExternalRejects(t *testing.T) {
	cases := []string{"", "1e2", "NaN", "Infinity", "-1.00", "1.001", "abc", "+1.00"}
	for _, c := range cases {
		if _, err := money.ParseExternal(c, "BRL"); err == nil {
			t.Fatalf("expected error for %q", c)
		}
	}
}

func TestCurrencyMismatch(t *testing.T) {
	a, _ := money.ParseExternal("1.00", "BRL")
	b, _ := money.ParseExternal("1.00", "USD")
	if _, err := a.Add(b); err == nil {
		t.Fatal("expected currency mismatch")
	}
}

func TestAddSubNegate(t *testing.T) {
	a, _ := money.ParseExternal("10.00", "BRL")
	b, _ := money.ParseExternal("3.50", "BRL")
	sum, err := a.Add(b)
	if err != nil || sum.DecimalString() != "13.50" {
		t.Fatalf("add: %v %s", err, sum.DecimalString())
	}
	diff, err := a.Sub(b)
	if err != nil || diff.DecimalString() != "6.50" {
		t.Fatalf("sub: %v %s", err, diff.DecimalString())
	}
	neg, err := b.Negate()
	if err != nil || neg.DecimalString() != "-3.50" {
		t.Fatalf("neg: %v %s", err, neg.DecimalString())
	}
}

func TestOverflow(t *testing.T) {
	a, _ := money.FromMinor(math.MaxInt64, "BRL")
	one, _ := money.FromMinor(1, "BRL")
	if _, err := a.Add(one); err == nil {
		t.Fatal("expected overflow")
	}
	min, _ := money.FromMinor(math.MinInt64, "BRL")
	if _, err := min.Negate(); err == nil {
		t.Fatal("expected negate overflow")
	}
}

func TestZeroAndJSON(t *testing.T) {
	z, err := money.Zero("BRL")
	if err != nil || !z.IsZero() {
		t.Fatal(err)
	}
	b, err := z.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	var m money.Money
	if err := m.UnmarshalJSON(b); err != nil {
		t.Fatal(err)
	}
	if !m.Equal(z) {
		t.Fatal("json roundtrip")
	}
}
