package httpadapter

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/diogo/golangbet/internal/adapters/auth"
	"github.com/diogo/golangbet/internal/adapters/http/handlers"
	"github.com/diogo/golangbet/internal/adapters/http/middleware"
	"github.com/diogo/golangbet/internal/adapters/metrics"
	"github.com/diogo/golangbet/internal/adapters/postgres"
	"github.com/diogo/golangbet/internal/platform/config"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
)

type ReadyDeps struct {
	Pool     *postgres.Pool
	SQS      *sqs.Client
	QueueURL string
}

func NewRouter(cfg config.Config, api *handlers.API, validator *auth.Validator, ready ReadyDeps, m *metrics.Registry) http.Handler {
	r := chi.NewRouter()
	r.Use(chimw.RequestID, chimw.RealIP, chimw.Recoverer)
	r.Get("/health/live", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"live"}`))
	})
	r.Get("/health/ready", func(w http.ResponseWriter, req *http.Request) {
		ctx, cancel := context.WithTimeout(req.Context(), 3*time.Second)
		defer cancel()
		if err := ready.Pool.Ping(ctx); err != nil {
			writeReady(w, http.StatusServiceUnavailable, false, err.Error())
			return
		}
		if _, err := ready.SQS.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{QueueUrl: &ready.QueueURL}); err != nil {
			writeReady(w, http.StatusServiceUnavailable, false, err.Error())
			return
		}
		writeReady(w, http.StatusOK, true, "ok")
	})
	r.Handle(cfg.MetricsAddr, m.Handler())
	r.Group(func(pr chi.Router) {
		pr.Use(middleware.Authenticate(validator))
		pr.With(middleware.RequireInternal).Post("/wallets", api.OpenWallet)
		pr.With(middleware.RequireInternal).Get("/wallets/{walletId}", api.GetWallet)
		pr.With(middleware.RequireInternal).Get("/wallets/{walletId}/ledger", api.GetLedger)
		pr.With(middleware.RequireInternal).Post("/wallets/{walletId}/reconciliation", api.Reconcile)
		pr.Post("/wagering/transactions", api.SubmitWager)
		pr.Get("/wagering/transactions/{transactionId}", api.GetTransaction)
		pr.Get("/providers/{providerId}/wagering/transactions/{externalTransactionId}", api.GetProviderTransaction)
	})
	return r
}

func writeReady(w http.ResponseWriter, status int, ready bool, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"ready": ready, "message": msg})
}
