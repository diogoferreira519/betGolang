package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/diogo/golangbet/internal/app/ports"
	domerr "github.com/diogo/golangbet/internal/domain/errors"
	"github.com/diogo/golangbet/internal/domain/money"
	"github.com/diogo/golangbet/internal/domain/wallet"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type WalletRepo struct{ pool *pgxpool.Pool }

func NewWalletRepo(pool *Pool) *WalletRepo { return &WalletRepo{pool: pool.Pool} }

func (r *WalletRepo) Insert(ctx context.Context, t ports.Tx, w *wallet.Wallet) error {
	tx, err := AsTx(t); if err != nil { return err }
	_, err = tx.Exec(ctx, `INSERT INTO wallets (id, player_id, currency, balance_minor, version, created_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		string(w.ID()), string(w.PlayerID()), w.Currency(), w.Balance().AmountMinor(), w.Version(), w.CreatedAt(), w.UpdatedAt())
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return domerr.New(domerr.FailureValidation, domerr.ErrWalletConflict, "wallet already exists")
	}
	return err
}

type scannable interface{ Scan(dest ...any) error }

func scanWallet(row scannable) (*wallet.Wallet, error) {
	var id, playerID, currency string
	var balanceMinor, version int64
	var createdAt, updatedAt time.Time
	if err := row.Scan(&id, &playerID, &currency, &balanceMinor, &version, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domerr.New(domerr.FailureWalletNotFound, domerr.ErrWalletNotFound, "wallet not found")
		}
		return nil, err
	}
	bal, err := money.FromMinor(balanceMinor, currency); if err != nil { return nil, err }
	w, err := wallet.Rehydrate(wallet.ID(id), wallet.PlayerID(playerID), bal, version, createdAt, updatedAt)
	if err != nil { return nil, fmt.Errorf("rehydrate wallet: %w", err) }
	return w, nil
}

func (r *WalletRepo) GetForUpdate(ctx context.Context, t ports.Tx, id wallet.ID) (*wallet.Wallet, error) {
	tx, err := AsTx(t); if err != nil { return nil, err }
	return scanWallet(tx.QueryRow(ctx, `SELECT id, player_id, currency, balance_minor, version, created_at, updated_at FROM wallets WHERE id=$1 FOR UPDATE`, string(id)))
}

func (r *WalletRepo) Get(ctx context.Context, id wallet.ID) (*wallet.Wallet, error) {
	return scanWallet(r.pool.QueryRow(ctx, `SELECT id, player_id, currency, balance_minor, version, created_at, updated_at FROM wallets WHERE id=$1`, string(id)))
}

func (r *WalletRepo) Update(ctx context.Context, t ports.Tx, w *wallet.Wallet, expectedVersion int64) error {
	tx, err := AsTx(t); if err != nil { return err }
	tag, err := tx.Exec(ctx, `UPDATE wallets SET balance_minor=$1, version=$2, updated_at=$3 WHERE id=$4 AND version=$5`,
		w.Balance().AmountMinor(), w.Version(), w.UpdatedAt(), string(w.ID()), expectedVersion)
	if err != nil { return err }
	if tag.RowsAffected() == 0 {
		return domerr.New(domerr.FailureValidation, domerr.ErrConcurrencyConflict, "wallet version conflict")
	}
	return nil
}
