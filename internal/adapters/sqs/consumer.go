package sqsadapter

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/diogo/golangbet/internal/adapters/metrics"
	"github.com/diogo/golangbet/internal/app/wagering"
	domerr "github.com/diogo/golangbet/internal/domain/errors"
	"github.com/diogo/golangbet/internal/domain/money"
	"github.com/diogo/golangbet/internal/platform/config"
	"github.com/diogo/golangbet/internal/platform/hash"
)

type Consumer struct {
	client     *sqs.Client
	queueURL   string
	svc        *wagering.Service
	log        *slog.Logger
	metrics    *metrics.Registry
	visTimeout int32
	stop       chan struct{}
	wg         sync.WaitGroup
}

func NewConsumer(client *sqs.Client, cfg config.Config, svc *wagering.Service, log *slog.Logger, m *metrics.Registry) *Consumer {
	return &Consumer{client: client, queueURL: cfg.SQSWagerQueueURL, svc: svc, log: log, metrics: m, visTimeout: cfg.SQSVisibilityTimeout, stop: make(chan struct{})}
}

type envelope struct {
	MessageID  string          `json:"messageId"`
	Type       string          `json:"type"`
	OccurredAt time.Time       `json:"occurredAt"`
	Data       json.RawMessage `json:"data"`
}

type wagerData struct {
	ProviderID, ExternalTransactionID, IdempotencyKey, PlayerID, WalletID, RoundID, GameID, Kind string
	Money struct {
		Amount, Currency string
	} `json:"money"`
	ReferenceExternalTransaction string `json:"referenceExternalTransactionId"`
}

func (c *Consumer) Start(ctx context.Context) {
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		for {
			select {
			case <-c.stop:
				return
			case <-ctx.Done():
				return
			default:
				c.poll(ctx)
			}
		}
	}()
}

func (c *Consumer) Stop(ctx context.Context) {
	close(c.stop)
	done := make(chan struct{})
	go func() { c.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

func (c *Consumer) poll(ctx context.Context) {
	out, err := c.client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl: &c.queueURL, MaxNumberOfMessages: 5, WaitTimeSeconds: 10, VisibilityTimeout: c.visTimeout,
	})
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		c.log.Error("sqs receive", "err", err)
		time.Sleep(time.Second)
		return
	}
	for _, msg := range out.Messages {
		c.handle(ctx, msg)
	}
}

func (c *Consumer) handle(ctx context.Context, msg types.Message) {
	body := []byte("")
	if msg.Body != nil {
		body = []byte(*msg.Body)
	}
	var env envelope
	if err := json.Unmarshal(body, &env); err != nil || env.MessageID == "" {
		c.log.Error("invalid message", "err", err)
		c.delete(ctx, msg)
		if c.metrics != nil {
			c.metrics.IncDLQ()
		}
		return
	}
	var data wagerData
	if err := json.Unmarshal(env.Data, &data); err != nil {
		c.delete(ctx, msg)
		return
	}
	m, err := money.ParseExternal(data.Money.Amount, data.Money.Currency)
	if err != nil {
		c.delete(ctx, msg)
		return
	}
	_, err = c.svc.Submit(ctx, wagering.SubmitInput{
		ProviderID: data.ProviderID, ExternalTransactionID: data.ExternalTransactionID, IdempotencyKey: data.IdempotencyKey,
		PlayerID: data.PlayerID, WalletID: data.WalletID, RoundID: data.RoundID, GameID: data.GameID, Kind: data.Kind, Money: m,
		ReferenceExternalTransaction: data.ReferenceExternalTransaction, CorrelationID: env.MessageID,
		MessageID: env.MessageID, MessageHash: hash.MessageBodyHash(body),
	})
	if err != nil {
		if errors.Is(err, domerr.ErrTransient) || strings.Contains(err.Error(), "connection") || strings.Contains(err.Error(), "timeout") {
			if c.metrics != nil {
				c.metrics.IncRetry()
			}
			return
		}
		c.log.Error("permanent consumer error", "err", err, "messageId", env.MessageID)
		c.delete(ctx, msg)
		return
	}
	c.delete(ctx, msg)
}

func (c *Consumer) delete(ctx context.Context, msg types.Message) {
	if msg.ReceiptHandle == nil {
		return
	}
	_, err := c.client.DeleteMessage(ctx, &sqs.DeleteMessageInput{QueueUrl: &c.queueURL, ReceiptHandle: msg.ReceiptHandle})
	if err != nil {
		c.log.Error("delete message", "err", err)
	}
}
