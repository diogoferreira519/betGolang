package money

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	domerr "github.com/diogo/golangbet/internal/domain/errors"
)

const Scale = 2

// Money is an immutable monetary value in minor units (cents for scale 2).
type Money struct {
	amountMinor int64
	currency    string
}

func Zero(currency string) (Money, error) {
	c, err := normalizeCurrency(currency)
	if err != nil {
		return Money{}, err
	}
	return Money{amountMinor: 0, currency: c}, nil
}

func FromMinor(amountMinor int64, currency string) (Money, error) {
	c, err := normalizeCurrency(currency)
	if err != nil {
		return Money{}, err
	}
	return Money{amountMinor: amountMinor, currency: c}, nil
}

// ParseExternal parses an external decimal amount. Rejects empty, NaN,
// Infinity, scientific notation, scale > 2, and negatives.
func ParseExternal(amount, currency string) (Money, error) {
	c, err := normalizeCurrency(currency)
	if err != nil {
		return Money{}, err
	}
	minor, err := parseDecimalStrict(amount, true)
	if err != nil {
		return Money{}, err
	}
	return Money{amountMinor: minor, currency: c}, nil
}

func ParseInternal(amount, currency string) (Money, error) {
	c, err := normalizeCurrency(currency)
	if err != nil {
		return Money{}, err
	}
	minor, err := parseDecimalStrict(amount, false)
	if err != nil {
		return Money{}, err
	}
	return Money{amountMinor: minor, currency: c}, nil
}

func (m Money) AmountMinor() int64 { return m.amountMinor }
func (m Money) Currency() string   { return m.currency }
func (m Money) IsZero() bool       { return m.amountMinor == 0 }
func (m Money) IsNegative() bool   { return m.amountMinor < 0 }
func (m Money) IsPositive() bool   { return m.amountMinor > 0 }

func (m Money) DecimalString() string {
	neg := m.amountMinor < 0
	v := m.amountMinor
	if neg {
		v = -v
	}
	s := fmt.Sprintf("%d.%02d", v/100, v%100)
	if neg {
		return "-" + s
	}
	return s
}

func (m Money) Add(other Money) (Money, error) {
	if err := m.sameCurrency(other); err != nil {
		return Money{}, err
	}
	sum, ok := addInt64(m.amountMinor, other.amountMinor)
	if !ok {
		return Money{}, domerr.New(domerr.FailureInvalidAmount, domerr.ErrOverflow, "add overflow")
	}
	return Money{amountMinor: sum, currency: m.currency}, nil
}

func (m Money) Sub(other Money) (Money, error) {
	if err := m.sameCurrency(other); err != nil {
		return Money{}, err
	}
	diff, ok := subInt64(m.amountMinor, other.amountMinor)
	if !ok {
		return Money{}, domerr.New(domerr.FailureInvalidAmount, domerr.ErrOverflow, "sub overflow")
	}
	return Money{amountMinor: diff, currency: m.currency}, nil
}

func (m Money) Negate() (Money, error) {
	if m.amountMinor == math.MinInt64 {
		return Money{}, domerr.New(domerr.FailureInvalidAmount, domerr.ErrOverflow, "negate overflow")
	}
	return Money{amountMinor: -m.amountMinor, currency: m.currency}, nil
}

func (m Money) Cmp(other Money) (int, error) {
	if err := m.sameCurrency(other); err != nil {
		return 0, err
	}
	switch {
	case m.amountMinor < other.amountMinor:
		return -1, nil
	case m.amountMinor > other.amountMinor:
		return 1, nil
	default:
		return 0, nil
	}
}

func (m Money) Equal(other Money) bool {
	return m.currency == other.currency && m.amountMinor == other.amountMinor
}

func (m Money) sameCurrency(other Money) error {
	if m.currency == "" || other.currency == "" {
		return domerr.New(domerr.FailureInvalidAmount, domerr.ErrInvalidMoney, "uninitialized money")
	}
	if m.currency != other.currency {
		return domerr.New(domerr.FailureCurrencyMismatch, domerr.ErrCurrencyMismatch, "currency mismatch: %s vs %s", m.currency, other.currency)
	}
	return nil
}

type dto struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

func (m Money) MarshalJSON() ([]byte, error) {
	if m.currency == "" {
		return nil, domerr.New(domerr.FailureInvalidAmount, domerr.ErrInvalidMoney, "uninitialized money")
	}
	return json.Marshal(dto{Amount: m.DecimalString(), Currency: m.currency})
}

