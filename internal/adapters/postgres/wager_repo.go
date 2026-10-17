package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/diogo/golangbet/internal/app/ports"
	domerr "github.com/diogo/golangbet/internal/domain/errors"
	"github.com/diogo/golangbet/internal/domain/money"
	"github.com/diogo/golangbet/internal/domain/wager"
	"github.com/diogo/golangbet/internal/domain/wallet"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type WagerRepo struct{ pool *pgxpool.Pool }

func NewWagerRepo(pool *Pool) *WagerRepo { return &WagerRepo{pool: pool.Pool} }

func (r *WagerRepo) Insert(ctx context.Context, t ports.Tx, txw *wager.Transaction) error {
	tx, err := AsTx(t)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO wager_transactions (
			id, origin, kind, status, wallet_id, player_id, provider_id, external_transaction_id,
			idempotency_key, payload_hash, round_id, game_id, money_minor, money_currency,
			reference_external_id, reference_internal_id, failure_code, result_balance_minor,
			result_balance_currency, pending_attempts, next_attempt_at, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,0,NULL,$20,$21)`,
		string(txw.ID()), string(txw.Origin()), string(txw.Kind()), string(txw.Status()),
		string(txw.WalletID()), string(txw.PlayerID()), nullStr(txw.ProviderID()), nullStr(txw.ExternalTransactionID()),
		nullStr(txw.IdempotencyKey()), nullStr(txw.PayloadHash()), nullStr(txw.RoundID()), nullStr(txw.GameID()),
		txw.Money().AmountMinor(), txw.Money().Currency(), nullStr(txw.ReferenceExternalID()),
		nullUUID(string(txw.ReferenceInternalID())), nullStr(string(txw.FailureCode())),
		txw.ResultBalanceMinor(), nullStr(txw.ResultBalanceCurrency()), txw.CreatedAt(), txw.UpdatedAt(),
	)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		if pgErr.ConstraintName == "uq_wager_idempotency_key" {
			return domerr.New(domerr.FailureValidation, domerr.ErrIdempotencyConflict, "idempotency conflict")
		}
		return domerr.New(domerr.FailureValidation, domerr.ErrExternalIDConflict, "external id conflict")
	}
	return err
}

func (r *WagerRepo) Update(ctx context.Context, t ports.Tx, txw *wager.Transaction) error {
	tx, err := AsTx(t)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		UPDATE wager_transactions SET status=$2, reference_internal_id=$3, failure_code=$4,
			result_balance_minor=$5, result_balance_currency=$6, updated_at=$7 WHERE id=$1`,
		string(txw.ID()), string(txw.Status()), nullUUID(string(txw.ReferenceInternalID())),
		nullStr(string(txw.FailureCode())), txw.ResultBalanceMinor(), nullStr(txw.ResultBalanceCurrency()), txw.UpdatedAt(),
	)
	return err
}

