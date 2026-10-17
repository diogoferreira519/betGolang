package postgres

import (
	"context"
	"time"

	"github.com/diogo/golangbet/internal/app/ports"
	"github.com/diogo/golangbet/internal/domain/ledger"
	"github.com/diogo/golangbet/internal/domain/money"
	"github.com/diogo/golangbet/internal/domain/wallet"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type LedgerRepo struct{ pool *pgxpool.Pool }

func NewLedgerRepo(pool *Pool) *LedgerRepo { return &LedgerRepo{pool: pool.Pool} }

func (r *LedgerRepo) Insert(ctx context.Context, t ports.Tx, e *ledger.Entry) error {
	tx, err := AsTx(t)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO wallet_ledger_entries (id, wallet_id, transaction_id, direction, money_minor, money_currency, balance_before, balance_after, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		string(e.ID()), string(e.WalletID()), string(e.TransactionID()), string(e.Direction()),
		e.Money().AmountMinor(), e.Money().Currency(), e.BalanceBefore().AmountMinor(), e.BalanceAfter().AmountMinor(), e.CreatedAt())
	return err
}

func (r *LedgerRepo) ListByWallet(ctx context.Context, walletID wallet.ID, cursorCreatedAt *time.Time, cursorID *string, limit int) ([]*ledger.Entry, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	var rows pgx.Rows
	var err error
	if cursorCreatedAt != nil && cursorID != nil {
		rows, err = r.pool.Query(ctx, `SELECT id, wallet_id, transaction_id, direction, money_minor, money_currency, balance_before, balance_after, created_at
			FROM wallet_ledger_entries WHERE wallet_id=$1 AND (created_at, id) > ($2::timestamptz, $3::uuid)
			ORDER BY created_at ASC, id ASC LIMIT $4`, string(walletID), *cursorCreatedAt, *cursorID, limit)
	} else {
		rows, err = r.pool.Query(ctx, `SELECT id, wallet_id, transaction_id, direction, money_minor, money_currency, balance_before, balance_after, created_at
			FROM wallet_ledger_entries WHERE wallet_id=$1 ORDER BY created_at ASC, id ASC LIMIT $2`, string(walletID), limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*ledger.Entry
	for rows.Next() {
		e, err := scanLedger(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *LedgerRepo) SumSigned(ctx context.Context, walletID wallet.ID) (int64, int, error) {
	var sum int64
	var n int
	err := r.pool.QueryRow(ctx, `SELECT COALESCE(SUM(CASE WHEN direction='CREDIT' THEN money_minor ELSE -money_minor END),0), COUNT(*)
		FROM wallet_ledger_entries WHERE wallet_id=$1`, string(walletID)).Scan(&sum, &n)
	return sum, n, err
}

func scanLedger(row scannable) (*ledger.Entry, error) {
	var id, walletID, txID, dir, currency string
	var moneyMinor, before, after int64
	var createdAt time.Time
	if err := row.Scan(&id, &walletID, &txID, &dir, &moneyMinor, &currency, &before, &after, &createdAt); err != nil {
		return nil, err
	}
	m, err := money.FromMinor(moneyMinor, currency)
	if err != nil {
		return nil, err
	}
	bBefore, err := money.FromMinor(before, currency)
	if err != nil {
		return nil, err
	}
	bAfter, err := money.FromMinor(after, currency)
	if err != nil {
		return nil, err
	}
	return ledger.Rehydrate(ledger.EntryID(id), wallet.ID(walletID), ledger.TransactionID(txID), ledger.Direction(dir), m, bBefore, bAfter, createdAt)
}
