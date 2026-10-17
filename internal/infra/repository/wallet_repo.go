package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/diogo/golangbet/internal/domain/money"
	"github.com/diogo/golangbet/internal/domain/wallet"
)

type WalletRepository struct {
	db DBTX
}

func NewWalletRepository(db DBTX) *WalletRepository {
	return &WalletRepository{db: db}
}

func (r *WalletRepository) WithDB(db DBTX) *WalletRepository {
	return &WalletRepository{db: db}
}

func (r *WalletRepository) Save(ctx context.Context, w *wallet.Wallet) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO wallets (id, player_id, currency, balance_cents, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`,
		string(w.ID()),
		string(w.PlayerID()),
		w.Currency(),
		w.Balance().AmountMinor(),
		w.Version(),
		w.CreatedAt().UTC(),
		w.UpdatedAt().UTC(),
	)
	if err != nil {
		return mapWalletUnique(err)
	}
	return nil
}

// Update persists wallet balance/version with optimistic locking on the prior version.
// Callers must pass expectedVersion equal to the version loaded before domain mutation.
func (r *WalletRepository) Update(ctx context.Context, w *wallet.Wallet, expectedVersion int64) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE wallets
		SET balance_cents = $1,
		    version = $2,
		    updated_at = $3
		WHERE id = $4 AND version = $5
	`,
		w.Balance().AmountMinor(),
		w.Version(),
		w.UpdatedAt().UTC(),
		string(w.ID()),
		expectedVersion,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrOptimisticLock
	}
	return nil
}

func (r *WalletRepository) GetByID(ctx context.Context, id wallet.ID) (*wallet.Wallet, error) {
	return r.scanOne(ctx, `
		SELECT id, player_id, currency, balance_cents, version, created_at, updated_at
		FROM wallets WHERE id = $1
	`, string(id))
}

func (r *WalletRepository) GetByIDForUpdate(ctx context.Context, id wallet.ID) (*wallet.Wallet, error) {
	return r.scanOne(ctx, `
		SELECT id, player_id, currency, balance_cents, version, created_at, updated_at
		FROM wallets WHERE id = $1
		FOR UPDATE
	`, string(id))
}

func (r *WalletRepository) GetByPlayerCurrency(ctx context.Context, playerID wallet.PlayerID, currency string) (*wallet.Wallet, error) {
	return r.scanOne(ctx, `
		SELECT id, player_id, currency, balance_cents, version, created_at, updated_at
		FROM wallets WHERE player_id = $1 AND currency = $2
	`, string(playerID), currency)
}

func (r *WalletRepository) scanOne(ctx context.Context, query string, args ...any) (*wallet.Wallet, error) {
	var (
		id, playerID, currency string
		balanceCents, version  int64
		createdAt, updatedAt   time.Time
	)
	err := r.db.QueryRow(ctx, query, args...).Scan(
		&id, &playerID, &currency, &balanceCents, &version, &createdAt, &updatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	bal, err := money.FromMinor(balanceCents, currency)
	if err != nil {
		return nil, fmt.Errorf("money: %w", err)
	}
	return wallet.Rehydrate(wallet.ID(id), wallet.PlayerID(playerID), bal, version, createdAt, updatedAt)
}