func (r *WagerRepo) UpdatePendingMeta(ctx context.Context, t ports.Tx, id wager.ID, attempts int, next *time.Time, updatedAt time.Time) error {
	tx, err := AsTx(t)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE wager_transactions SET pending_attempts=$2, next_attempt_at=$3, updated_at=$4 WHERE id=$1`,
		string(id), attempts, next, updatedAt)
	return err
}

const wagerSelect = `SELECT id, origin, kind, status, wallet_id, player_id, provider_id, external_transaction_id,
       idempotency_key, payload_hash, round_id, game_id, money_minor, money_currency,
       reference_external_id, reference_internal_id, failure_code, result_balance_minor,
       result_balance_currency, created_at, updated_at FROM wager_transactions`

func (r *WagerRepo) GetByID(ctx context.Context, id wager.ID) (*wager.Transaction, error) {
	return scanWager(r.pool.QueryRow(ctx, wagerSelect+" WHERE id=$1", string(id)))
}

func (r *WagerRepo) GetByIDTx(ctx context.Context, t ports.Tx, id wager.ID) (*wager.Transaction, error) {
	tx, err := AsTx(t)
	if err != nil {
		return nil, err
	}
	return scanWager(tx.QueryRow(ctx, wagerSelect+" WHERE id=$1", string(id)))
}

func (r *WagerRepo) GetByIdempotencyKey(ctx context.Context, t ports.Tx, key string) (*wager.Transaction, error) {
	tx, err := AsTx(t)
	if err != nil {
		return nil, err
	}
	return scanWager(tx.QueryRow(ctx, wagerSelect+" WHERE idempotency_key=$1", key))
}

func (r *WagerRepo) GetByProviderExternal(ctx context.Context, t ports.Tx, providerID, externalID string) (*wager.Transaction, error) {
	tx, err := AsTx(t)
	if err != nil {
		return nil, err
	}
	return scanWager(tx.QueryRow(ctx, wagerSelect+" WHERE provider_id=$1 AND external_transaction_id=$2", providerID, externalID))
}

func (r *WagerRepo) ListPendingReferences(ctx context.Context, limit int, now time.Time) ([]*wager.Transaction, error) {
	rows, err := r.pool.Query(ctx, wagerSelect+`
		WHERE status='PENDING_REFERENCE' AND (next_attempt_at IS NULL OR next_attempt_at <= $1)
		ORDER BY next_attempt_at NULLS FIRST LIMIT $2`, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*wager.Transaction
	for rows.Next() {
		txw, err := scanWager(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, txw)
	}
	return out, rows.Err()
}

func (r *WagerRepo) CountSuccessfulBetRestorations(ctx context.Context, t ports.Tx, betID wager.ID) (int, error) {
	tx, err := AsTx(t)
	if err != nil {
		return 0, err
	}
	var n int
	err = tx.QueryRow(ctx, `
		SELECT COUNT(*) FROM wager_transactions
		WHERE reference_internal_id=$1 AND status='PROCESSED'
		  AND (kind='REFUND' OR kind='ROLLBACK')`, string(betID)).Scan(&n)
	return n, err
}

func (r *WagerRepo) PendingAttempts(ctx context.Context, id wager.ID) (int, time.Time, error) {
	var attempts int
	var createdAt time.Time
	err := r.pool.QueryRow(ctx, `SELECT pending_attempts, created_at FROM wager_transactions WHERE id=$1`, string(id)).
		Scan(&attempts, &createdAt)
	return attempts, createdAt, err
}

func scanWager(row scannable) (*wager.Transaction, error) {
	var (
		id, origin, kind, status, walletID, playerID string
		providerID, extID, idemKey, hash, roundID, gameID *string
		moneyMinor                                        int64
		moneyCurrency                                     string
		refExt, refInt, failure                           *string
		resultMinor                                       *int64
		resultCurrency                                    *string
		createdAt, updatedAt                              time.Time
	)
	if err := row.Scan(
		&id, &origin, &kind, &status, &walletID, &playerID, &providerID, &extID,
		&idemKey, &hash, &roundID, &gameID, &moneyMinor, &moneyCurrency,
		&refExt, &refInt, &failure, &resultMinor, &resultCurrency, &createdAt, &updatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domerr.New(domerr.FailureValidation, domerr.ErrNotFound, "transaction not found")
		}
		return nil, err
	}
	m, err := money.FromMinor(moneyMinor, moneyCurrency)
	if err != nil {
		return nil, err
	}
	var fc domerr.FailureCode
	if failure != nil {
		fc = domerr.FailureCode(*failure)
	}
	return wager.Rehydrate(wager.RehydrateParams{
		ID: wager.ID(id), Origin: wager.Origin(origin), Kind: wager.Kind(kind), Status: wager.Status(status),
		WalletID: wallet.ID(walletID), PlayerID: wallet.PlayerID(playerID),
		ProviderID: deref(providerID), ExternalTransactionID: deref(extID),
		IdempotencyKey: deref(idemKey), PayloadHash: deref(hash),
		RoundID: deref(roundID), GameID: deref(gameID), Money: m,
		ReferenceExternalID: deref(refExt), ReferenceInternalID: wager.ID(deref(refInt)),
		FailureCode: fc, ResultBalanceMinor: resultMinor, ResultBalanceCurrency: deref(resultCurrency),
		CreatedAt: createdAt, UpdatedAt: updatedAt,
	})
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullUUID(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
