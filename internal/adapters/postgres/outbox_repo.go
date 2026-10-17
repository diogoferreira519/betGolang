package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/diogo/golangbet/internal/app/ports"
	"github.com/diogo/golangbet/internal/domain/events"
)

type OutboxRepo struct{}
func NewOutboxRepo() *OutboxRepo { return &OutboxRepo{} }

func (r *OutboxRepo) Insert(ctx context.Context, t ports.Tx, env events.Envelope) error {
	tx, err := AsTx(t); if err != nil { return err }
	payload, err := json.Marshal(env); if err != nil { return err }
	_, err = tx.Exec(ctx, `INSERT INTO outbox_events (event_id, aggregate_id, event_type, payload, correlation_id, occurred_at, created_at) VALUES ($1,$2,$3,$4,$5,$6,$6)`,
		env.EventID, env.AggregateID, env.EventType, payload, env.CorrelationID, env.OccurredAt)
	return err
}

func (r *OutboxRepo) ClaimBatch(ctx context.Context, t ports.Tx, owner string, limit int, lease time.Duration, now time.Time) ([]events.Envelope, error) {
	tx, err := AsTx(t); if err != nil { return nil, err }
	until := now.Add(lease)
	rows, err := tx.Query(ctx, `
		WITH cte AS (
			SELECT event_id FROM outbox_events
			WHERE published_at IS NULL AND next_attempt_at <= $1 AND (lease_until IS NULL OR lease_until < $1)
			ORDER BY next_attempt_at FOR UPDATE SKIP LOCKED LIMIT $2
		)
		UPDATE outbox_events o SET lease_owner=$3, lease_until=$4, attempts=attempts+1
		FROM cte WHERE o.event_id=cte.event_id RETURNING o.payload`, now, limit, owner, until)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []events.Envelope
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil { return nil, err }
		var env events.Envelope
		if err := json.Unmarshal(raw, &env); err != nil { return nil, err }
		out = append(out, env)
	}
	return out, rows.Err()
}

func (r *OutboxRepo) MarkPublished(ctx context.Context, t ports.Tx, eventID string, now time.Time) error {
	tx, err := AsTx(t); if err != nil { return err }
	_, err = tx.Exec(ctx, `UPDATE outbox_events SET published_at=$2, lease_owner=NULL, lease_until=NULL WHERE event_id=$1`, eventID, now)
	return err
}

func (r *OutboxRepo) MarkRetry(ctx context.Context, t ports.Tx, eventID string, attempts int, next time.Time) error {
	tx, err := AsTx(t); if err != nil { return err }
	_, err = tx.Exec(ctx, `UPDATE outbox_events SET next_attempt_at=$2, lease_owner=NULL, lease_until=NULL WHERE event_id=$1`, eventID, next)
	return err
}
