package repository

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrNotFound                     = errors.New("not found")
	ErrOptimisticLock               = errors.New("optimistic lock conflict")
	ErrDuplicateWallet              = errors.New("duplicate wallet")
	ErrDuplicateExternalTransaction = errors.New("duplicate external transaction")
	ErrDuplicateIdempotencyKey      = errors.New("duplicate idempotency key")
	ErrDuplicateLedgerEntry         = errors.New("duplicate ledger entry")
)

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func mapWalletUnique(err error) error {
	if isUniqueViolation(err) {
		return ErrDuplicateWallet
	}
	return err
}

func mapWagerUnique(err error) error {
	if !isUniqueViolation(err) {
		return err
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.ConstraintName {
		case "wager_transactions_provider_idempotency_uidx":
			return ErrDuplicateIdempotencyKey
		case "wager_transactions_provider_external_uidx":
			return ErrDuplicateExternalTransaction
		}
	}
	return ErrDuplicateExternalTransaction
}

func mapLedgerUnique(err error) error {
	if isUniqueViolation(err) {
		return ErrDuplicateLedgerEntry
	}
	return err
}
