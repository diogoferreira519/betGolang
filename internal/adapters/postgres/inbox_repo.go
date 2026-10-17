package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/diogo/golangbet/internal/app/ports"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type InboxRepo struct{}
func NewInboxRepo() *InboxRepo { return &InboxRepo{} }

func (r *InboxRepo) TryInsert(ctx context.Context, t ports.Tx, consumerName, messageID, payloadHash string, now time.Time) (bool, error) {
	tx, err := AsTx(t); if err != nil { return false, err }
	_, err = tx.Exec(ctx, `INSERT INTO inbox_messages (consumer_name, message_id, payload_hash, received_at) VALUES ($1,$2,$3,$4)`, consumerName, messageID, payloadHash, now)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" { return false, nil }
	return err == nil, err
}

func (r *InboxRepo) Complete(ctx context.Context, t ports.Tx, consumerName, messageID string, now time.Time) error {
	tx, err := AsTx(t); if err != nil { return err }
	_, err = tx.Exec(ctx, `UPDATE inbox_messages SET completed_at=$3 WHERE consumer_name=$1 AND message_id=$2`, consumerName, messageID, now)
	return err
}

func (r *InboxRepo) Get(ctx context.Context, t ports.Tx, consumerName, messageID string) (string, bool, error) {
	tx, err := AsTx(t); if err != nil { return "", false, err }
	var hash string; var completedAt *time.Time
	err = tx.QueryRow(ctx, `SELECT payload_hash, completed_at FROM inbox_messages WHERE consumer_name=$1 AND message_id=$2`, consumerName, messageID).Scan(&hash, &completedAt)
	if errors.Is(err, pgx.ErrNoRows) { return "", false, nil }
	if err != nil { return "", false, err }
	return hash, completedAt != nil, nil
}
