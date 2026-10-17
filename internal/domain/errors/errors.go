package errors

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidMoney              = errors.New("invalid money")
	ErrCurrencyMismatch          = errors.New("currency mismatch")
	ErrOverflow                  = errors.New("numeric overflow")
	ErrInsufficientFunds         = errors.New("insufficient funds")
	ErrInsufficientFundsReversal = errors.New("insufficient funds for reversal")
	ErrInvalidWallet             = errors.New("invalid wallet")
	ErrWalletConflict            = errors.New("wallet already exists")
	ErrWalletNotFound            = errors.New("wallet not found")
	ErrInvalidTransition         = errors.New("invalid state transition")
	ErrTerminalState             = errors.New("transaction is terminal")
	ErrIdempotencyConflict       = errors.New("idempotency key conflict")
	ErrExternalIDConflict        = errors.New("external transaction id conflict")
	ErrReferenceNotFound         = errors.New("reference not found")
	ErrReferenceNotProcessed     = errors.New("reference not processed")
	ErrReferenceRequired         = errors.New("reference required")
	ErrDuplicateReversal         = errors.New("duplicate reversal")
	ErrBetAlreadyRestored        = errors.New("bet debit already restored")
	ErrInvalidKind               = errors.New("invalid transaction kind")
	ErrOpeningNotAllowed         = errors.New("OPENING not allowed on external channels")
	ErrUnauthorized              = errors.New("unauthorized")
	ErrForbidden                 = errors.New("forbidden")
	ErrNotFound                  = errors.New("not found")
	ErrValidation                = errors.New("validation error")
	ErrConcurrencyConflict       = errors.New("concurrency conflict")
	ErrTransient                 = errors.New("transient error")
)

type FailureCode string

const (
	FailureInsufficientFunds         FailureCode = "INSUFFICIENT_FUNDS"
	FailureInsufficientFundsReversal FailureCode = "INSUFFICIENT_FUNDS_REVERSAL"
	FailureReferenceNotFound         FailureCode = "REFERENCE_NOT_FOUND"
	FailureReferenceNotProcessed     FailureCode = "REFERENCE_NOT_PROCESSED"
	FailureReferenceRequired         FailureCode = "REFERENCE_REQUIRED"
	FailureDuplicateReversal         FailureCode = "DUPLICATE_REVERSAL"
	FailureBetAlreadyRestored        FailureCode = "BET_ALREADY_RESTORED"
	FailureCurrencyMismatch          FailureCode = "CURRENCY_MISMATCH"
	FailureInvalidAmount             FailureCode = "INVALID_AMOUNT"
	FailureInvalidKind               FailureCode = "INVALID_KIND"
	FailureWalletNotFound            FailureCode = "WALLET_NOT_FOUND"
	FailurePlayerMismatch            FailureCode = "PLAYER_MISMATCH"
	FailureRoundMismatch             FailureCode = "ROUND_MISMATCH"
	FailureProviderMismatch          FailureCode = "PROVIDER_MISMATCH"
	FailureValidation                FailureCode = "VALIDATION_ERROR"
	FailureOpeningNotAllowed         FailureCode = "OPENING_NOT_ALLOWED"
	FailureOverflow                  FailureCode = "OVERFLOW"
	FailureInternal                  FailureCode = "INTERNAL_ERROR"
)

type DomainError struct {
	Code    FailureCode
	Msg     string
	Wrapped error
}

func (e *DomainError) Error() string {
	if e.Msg != "" {
		return e.Msg
	}
	if e.Wrapped != nil {
		return e.Wrapped.Error()
	}
	return string(e.Code)
}

func (e *DomainError) Unwrap() error { return e.Wrapped }

func New(code FailureCode, wrapped error, msg string, args ...any) *DomainError {
	if len(args) > 0 {
		msg = fmt.Sprintf(msg, args...)
	}
	return &DomainError{Code: code, Msg: msg, Wrapped: wrapped}
}

func AsDomain(err error) (*DomainError, bool) {
	var d *DomainError
	if errors.As(err, &d) {
		return d, true
	}
	return nil, false
}
