package sqsadapter

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/diogo/golangbet/internal/app/ports"
	"github.com/diogo/golangbet/internal/app/wagering"
	"github.com/diogo/golangbet/internal/platform/config"
)

type RefWorker struct {
	wagers      ports.WagerRepository
	svc         *wagering.Service
	log         *slog.Logger
	maxAttempts int
	ttl         time.Duration
	stop        chan struct{}
	wg          sync.WaitGroup
}

func NewRefWorker(wagers ports.WagerRepository, svc *wagering.Service, cfg config.Config, log *slog.Logger) *RefWorker {
	return &RefWorker{wagers: wagers, svc: svc, log: log, maxAttempts: cfg.PendingRefMaxAttempts, ttl: cfg.PendingRefTTL, stop: make(chan struct{})}
}

func (w *RefWorker) Start(ctx context.Context) {
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-w.stop:
				return
			case <-ctx.Done():
				return
			case <-t.C:
				w.tick(ctx)
			}
		}
	}()
}

func (w *RefWorker) Stop(ctx context.Context) {
	close(w.stop)
	done := make(chan struct{})
	go func() { w.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

func (w *RefWorker) tick(ctx context.Context) {
	list, err := w.wagers.ListPendingReferences(ctx, 20, time.Now().UTC())
	if err != nil {
		w.log.Error("list pending references", "err", err)
		return
	}
	for _, tx := range list {
		if err := w.svc.ResumePendingReference(ctx, tx.ID(), w.maxAttempts, w.ttl); err != nil {
			w.log.Error("resume pending reference", "err", err, "transactionId", tx.ID())
		}
	}
}
