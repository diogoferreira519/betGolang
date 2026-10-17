package walletapp

import (
	"context"

	"github.com/diogo/golangbet/internal/app/ports"
	"github.com/diogo/golangbet/internal/domain/events"
	"github.com/diogo/golangbet/internal/domain/ledger"
	"github.com/diogo/golangbet/internal/domain/money"
	"github.com/diogo/golangbet/internal/domain/wager"
	"github.com/diogo/golangbet/internal/domain/wallet"
	"github.com/diogo/golangbet/internal/platform/id"
)

type Service struct {
	uow     ports.UnitOfWork
	wallets ports.WalletRepository
	wagers  ports.WagerRepository
	ledger  ports.LedgerRepository
	outbox  ports.OutboxRepository
	clock   ports.Clock
}

func NewService(uow ports.UnitOfWork, wallets ports.WalletRepository, wagers ports.WagerRepository, ledger ports.LedgerRepository, outbox ports.OutboxRepository, clock ports.Clock) *Service {
	return &Service{uow: uow, wallets: wallets, wagers: wagers, ledger: ledger, outbox: outbox, clock: clock}
}

type OpenInput struct {
	PlayerID       string
	InitialBalance money.Money
	CorrelationID  string
}

type OpenOutput struct{ Wallet *wallet.Wallet }

func (s *Service) Open(ctx context.Context, in OpenInput) (*OpenOutput, error) {
	now := s.clock.Now()
	wid := wallet.ID(id.New())
	w, err := wallet.Create(wallet.CreateParams{
		ID: wid, PlayerID: wallet.PlayerID(in.PlayerID), InitialBalance: in.InitialBalance, Now: now,
	})
	if err != nil {
		return nil, err
	}
	tx, err := s.uow.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := s.wallets.Insert(ctx, tx, w); err != nil {
		return nil, err
	}
	if in.InitialBalance.IsPositive() {
		txID := wager.ID(id.New())
		opening, err := wager.CreateOpening(wager.OpeningCreateParams{
			ID: txID, WalletID: wid, PlayerID: wallet.PlayerID(in.PlayerID), Money: in.InitialBalance, Now: now,
		})
		if err != nil {
			return nil, err
		}
		zero, _ := money.Zero(in.InitialBalance.Currency())
		if err := opening.MarkProcessed(in.InitialBalance, now); err != nil {
			return nil, err
		}
		if err := s.wagers.Insert(ctx, tx, opening); err != nil {
			return nil, err
		}
		entry, err := ledger.Create(ledger.CreateParams{
			ID: ledger.EntryID(id.New()), WalletID: wid, TransactionID: ledger.TransactionID(txID),
			Direction: ledger.Credit, Money: in.InitialBalance, BalanceBefore: zero, BalanceAfter: in.InitialBalance, Now: now,
		})
		if err != nil {
			return nil, err
		}
		if err := s.ledger.Insert(ctx, tx, entry); err != nil {
			return nil, err
		}
		corr := in.CorrelationID
		if corr == "" {
			corr = string(txID)
		}
		if err := s.outbox.Insert(ctx, tx, events.NewProcessed(id.New(), corr, opening, in.InitialBalance, now)); err != nil {
			return nil, err
		}
		if err := s.outbox.Insert(ctx, tx, events.NewBalanceChanged(id.New(), corr, wid, txID, ledger.Credit, in.InitialBalance, zero, in.InitialBalance, w.Version(), now)); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &OpenOutput{Wallet: w}, nil
}

func (s *Service) Get(ctx context.Context, walletID string) (*wallet.Wallet, error) {
	return s.wallets.Get(ctx, wallet.ID(walletID))
}
