package reconciliation

import (
	"context"

	"github.com/diogo/golangbet/internal/app/ports"
	"github.com/diogo/golangbet/internal/domain/money"
	"github.com/diogo/golangbet/internal/domain/wallet"
)

type Service struct {
	wallets ports.WalletRepository
	ledger  ports.LedgerRepository
}

func NewService(wallets ports.WalletRepository, ledger ports.LedgerRepository) *Service {
	return &Service{wallets: wallets, ledger: ledger}
}

type Result struct {
	WalletID          string
	StoredBalance     money.Money
	CalculatedBalance money.Money
	Difference        money.Money
	Consistent        bool
	CheckedEntries    int
}

func (s *Service) Reconcile(ctx context.Context, walletID string) (*Result, error) {
	w, err := s.wallets.Get(ctx, wallet.ID(walletID))
	if err != nil {
		return nil, err
	}
	sum, n, err := s.ledger.SumSigned(ctx, w.ID())
	if err != nil {
		return nil, err
	}
	calc, err := money.FromMinor(sum, w.Currency())
	if err != nil {
		return nil, err
	}
	diff, err := w.Balance().Sub(calc)
	if err != nil {
		return nil, err
	}
	return &Result{
		WalletID: string(w.ID()), StoredBalance: w.Balance(), CalculatedBalance: calc,
		Difference: diff, Consistent: diff.IsZero(), CheckedEntries: n,
	}, nil
}
