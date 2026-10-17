package sqsadapter

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/diogo/golangbet/internal/adapters/metrics"
	"github.com/diogo/golangbet/internal/app/ports"
	"github.com/diogo/golangbet/internal/platform/config"
	"github.com/diogo/golangbet/internal/platform/id"
)

type OutboxPublisher struct {
	client   *sqs.Client
	queueURL string
	uow      ports.UnitOfWork
	outbox   ports.OutboxRepository
	log      *slog.Logger
	metrics  *metrics.Registry
	batch    int
	lease    time.Duration
	owner    string
	stop     chan struct{}
	wg       sync.WaitGroup
}

func NewOutboxPublisher(client *sqs.Client, cfg config.Config, uow ports.UnitOfWork, outbox ports.OutboxRepository, log *slog.Logger, m *metrics.Registry) *OutboxPublisher {
	return &OutboxPublisher{
		client: client, queueURL: cfg.SQSEventsQueueURL, uow: uow, outbox: outbox, log: log, metrics: m,
		batch: cfg.OutboxBatchSize, lease: cfg.OutboxLeaseDuration, owner: id.New(), stop: make(chan struct{}),
	}
}

func (p *OutboxPublisher) Start(ctx context.Context) {
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		t := time.NewTicker(500 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-p.stop:
				return
			case <-ctx.Done():
				return
			case <-t.C:
				p.publishBatch(ctx)
			}
		}
	}()
}

func (p *OutboxPublisher) Stop(ctx context.Context) {
	close(p.stop)
	done := make(chan struct{})
	go func() { p.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

func (p *OutboxPublisher) publishBatch(ctx context.Context) {
	tx, err := p.uow.Begin(ctx)
	if err != nil {
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()
	now := time.Now().UTC()
	envs, err := p.outbox.ClaimBatch(ctx, tx, p.owner, p.batch, p.lease, now)
	if err != nil {
		return
	}
	if err := tx.Commit(ctx); err != nil {
		return
	}
	for _, env := range envs {
		body, err := json.Marshal(env)
		if err != nil {
			p.retry(ctx, env.EventID)
			continue
		}
		group, dedup := env.AggregateID, env.EventID
		bodyStr := string(body)
		_, err = p.client.SendMessage(ctx, &sqs.SendMessageInput{
			QueueUrl: &p.queueURL, MessageBody: &bodyStr, MessageGroupId: &group, MessageDeduplicationId: &dedup,
		})
		if err != nil {
			p.log.Error("outbox publish", "err", err, "eventId", env.EventID)
			p.retry(ctx, env.EventID)
			continue
		}
		tx2, err := p.uow.Begin(ctx)
		if err != nil {
			continue
		}
		if err := p.outbox.MarkPublished(ctx, tx2, env.EventID, time.Now().UTC()); err != nil {
			_ = tx2.Rollback(ctx)
			continue
		}
		_ = tx2.Commit(ctx)
	}
}

func (p *OutboxPublisher) retry(ctx context.Context, eventID string) {
	tx, err := p.uow.Begin(ctx)
	if err != nil {
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()
	_ = p.outbox.MarkRetry(ctx, tx, eventID, 1, time.Now().UTC().Add(2*time.Second))
	_ = tx.Commit(ctx)
}
