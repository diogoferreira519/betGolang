package fxmodules

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/diogo/golangbet/internal/adapters/auth"
	httpadapter "github.com/diogo/golangbet/internal/adapters/http"
	"github.com/diogo/golangbet/internal/adapters/http/handlers"
	"github.com/diogo/golangbet/internal/adapters/metrics"
	"github.com/diogo/golangbet/internal/adapters/postgres"
	sqsadapter "github.com/diogo/golangbet/internal/adapters/sqs"
	"github.com/diogo/golangbet/internal/app/ports"
	reconapp "github.com/diogo/golangbet/internal/app/reconciliation"
	walletapp "github.com/diogo/golangbet/internal/app/wallet"
	"github.com/diogo/golangbet/internal/app/wagering"
	"github.com/diogo/golangbet/internal/platform/clock"
	"github.com/diogo/golangbet/internal/platform/config"
	"go.uber.org/fx"
)

var Module = fx.Options(
	fx.Provide(
		config.Load,
		provideLogger,
		providePool,
		provideMigrate,
		postgres.NewUnitOfWork,
		func(p *postgres.Pool) *postgres.WalletRepo { return postgres.NewWalletRepo(p) },
		func(p *postgres.Pool) *postgres.WagerRepo { return postgres.NewWagerRepo(p) },
		func(p *postgres.Pool) *postgres.LedgerRepo { return postgres.NewLedgerRepo(p) },
		postgres.NewInboxRepo,
		postgres.NewOutboxRepo,
		func(r *postgres.WalletRepo) ports.WalletRepository { return r },
		func(r *postgres.WagerRepo) ports.WagerRepository { return r },
		func(r *postgres.LedgerRepo) ports.LedgerRepository { return r },
		func(r *postgres.InboxRepo) ports.InboxRepository { return r },
		func(r *postgres.OutboxRepo) ports.OutboxRepository { return r },
		func() ports.Clock { return clock.System{} },
		metrics.New,
		provideAuth,
		walletapp.NewService,
		wagering.NewService,
		reconapp.NewService,
		func(wallets *walletapp.Service, wagers *wagering.Service, recon *reconapp.Service, ledger *postgres.LedgerRepo, log *slog.Logger, m *metrics.Registry) *handlers.API {
			return handlers.NewAPI(wallets, wagers, recon, ledger, log, m)
		},
		sqsadapter.NewClient,
		func(client *sqs.Client, cfg config.Config, svc *wagering.Service, log *slog.Logger, m *metrics.Registry) *sqsadapter.Consumer {
			return sqsadapter.NewConsumer(client, cfg, svc, log, m)
		},
		func(client *sqs.Client, cfg config.Config, uow ports.UnitOfWork, outbox ports.OutboxRepository, log *slog.Logger, m *metrics.Registry) *sqsadapter.OutboxPublisher {
			return sqsadapter.NewOutboxPublisher(client, cfg, uow, outbox, log, m)
		},
		func(wagers ports.WagerRepository, svc *wagering.Service, cfg config.Config, log *slog.Logger) *sqsadapter.RefWorker {
			return sqsadapter.NewRefWorker(wagers, svc, cfg, log)
		},
		provideRouter,
	),
	fx.Invoke(registerLifecycle),
)

func provideLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
}

func providePool(lc fx.Lifecycle, cfg config.Config) (*postgres.Pool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := postgres.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	lc.Append(fx.Hook{OnStop: func(ctx context.Context) error {
		pool.Close()
		return nil
	}})
	return pool, nil
}

type migrateResult struct{}

func provideMigrate(cfg config.Config, _ *postgres.Pool) (migrateResult, error) {
	return migrateResult{}, postgres.RunMigrations(cfg.DatabaseURL, cfg.MigrationsPath)
}

func provideAuth(cfg config.Config) (*auth.Validator, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return auth.NewValidator(ctx, cfg.OIDCIssuerURL, cfg.OIDCAudience, cfg.InternalClientID)
}

func provideRouter(cfg config.Config, api *handlers.API, v *auth.Validator, pool *postgres.Pool, client *sqs.Client, m *metrics.Registry) http.Handler {
	return httpadapter.NewRouter(cfg, api, v, httpadapter.ReadyDeps{Pool: pool, SQS: client, QueueURL: cfg.SQSWagerQueueURL}, m)
}

func registerLifecycle(lc fx.Lifecycle, cfg config.Config, handler http.Handler, consumer *sqsadapter.Consumer, publisher *sqsadapter.OutboxPublisher, refWorker *sqsadapter.RefWorker, log *slog.Logger) {
	srv := &http.Server{Addr: cfg.HTTPAddr, Handler: handler}
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			log.Info("starting http", "addr", cfg.HTTPAddr)
			go func() {
				if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					log.Error("http server failed", "err", err)
				}
			}()
			workerCtx := context.Background()
			consumer.Start(workerCtx)
			publisher.Start(workerCtx)
			refWorker.Start(workerCtx)
			return nil
		},
		OnStop: func(ctx context.Context) error {
			log.Info("shutting down")
			shutdownCtx, cancel := context.WithTimeout(ctx, cfg.ShutdownTimeout)
			defer cancel()
			_ = srv.Shutdown(shutdownCtx)
			consumer.Stop(shutdownCtx)
			publisher.Stop(shutdownCtx)
			refWorker.Stop(shutdownCtx)
			return nil
		},
	})
}
