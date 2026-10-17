package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/diogo/golangbet/internal/adapters/auth"
	"github.com/diogo/golangbet/internal/adapters/http/dto"
	"github.com/diogo/golangbet/internal/adapters/metrics"
	reconapp "github.com/diogo/golangbet/internal/app/reconciliation"
	walletapp "github.com/diogo/golangbet/internal/app/wallet"
	"github.com/diogo/golangbet/internal/app/wagering"
	domerr "github.com/diogo/golangbet/internal/domain/errors"
	"github.com/diogo/golangbet/internal/domain/ledger"
	"github.com/diogo/golangbet/internal/domain/money"
	"github.com/diogo/golangbet/internal/domain/wager"
	"github.com/diogo/golangbet/internal/domain/wallet"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type LedgerRepo interface {
	ListByWallet(ctx context.Context, walletID wallet.ID, cursorCreatedAt *time.Time, cursorID *string, limit int) ([]*ledger.Entry, error)
}

type API struct {
	wallets *walletapp.Service
	wagers  *wagering.Service
	recon   *reconapp.Service
	ledger  LedgerRepo
	log     *slog.Logger
	metrics *metrics.Registry
}

func NewAPI(wallets *walletapp.Service, wagers *wagering.Service, recon *reconapp.Service, ledgerRepo LedgerRepo, log *slog.Logger, m *metrics.Registry) *API {
	return &API{wallets: wallets, wagers: wagers, recon: recon, ledger: ledgerRepo, log: log, metrics: m}
}

func (a *API) OpenWallet(w http.ResponseWriter, r *http.Request) {
	var req dto.OpenWalletRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), "")
		return
	}
	if _, err := uuid.Parse(req.PlayerID); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid playerId", "")
		return
	}
	bal, err := req.InitialBalance.ToMoney()
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), "")
		return
	}
	out, err := a.wallets.Open(r.Context(), walletapp.OpenInput{PlayerID: req.PlayerID, InitialBalance: bal, CorrelationID: r.Header.Get("X-Correlation-Id")})
	if err != nil {
		if errors.Is(err, domerr.ErrWalletConflict) {
			writeError(w, http.StatusConflict, "CONFLICT", err.Error(), "")
			return
		}
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, dto.WalletResponse{ID: string(out.Wallet.ID()), PlayerID: string(out.Wallet.PlayerID()), Balance: dto.FromMoney(out.Wallet.Balance()), Version: out.Wallet.Version()})
}

func (a *API) GetWallet(w http.ResponseWriter, r *http.Request) {
	wal, err := a.wallets.Get(r.Context(), chi.URLParam(r, "walletId"))
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, dto.WalletResponse{ID: string(wal.ID()), PlayerID: string(wal.PlayerID()), Balance: dto.FromMoney(wal.Balance()), Version: wal.Version()})
}

func (a *API) GetLedger(w http.ResponseWriter, r *http.Request) {
	walletID := chi.URLParam(r, "walletId")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	var cursorAt *time.Time
	var cursorID *string
	if c := r.URL.Query().Get("cursor"); c != "" {
		if raw, err := base64.RawURLEncoding.DecodeString(c); err == nil {
			parts := strings.SplitN(string(raw), "|", 2)
			if len(parts) == 2 {
				if t, err := time.Parse(time.RFC3339Nano, parts[0]); err == nil {
					cursorAt = &t
					id := parts[1]
					cursorID = &id
				}
			}
		}
	}
	entries, err := a.ledger.ListByWallet(r.Context(), wallet.ID(walletID), cursorAt, cursorID, limit)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	resp := dto.LedgerPageResponse{Items: make([]dto.LedgerEntryResponse, 0, len(entries))}
	for _, e := range entries {
		resp.Items = append(resp.Items, dto.LedgerEntryResponse{
			ID: string(e.ID()), TransactionID: string(e.TransactionID()), Direction: string(e.Direction()),
			Money: dto.FromMoney(e.Money()), BalanceBefore: dto.FromMoney(e.BalanceBefore()),
			BalanceAfter: dto.FromMoney(e.BalanceAfter()), CreatedAt: e.CreatedAt().Format(time.RFC3339Nano),
		})
	}
	if len(entries) == limit {
		last := entries[len(entries)-1]
		resp.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(last.CreatedAt().Format(time.RFC3339Nano) + "|" + string(last.ID())))
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *API) SubmitWager(w http.ResponseWriter, r *http.Request) {
	p, ok := auth.FromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing principal", "")
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if key == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Idempotency-Key required", "")
		return
	}
	var req dto.SubmitWagerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), "")
		return
	}
	if !p.Internal && req.ProviderID != p.ProviderID {
		writeError(w, http.StatusForbidden, "FORBIDDEN", "provider mismatch", "")
		return
	}
	m, err := req.Money.ToMoney()
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), "")
		return
	}
	start := time.Now()
	out, err := a.wagers.Submit(r.Context(), wagering.SubmitInput{
		ProviderID: req.ProviderID, ExternalTransactionID: req.ExternalTransactionID, IdempotencyKey: key,
		PlayerID: req.PlayerID, WalletID: req.WalletID, RoundID: req.RoundID, GameID: req.GameID, Kind: req.Kind, Money: m,
		ReferenceExternalTransaction: req.ReferenceExternalTransaction, CorrelationID: r.Header.Get("X-Correlation-Id"),
	})
	if a.metrics != nil {
		a.metrics.ObserveProcessing(time.Since(start))
	}
	if err != nil {
		if errors.Is(err, domerr.ErrIdempotencyConflict) || errors.Is(err, domerr.ErrExternalIDConflict) {
			if a.metrics != nil {
				a.metrics.IncDuplicate()
			}
			writeError(w, http.StatusConflict, "CONFLICT", err.Error(), "")
			return
		}
		if errors.Is(err, domerr.ErrTransient) {
			writeError(w, http.StatusServiceUnavailable, "TRANSIENT", err.Error(), "")
			return
		}
		writeDomainError(w, err)
		return
	}
	resp := dto.SubmitWagerResponse{
		TransactionID: string(out.Transaction.ID()), Status: string(out.Transaction.Status()),
		Balance: dto.FromMoney(out.Balance), IdempotentReplay: out.IdempotentReplay, FailureCode: string(out.Transaction.FailureCode()),
	}
	if a.metrics != nil {
		a.metrics.IncStatus(string(out.Transaction.Status()))
		if out.IdempotentReplay {
			a.metrics.IncDuplicate()
		}
	}
	status := http.StatusOK
	switch out.Transaction.Status() {
	case wager.StatusPending, wager.StatusPendingReference:
		status = http.StatusAccepted
	case wager.StatusRejected:
		status = http.StatusUnprocessableEntity
	}
	writeJSON(w, status, resp)
}

