package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/diogo/golangbet/internal/domain/ledger"
	"github.com/diogo/golangbet/internal/domain/money"
	"github.com/diogo/golangbet/internal/domain/wallet"
)

type LedgerRepository struct {
	db DBTX
}

func NewLedgerRepository(db DBTX) *LedgerRepository {
	return &LedgerRepository{db: db}
}

func (r *LedgerRepository) WithDB(db DBTX) *LedgerRepository {
	return &LedgerRepository{db: db}
}

func (r *LedgerRepository) Append(ctx context.Context, entry *ledger.Entry) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO wallet_ledger_entries (
			id, wallet_id, transaction_id, direction,
			amount_cents, currency, balance_before_cents, balance_after_cents, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
	`,
		string(entry.ID()),
		string(entry.WalletID()),
		string(entry.TransactionID()),
		string(entry.Direction()),
		entry.Money().AmountMinor(),
		entry.Money().Currency(),
		entry.BalanceBefore().AmountMinor(),
		entry.BalanceAfter().AmountMinor(),
		entry.CreatedAt().UTC(),
	)
	if err != nil {
		return mapLedgerUnique(err)
	}
	return nil
}

func (r *LedgerRepository) ListByWalletID(ctx context.Context, walletID wallet.ID) ([]*ledger.Entry, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, wallet_id, transaction_id, direction,
		       amount_cents, currency, balance_before_cents, balance_after_cents, created_at
		FROM wallet_ledger_entries
		WHERE wallet_id = $1
		ORDER BY created_at ASC, id ASC
	`, string(walletID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*ledger.Entry
	for rows.Next() {
		var (
			id, wid, txID, direction, currency string
			amount, before, after              int64
			createdAt                          time.Time
		)
		if err := rows.Scan(&id, &wid, &txID, &direction, &amount, &currency, &before, &after, &createdAt); err != nil {
			return nil, err
		}
		m, err := money.FromMinor(amount, currency)
		if err != nil {
			return nil, fmt.Errorf("money: %w", err)
		}
		beforeM, err := money.FromMinor(before, currency)
		if err != nil {
			return nil, fmt.Errorf("balance before: %w", err)
		}
		afterM, err := money.FromMinor(after, currency)
		if err != nil {
			return nil, fmt.Errorf("balance after: %w", err)
		}
		entry, err := ledger.Rehydrate(
			ledger.EntryID(id),
			wallet.ID(wid),
			ledger.TransactionID(txID),
			ledger.Direction(direction),
			m, beforeM, afterM, createdAt,
		)
		if err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
