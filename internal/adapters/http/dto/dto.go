package dto

import "github.com/diogo/golangbet/internal/domain/money"

type Money struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

func FromMoney(m money.Money) Money {
	return Money{Amount: m.DecimalString(), Currency: m.Currency()}
}
func (m Money) ToMoney() (money.Money, error) {
	return money.ParseExternal(m.Amount, m.Currency)
}

type OpenWalletRequest struct {
	PlayerID       string `json:"playerId"`
	InitialBalance Money  `json:"initialBalance"`
}
type WalletResponse struct {
	ID       string `json:"id"`
	PlayerID string `json:"playerId"`
	Balance  Money  `json:"balance"`
	Version  int64  `json:"version"`
}
type SubmitWagerRequest struct {
	ProviderID                   string `json:"providerId"`
	ExternalTransactionID        string `json:"externalTransactionId"`
	PlayerID                     string `json:"playerId"`
	WalletID                     string `json:"walletId"`
	RoundID                      string `json:"roundId"`
	GameID                       string `json:"gameId"`
	Kind                         string `json:"kind"`
	Money                        Money  `json:"money"`
	ReferenceExternalTransaction string `json:"referenceExternalTransactionId,omitempty"`
}
type SubmitWagerResponse struct {
	TransactionID    string `json:"transactionId"`
	Status           string `json:"status"`
	Balance          Money  `json:"balance"`
	IdempotentReplay bool   `json:"idempotentReplay"`
	FailureCode      string `json:"failureCode,omitempty"`
}
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
	Code    string `json:"failureCode,omitempty"`
}
type ReconciliationResponse struct {
	WalletID          string `json:"walletId"`
	StoredBalance     Money  `json:"storedBalance"`
	CalculatedBalance Money  `json:"calculatedBalance"`
	Difference        Money  `json:"difference"`
	Consistent        bool   `json:"consistent"`
	CheckedEntries    int    `json:"checkedEntries"`
}
type LedgerEntryResponse struct {
	ID            string `json:"id"`
	TransactionID string `json:"transactionId"`
	Direction     string `json:"direction"`
	Money         Money  `json:"money"`
	BalanceBefore Money  `json:"balanceBefore"`
	BalanceAfter  Money  `json:"balanceAfter"`
	CreatedAt     string `json:"createdAt"`
}
type LedgerPageResponse struct {
	Items      []LedgerEntryResponse `json:"items"`
	NextCursor string                `json:"nextCursor,omitempty"`
}
type TransactionResponse struct {
	TransactionID         string `json:"transactionId"`
	Status                string `json:"status"`
	Kind                  string `json:"kind"`
	ProviderID            string `json:"providerId,omitempty"`
	ExternalTransactionID string `json:"externalTransactionId,omitempty"`
	WalletID              string `json:"walletId"`
	PlayerID              string `json:"playerId"`
	Money                 Money  `json:"money"`
	FailureCode           string `json:"failureCode,omitempty"`
	Balance               *Money `json:"balance,omitempty"`
	IdempotencyKey        string `json:"idempotencyKey,omitempty"`
}