func (a *API) GetTransaction(w http.ResponseWriter, r *http.Request) {
	tx, err := a.wagers.GetByID(r.Context(), chi.URLParam(r, "transactionId"))
	if err != nil {
		writeDomainError(w, err)
		return
	}
	if err := a.authorizeTx(r, tx); err != nil {
		writeError(w, http.StatusForbidden, "FORBIDDEN", err.Error(), "")
		return
	}
	writeJSON(w, http.StatusOK, toTxDTO(tx))
}

func (a *API) GetProviderTransaction(w http.ResponseWriter, r *http.Request) {
	providerID := chi.URLParam(r, "providerId")
	p, ok := auth.FromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing principal", "")
		return
	}
	if !p.Internal && p.ProviderID != providerID {
		writeError(w, http.StatusForbidden, "FORBIDDEN", "provider mismatch", "")
		return
	}
	tx, err := a.wagers.GetByProviderExternal(r.Context(), providerID, chi.URLParam(r, "externalTransactionId"))
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toTxDTO(tx))
}

func (a *API) Reconcile(w http.ResponseWriter, r *http.Request) {
	res, err := a.recon.Reconcile(r.Context(), chi.URLParam(r, "walletId"))
	if err != nil {
		writeDomainError(w, err)
		return
	}
	if !res.Consistent && a.metrics != nil {
		a.metrics.IncReconciliationDivergence()
		a.log.Error("reconciliation divergence", "walletId", res.WalletID)
	}
	writeJSON(w, http.StatusOK, dto.ReconciliationResponse{
		WalletID: res.WalletID, StoredBalance: dto.FromMoney(res.StoredBalance),
		CalculatedBalance: dto.FromMoney(res.CalculatedBalance), Difference: dto.FromMoney(res.Difference),
		Consistent: res.Consistent, CheckedEntries: res.CheckedEntries,
	})
}

func (a *API) authorizeTx(r *http.Request, tx *wager.Transaction) error {
	p, ok := auth.FromContext(r.Context())
	if !ok {
		return errors.New("missing principal")
	}
	if p.Internal || tx.ProviderID() == p.ProviderID {
		return nil
	}
	return errors.New("provider mismatch")
}

func toTxDTO(tx *wager.Transaction) dto.TransactionResponse {
	resp := dto.TransactionResponse{
		TransactionID: string(tx.ID()), Status: string(tx.Status()), Kind: string(tx.Kind()),
		ProviderID: tx.ProviderID(), ExternalTransactionID: tx.ExternalTransactionID(),
		WalletID: string(tx.WalletID()), PlayerID: string(tx.PlayerID()), Money: dto.FromMoney(tx.Money()),
		FailureCode: string(tx.FailureCode()), IdempotencyKey: tx.IdempotencyKey(),
	}
	if tx.ResultBalanceMinor() != nil && tx.ResultBalanceCurrency() != "" {
		if m, err := money.FromMinor(*tx.ResultBalanceMinor(), tx.ResultBalanceCurrency()); err == nil {
			b := dto.FromMoney(m)
			resp.Balance = &b
		}
	}
	return resp
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, msg, failure string) {
	writeJSON(w, status, dto.ErrorResponse{Error: code, Message: msg, Code: failure})
}

func writeDomainError(w http.ResponseWriter, err error) {
	if errors.Is(err, domerr.ErrNotFound) || errors.Is(err, domerr.ErrWalletNotFound) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", err.Error(), "")
		return
	}
	if d, ok := domerr.AsDomain(err); ok {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", d.Error(), string(d.Code))
		return
	}
	writeError(w, http.StatusInternalServerError, "INTERNAL", err.Error(), "")
}
