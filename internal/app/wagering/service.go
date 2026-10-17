package wagering

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/diogo/golangbet/internal/app/ports"
	domerr "github.com/diogo/golangbet/internal/domain/errors"
	"github.com/diogo/golangbet/internal/domain/events"
	"github.com/diogo/golangbet/internal/domain/ledger"
	"github.com/diogo/golangbet/internal/domain/money"
	"github.com/diogo/golangbet/internal/domain/wager"
	"github.com/diogo/golangbet/internal/domain/wallet"
	"github.com/diogo/golangbet/internal/platform/hash"
	"github.com/diogo/golangbet/internal/platform/id"
)

const ConsumerName = "wager-transactions"

type Service struct {
	uow     ports.UnitOfWork
	wallets ports.WalletRepository
	wagers  ports.WagerRepository
	ledger  ports.LedgerRepository
	inbox   ports.InboxRepository
	outbox  ports.OutboxRepository
	clock   ports.Clock
	log     *slog.Logger
}

func NewService(uow ports.UnitOfWork, wallets ports.WalletRepository, wagers ports.WagerRepository, ledger ports.LedgerRepository, inbox ports.InboxRepository, outbox ports.OutboxRepository, clock ports.Clock, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{uow: uow, wallets: wallets, wagers: wagers, ledger: ledger, inbox: inbox, outbox: outbox, clock: clock, log: log}
}

type SubmitInput struct {
	ProviderID, ExternalTransactionID, IdempotencyKey, PlayerID, WalletID, RoundID, GameID, Kind string
	Money                        money.Money
	ReferenceExternalTransaction string
	CorrelationID                string
	MessageID, MessageHash       string
}

type SubmitOutput struct {
	Transaction      *wager.Transaction
	Balance          money.Money
	IdempotentReplay bool
}

