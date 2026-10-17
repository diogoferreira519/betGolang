package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	domerr "github.com/diogo/golangbet/internal/domain/errors"
	"github.com/diogo/golangbet/internal/domain/money"
	"github.com/diogo/golangbet/internal/domain/wager"
	"github.com/diogo/golangbet/internal/domain/wallet"
)

type WagerRepository struct {
	db DBTX
}

func NewWagerRepository(db DBTX) *WagerRepository {
	return &WagerRepository{db: db}
}

func (r *WagerRepository) WithDB(db DBTX) *WagerRepository {
	return &WagerRepository{db: db}
}

func (r *WagerRepository) Insert(ctx context.Context, tx *wager.Transaction) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO wager_transactions (
			id, origin, kind, status, wallet_id, player_id,
			provider_id, external_transaction_id, idempotency_key, payload_hash,
			round_id, game_id, amount_cents, currency,
			reference_external_id, reference_internal_id, failure_code,
			result_balance_minor, result_balance_currency, created_at, updated_at
		) VALUES (
			$1,$2,$3,$4,$5,$6,
			NULLIF($7,''), NULLIF($8,''), NULLIF($9,''), NULLIF($10,''),
			NULLIF($11,''), NULLIF($12,''), $13, $14,
			NULLIF($15,''), NULLIF($16,''), NULLIF($17,''),
			$18, NULLIF($19,''), $20, $21
		)
	`,
		string(tx.ID()),
		string(tx.Origin()),
		string(tx.Kind()),
		string(tx.Status()),
		string(tx.WalletID()),
		string(tx.PlayerID()),
		tx.ProviderID(),
		tx.ExternalTransactionID(),
		tx.IdempotencyKey(),
		tx.PayloadHash(),
		tx.RoundID(),
		tx.GameID(),
		tx.Money().AmountMinor(),
		tx.Money().Currency(),
		tx.ReferenceExternalID(),
		string(tx.ReferenceInternalID()),
		string(tx.FailureCode()),
		tx.ResultBalanceMinor(),
		tx.ResultBalanceCurrency(),
		tx.CreatedAt().UTC(),
		tx.UpdatedAt().UTC(),
	)
	if err != nil {
		return mapWagerUnique(err)
	}
	return nil
}

func (r *WagerRepository) UpdateStatus(ctx context.Context, tx *wager.Transaction) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE wager_transactions
		SET status = $1,
		    reference_internal_id = NULLIF($2,''),
		    failure_code = NULLIF($3,''),
		    result_balance_minor = $4,
		    result_balance_currency = NULLIF($5,''),
		    updated_at = $6
		WHERE id = $7
	`,
		string(tx.Status()),
		string(tx.ReferenceInternalID()),
		string(tx.FailureCode()),
		tx.ResultBalanceMinor(),
		tx.ResultBalanceCurrency(),
		tx.UpdatedAt().UTC(),
		string(tx.ID()),
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *WagerRepository) GetByID(ctx context.Context, id wager.ID) (*wager.Transaction, error) {
	return r.scanOne(ctx, `
		SELECT id, origin, kind, status, wallet_id, player_id,
		       provider_id, external_transaction_id, idempotency_key, payload_hash,
		       round_id, game_id, amount_cents, currency,
		       reference_external_id, reference_internal_id, failure_code,
		       result_balance_minor, result_balance_currency, created_at, updated_at
		FROM wager_transactions WHERE id = $1
	`, string(id))
}

func (r *WagerRepository) GetByProviderExternalID(ctx context.Context, providerID, externalTransactionID string) (*wager.Transaction, error) {
	return r.scanOne(ctx, `
		SELECT id, origin, kind, status, wallet_id, player_id,
		       provider_id, external_transaction_id, idempotency_key, payload_hash,
		       round_id, game_id, amount_cents, currency,
		       reference_external_id, reference_internal_id, failure_code,
		       result_balance_minor, result_balance_currency, created_at, updated_at
		FROM wager_transactions
		WHERE provider_id = $1 AND external_transaction_id = $2
	`, providerID, externalTransactionID)
}

func (r *WagerRepository) scanOne(ctx context.Context, query string, args ...any) (*wager.Transaction, error) {
	var (
		id, origin, kind, status, walletID, playerID string
		providerID, externalID, idemKey, payloadHash sql.NullString
		roundID, gameID                              sql.NullString
		amountCents                                  int64
		currency                                     string
		refExternal, refInternal, failureCode        sql.NullString
		resultMinor                                  sql.NullInt64
		resultCurrency                               sql.NullString
		createdAt, updatedAt                         time.Time
	)
	err := r.db.QueryRow(ctx, query, args...).Scan(
		&id, &origin, &kind, &status, &walletID, &playerID,
		&providerID, &externalID, &idemKey, &payloadHash,
		&roundID, &gameID, &amountCents, &currency,
		&refExternal, &refInternal, &failureCode,
		&resultMinor, &resultCurrency, &createdAt, &updatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	m, err := money.FromMinor(amountCents, currency)
	if err != nil {
		return nil, fmt.Errorf("money: %w", err)
	}
	var resultPtr *int64
	if resultMinor.Valid {
		v := resultMinor.Int64
		resultPtr = &v
	}
	return wager.Rehydrate(wager.RehydrateParams{
		ID:                    wager.ID(id),
		Origin:                wager.Origin(origin),
		Kind:                  wager.Kind(kind),
		Status:                wager.Status(status),
		WalletID:              wallet.ID(walletID),
		PlayerID:              wallet.PlayerID(playerID),
		ProviderID:            nullStr(providerID),
		ExternalTransactionID: nullStr(externalID),
		IdempotencyKey:        nullStr(idemKey),
		PayloadHash:           nullStr(payloadHash),
		RoundID:               nullStr(roundID),
		GameID:                nullStr(gameID),
		ReferenceExternalID:   nullStr(refExternal),
		ReferenceInternalID:   wager.ID(nullStr(refInternal)),
		FailureCode:           domerr.FailureCode(nullStr(failureCode)),
		Money:                 m,
		ResultBalanceMinor:    resultPtr,
		ResultBalanceCurrency: nullStr(resultCurrency),
		CreatedAt:             createdAt,
		UpdatedAt:             updatedAt,
	})
}

func nullStr(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}