func (m *Money) UnmarshalJSON(b []byte) error {
	var d dto
	if err := json.Unmarshal(b, &d); err != nil {
		return err
	}
	parsed, err := ParseExternal(d.Amount, d.Currency)
	if err != nil {
		return err
	}
	*m = parsed
	return nil
}

func normalizeCurrency(currency string) (string, error) {
	c := strings.ToUpper(strings.TrimSpace(currency))
	if len(c) != 3 {
		return "", domerr.New(domerr.FailureInvalidAmount, domerr.ErrInvalidMoney, "invalid currency code")
	}
	for _, r := range c {
		if r < 'A' || r > 'Z' {
			return "", domerr.New(domerr.FailureInvalidAmount, domerr.ErrInvalidMoney, "invalid currency code")
		}
	}
	return c, nil
}

func parseDecimalStrict(amount string, rejectNegative bool) (int64, error) {
	s := strings.TrimSpace(amount)
	if s == "" {
		return 0, domerr.New(domerr.FailureInvalidAmount, domerr.ErrInvalidMoney, "empty amount")
	}
	lower := strings.ToLower(s)
	if strings.Contains(lower, "nan") || strings.Contains(lower, "inf") || strings.ContainsAny(s, "eE") {
		return 0, domerr.New(domerr.FailureInvalidAmount, domerr.ErrInvalidMoney, "invalid amount format")
	}
	neg := false
	if s[0] == '-' {
		neg = true
		s = s[1:]
		if rejectNegative {
			return 0, domerr.New(domerr.FailureInvalidAmount, domerr.ErrInvalidMoney, "negative amount not allowed")
		}
	}
	if s == "" || s[0] == '+' {
		return 0, domerr.New(domerr.FailureInvalidAmount, domerr.ErrInvalidMoney, "invalid amount format")
	}
	parts := strings.Split(s, ".")
	if len(parts) > 2 {
		return 0, domerr.New(domerr.FailureInvalidAmount, domerr.ErrInvalidMoney, "invalid amount format")
	}
	whole := parts[0]
	frac := ""
	if len(parts) == 2 {
		frac = parts[1]
		if len(frac) > Scale {
			return 0, domerr.New(domerr.FailureInvalidAmount, domerr.ErrInvalidMoney, "scale exceeds %d", Scale)
		}
	}
	if whole == "" {
		return 0, domerr.New(domerr.FailureInvalidAmount, domerr.ErrInvalidMoney, "invalid amount format")
	}
	for _, r := range whole + frac {
		if r < '0' || r > '9' {
			return 0, domerr.New(domerr.FailureInvalidAmount, domerr.ErrInvalidMoney, "invalid amount format")
		}
	}
	for len(frac) < Scale {
		frac += "0"
	}
	wholeVal, ok := parseUint64Digits(whole)
	if !ok {
		return 0, domerr.New(domerr.FailureInvalidAmount, domerr.ErrOverflow, "amount overflow")
	}
	fracVal, ok := parseUint64Digits(frac)
	if !ok {
		return 0, domerr.New(domerr.FailureInvalidAmount, domerr.ErrOverflow, "amount overflow")
	}
	if wholeVal > uint64(math.MaxInt64)/100 {
		return 0, domerr.New(domerr.FailureInvalidAmount, domerr.ErrOverflow, "amount overflow")
	}
	minorU := wholeVal*100 + fracVal
	if minorU > uint64(math.MaxInt64) {
		return 0, domerr.New(domerr.FailureInvalidAmount, domerr.ErrOverflow, "amount overflow")
	}
	minor := int64(minorU)
	if neg {
		minor = -minor
	}
	return minor, nil
}

func parseUint64Digits(s string) (uint64, bool) {
	var v uint64
	for _, r := range s {
		d := uint64(r - '0')
		if v > (math.MaxUint64-d)/10 {
			return 0, false
		}
		v = v*10 + d
	}
	return v, true
}

func addInt64(a, b int64) (int64, bool) {
	if b > 0 && a > math.MaxInt64-b {
		return 0, false
	}
	if b < 0 && a < math.MinInt64-b {
		return 0, false
	}
	return a + b, true
}

func subInt64(a, b int64) (int64, bool) {
	if b > 0 && a < math.MinInt64+b {
		return 0, false
	}
	if b < 0 && a > math.MaxInt64+b {
		return 0, false
	}
	return a - b, true
}