func (s *Service) Submit(ctx context.Context, in SubmitInput) (*SubmitOutput, error) {
	if in.Kind == string(wager.KindOpening) {
		return nil, domerr.New(domerr.FailureOpeningNotAllowed, domerr.ErrOpeningNotAllowed, "OPENING not allowed")
	}
	payloadHash, err := hash.PayloadHash(hash.BusinessPayload{
		ProviderID: in.ProviderID, ExternalTransactionID: in.ExternalTransactionID,
		PlayerID: in.PlayerID, WalletID: in.WalletID, RoundID: in.RoundID, GameID: in.GameID,
		Kind: in.Kind, MoneyAmount: in.Money.DecimalString(), MoneyCurrency: in.Money.Currency(),
		ReferenceExternalTransaction: in.ReferenceExternalTransaction,
	})
	if err != nil {
		return nil, err
	}
	now := s.clock.Now()
	corr := in.CorrelationID
	if corr == "" {
		corr = in.IdempotencyKey
	}
	tx, err := s.uow.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if in.MessageID != "" {
		existingHash, completed, err := s.inbox.Get(ctx, tx, ConsumerName, in.MessageID)
		if err != nil {
			return nil, err
		}
		if existingHash != "" {
			if existingHash != in.MessageHash {
				return nil, domerr.New(domerr.FailureValidation, domerr.ErrIdempotencyConflict, "inbox hash mismatch")
			}
			if completed {
				if existing, err := s.wagers.GetByIdempotencyKey(ctx, tx, in.IdempotencyKey); err == nil {
					if err := tx.Commit(ctx); err != nil {
						return nil, err
					}
					return &SubmitOutput{Transaction: existing, Balance: resultBalance(existing), IdempotentReplay: true}, nil
				}
			}
		} else if _, err := s.inbox.TryInsert(ctx, tx, ConsumerName, in.MessageID, in.MessageHash, now); err != nil {
			return nil, err
		}
	}

	if existing, err := s.wagers.GetByIdempotencyKey(ctx, tx, in.IdempotencyKey); err == nil {
		if existing.PayloadHash() != payloadHash {
			return nil, domerr.New(domerr.FailureValidation, domerr.ErrIdempotencyConflict, "idempotency key reused with different payload")
		}
		if in.MessageID != "" {
			_ = s.inbox.Complete(ctx, tx, ConsumerName, in.MessageID, now)
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return &SubmitOutput{Transaction: existing, Balance: resultBalance(existing), IdempotentReplay: true}, nil
	} else if !errors.Is(err, domerr.ErrNotFound) {
		return nil, err
	}

	if existing, err := s.wagers.GetByProviderExternal(ctx, tx, in.ProviderID, in.ExternalTransactionID); err == nil {
		if existing.IdempotencyKey() != in.IdempotencyKey {
			return nil, domerr.New(domerr.FailureValidation, domerr.ErrExternalIDConflict, "external id already used with different key")
		}
		if in.MessageID != "" {
			_ = s.inbox.Complete(ctx, tx, ConsumerName, in.MessageID, now)
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return &SubmitOutput{Transaction: existing, Balance: resultBalance(existing), IdempotentReplay: true}, nil
	} else if !errors.Is(err, domerr.ErrNotFound) {
		return nil, err
	}

	txw, err := wager.CreateExternal(wager.ExternalCreateParams{
		ID: wager.ID(id.New()), Kind: wager.Kind(in.Kind), WalletID: wallet.ID(in.WalletID), PlayerID: wallet.PlayerID(in.PlayerID),
		ProviderID: in.ProviderID, ExternalTransactionID: in.ExternalTransactionID, IdempotencyKey: in.IdempotencyKey,
		PayloadHash: payloadHash, RoundID: in.RoundID, GameID: in.GameID, Money: in.Money,
		ReferenceExternalID: in.ReferenceExternalTransaction, Now: now,
	})
	if err != nil {
		return nil, err
	}
	if err := s.wagers.Insert(ctx, tx, txw); err != nil {
		return nil, err
	}
	out, err := s.processLocked(ctx, tx, txw, corr, now)
	if err != nil {
		return nil, err
	}
	if in.MessageID != "" {
		if err := s.inbox.Complete(ctx, tx, ConsumerName, in.MessageID, now); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Service) processLocked(ctx context.Context, tx ports.Tx, txw *wager.Transaction, corr string, now time.Time) (*SubmitOutput, error) {
	w, err := s.wallets.GetForUpdate(ctx, tx, txw.WalletID())
	if err != nil {
		return nil, err
	}
	if string(w.PlayerID()) != string(txw.PlayerID()) {
		return s.reject(ctx, tx, txw, w, domerr.FailurePlayerMismatch, corr, now)
	}
	if w.Currency() != txw.Money().Currency() {
		return s.reject(ctx, tx, txw, w, domerr.FailureCurrencyMismatch, corr, now)
	}
	kind := txw.Kind()
	if kind == wager.KindRefund || kind == wager.KindRollback {
		ref, err := s.wagers.GetByProviderExternal(ctx, tx, txw.ProviderID(), txw.ReferenceExternalID())
		if err != nil {
			if errors.Is(err, domerr.ErrNotFound) {
				if err := txw.MarkPendingReference(now); err != nil {
					return nil, err
				}
				if err := s.wagers.Update(ctx, tx, txw); err != nil {
					return nil, err
				}
				if err := s.outbox.Insert(ctx, tx, events.NewPendingReference(id.New(), corr, txw, now)); err != nil {
					return nil, err
				}
				return &SubmitOutput{Transaction: txw, Balance: w.Balance()}, nil
			}
			return nil, err
		}
		if ref.Status() != wager.StatusProcessed {
			if ref.Status().Terminal() {
				return s.reject(ctx, tx, txw, w, domerr.FailureReferenceNotProcessed, corr, now)
			}
			if err := txw.MarkPendingReference(now); err != nil {
				return nil, err
			}
			if err := s.wagers.Update(ctx, tx, txw); err != nil {
				return nil, err
			}
			if err := s.outbox.Insert(ctx, tx, events.NewPendingReference(id.New(), corr, txw, now)); err != nil {
				return nil, err
			}
			return &SubmitOutput{Transaction: txw, Balance: w.Balance()}, nil
		}
		if err := s.validateReference(txw, ref); err != nil {
			code := domerr.FailureValidation
			if d, ok := domerr.AsDomain(err); ok {
				code = d.Code
			}
			return s.reject(ctx, tx, txw, w, code, corr, now)
		}
		txw.SetReferenceInternal(ref.ID())
		if kind == wager.KindRefund || (kind == wager.KindRollback && ref.Kind() == wager.KindBet) {
			n, err := s.wagers.CountSuccessfulBetRestorations(ctx, tx, ref.ID())
			if err != nil {
				return nil, err
			}
			if n > 0 {
				return s.reject(ctx, tx, txw, w, domerr.FailureBetAlreadyRestored, corr, now)
			}
		}
		return s.applyMovement(ctx, tx, txw, w, ref, corr, now)
	}
	if kind == wager.KindLoss {
		if err := txw.MarkProcessed(w.Balance(), now); err != nil {
			return nil, err
		}
		if err := s.wagers.Update(ctx, tx, txw); err != nil {
			return nil, err
		}
		if err := s.outbox.Insert(ctx, tx, events.NewProcessed(id.New(), corr, txw, w.Balance(), now)); err != nil {
			return nil, err
		}
		return &SubmitOutput{Transaction: txw, Balance: w.Balance()}, nil
	}
	return s.applyMovement(ctx, tx, txw, w, nil, corr, now)
}

func (s *Service) validateReference(op, ref *wager.Transaction) error {
	if ref.ProviderID() != op.ProviderID() {
		return domerr.New(domerr.FailureProviderMismatch, domerr.ErrValidation, "provider mismatch")
	}
	if ref.PlayerID() != op.PlayerID() || ref.WalletID() != op.WalletID() {
		return domerr.New(domerr.FailurePlayerMismatch, domerr.ErrValidation, "player/wallet mismatch")
	}
	if ref.RoundID() != op.RoundID() {
		return domerr.New(domerr.FailureRoundMismatch, domerr.ErrValidation, "round mismatch")
	}
	if !ref.Money().Equal(op.Money()) {
		return domerr.New(domerr.FailureInvalidAmount, domerr.ErrValidation, "amount must equal reference")
	}
	switch op.Kind() {
	case wager.KindRefund:
		if ref.Kind() != wager.KindBet {
			return domerr.New(domerr.FailureValidation, domerr.ErrValidation, "REFUND requires BET reference")
		}
	case wager.KindRollback:
		if ref.Kind() != wager.KindBet && ref.Kind() != wager.KindWin && ref.Kind() != wager.KindRefund {
			return domerr.New(domerr.FailureValidation, domerr.ErrValidation, "ROLLBACK invalid reference kind")
		}
	}
	return nil
}

func (s *Service) applyMovement(ctx context.Context, tx ports.Tx, txw *wager.Transaction, w *wallet.Wallet, ref *wager.Transaction, corr string, now time.Time) (*SubmitOutput, error) {
	expectedVersion := w.Version()
	var change wallet.BalanceChange
	var dir ledger.Direction
	var err error
	switch txw.Kind() {
	case wager.KindBet:
		change, err = w.Debit(txw.Money(), now)
		dir = ledger.Debit
	case wager.KindWin, wager.KindRefund, wager.KindOpening:
		change, err = w.Credit(txw.Money(), now)
		dir = ledger.Credit
	case wager.KindRollback:
		if ref.Kind() == wager.KindBet {
			change, err = w.Credit(txw.Money(), now)
			dir = ledger.Credit
		} else {
			change, err = w.DebitReversal(txw.Money(), now)
			dir = ledger.Debit
		}
	default:
		return nil, domerr.New(domerr.FailureInvalidKind, domerr.ErrInvalidKind, "no movement")
	}
	if err != nil {
		code := domerr.FailureInsufficientFunds
		if errors.Is(err, domerr.ErrInsufficientFundsReversal) {
			code = domerr.FailureInsufficientFundsReversal
		} else if d, ok := domerr.AsDomain(err); ok {
			code = d.Code
		}
		return s.reject(ctx, tx, txw, w, code, corr, now)
	}
	if err := s.wallets.Update(ctx, tx, w, expectedVersion); err != nil {
		return nil, err
	}
	entry, err := ledger.Create(ledger.CreateParams{
		ID: ledger.EntryID(id.New()), WalletID: w.ID(), TransactionID: ledger.TransactionID(txw.ID()),
		Direction: dir, Money: txw.Money(), BalanceBefore: change.Before, BalanceAfter: change.After, Now: now,
	})
	if err != nil {
		return nil, err
	}
	if err := s.ledger.Insert(ctx, tx, entry); err != nil {
		return nil, err
	}
	if err := txw.MarkProcessed(change.After, now); err != nil {
		return nil, err
	}
	if err := s.wagers.Update(ctx, tx, txw); err != nil {
		return nil, err
	}
	if err := s.outbox.Insert(ctx, tx, events.NewProcessed(id.New(), corr, txw, change.After, now)); err != nil {
		return nil, err
	}
	if err := s.outbox.Insert(ctx, tx, events.NewBalanceChanged(id.New(), corr, w.ID(), txw.ID(), dir, txw.Money(), change.Before, change.After, w.Version(), now)); err != nil {
		return nil, err
	}
	s.log.Info("wager processed", "transactionId", txw.ID(), "walletId", w.ID(), "providerId", txw.ProviderID(), "correlationId", corr)
	return &SubmitOutput{Transaction: txw, Balance: change.After}, nil
}

func (s *Service) reject(ctx context.Context, tx ports.Tx, txw *wager.Transaction, w *wallet.Wallet, code domerr.FailureCode, corr string, now time.Time) (*SubmitOutput, error) {
	if err := txw.MarkRejected(code, w.Balance(), now); err != nil {
		return nil, err
	}
	if err := s.wagers.Update(ctx, tx, txw); err != nil {
		return nil, err
	}
	if err := s.outbox.Insert(ctx, tx, events.NewRejected(id.New(), corr, txw, now)); err != nil {
		return nil, err
	}
	return &SubmitOutput{Transaction: txw, Balance: w.Balance()}, nil
}

func (s *Service) GetByID(ctx context.Context, tid string) (*wager.Transaction, error) {
	return s.wagers.GetByID(ctx, wager.ID(tid))
}

func (s *Service) GetByProviderExternal(ctx context.Context, providerID, externalID string) (*wager.Transaction, error) {
	tx, err := s.uow.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	t, err := s.wagers.GetByProviderExternal(ctx, tx, providerID, externalID)
	if err != nil {
		return nil, err
	}
	_ = tx.Commit(ctx)
	return t, nil
}

func (s *Service) ResumePendingReference(ctx context.Context, txID wager.ID, maxAttempts int, ttl time.Duration) error {
	now := s.clock.Now()
	tx, err := s.uow.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	txw, err := s.wagers.GetByIDTx(ctx, tx, txID)
	if err != nil {
		return err
	}
	if txw.Status() != wager.StatusPendingReference {
		return tx.Commit(ctx)
	}
	corr := txw.IdempotencyKey()
	attempts, createdAt, _ := s.wagers.PendingAttempts(ctx, txID)
	if attempts >= maxAttempts || now.Sub(createdAt) > ttl {
		w, err := s.wallets.GetForUpdate(ctx, tx, txw.WalletID())
		if err != nil {
			return err
		}
		if _, err := s.reject(ctx, tx, txw, w, domerr.FailureReferenceNotFound, corr, now); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	ref, err := s.wagers.GetByProviderExternal(ctx, tx, txw.ProviderID(), txw.ReferenceExternalID())
	if err != nil {
		if errors.Is(err, domerr.ErrNotFound) {
			next := now.Add(backoff(attempts))
			_ = s.wagers.UpdatePendingMeta(ctx, tx, txID, attempts+1, &next, now)
			return tx.Commit(ctx)
		}
		return err
	}
	if ref.Status() != wager.StatusProcessed {
		if ref.Status().Terminal() {
			w, err := s.wallets.GetForUpdate(ctx, tx, txw.WalletID())
			if err != nil {
				return err
			}
			if _, err := s.reject(ctx, tx, txw, w, domerr.FailureReferenceNotProcessed, corr, now); err != nil {
				return err
			}
			return tx.Commit(ctx)
		}
		next := now.Add(backoff(attempts))
		_ = s.wagers.UpdatePendingMeta(ctx, tx, txID, attempts+1, &next, now)
		return tx.Commit(ctx)
	}
	w, err := s.wallets.GetForUpdate(ctx, tx, txw.WalletID())
	if err != nil {
		return err
	}
	if err := s.validateReference(txw, ref); err != nil {
		code := domerr.FailureValidation
		if d, ok := domerr.AsDomain(err); ok {
			code = d.Code
		}
		if _, err := s.reject(ctx, tx, txw, w, code, corr, now); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	txw.SetReferenceInternal(ref.ID())
	if txw.Kind() == wager.KindRefund || (txw.Kind() == wager.KindRollback && ref.Kind() == wager.KindBet) {
		n, err := s.wagers.CountSuccessfulBetRestorations(ctx, tx, ref.ID())
		if err != nil {
			return err
		}
		if n > 0 {
			if _, err := s.reject(ctx, tx, txw, w, domerr.FailureBetAlreadyRestored, corr, now); err != nil {
				return err
			}
			return tx.Commit(ctx)
		}
	}
	if _, err := s.applyMovement(ctx, tx, txw, w, ref, corr, now); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func backoff(attempts int) time.Duration {
	d := time.Second << attempts
	if d > 60*time.Second {
		d = 60 * time.Second
	}
	if d < time.Second {
		d = time.Second
	}
	return d
}

func resultBalance(tx *wager.Transaction) money.Money {
	if tx.ResultBalanceMinor() == nil || tx.ResultBalanceCurrency() == "" {
		z, _ := money.Zero("BRL")
		return z
	}
	m, err := money.FromMinor(*tx.ResultBalanceMinor(), tx.ResultBalanceCurrency())
	if err != nil {
		z, _ := money.Zero("BRL")
		return z
	}
	return m
}
