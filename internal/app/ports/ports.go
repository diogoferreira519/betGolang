package ports

import (
	"context"
	"time"

	"github.com/diogo/golangbet/internal/domain/events"
	"github.com/diogo/golangbet/internal/domain/ledger"
	"github.com/diogo/golangbet/internal/domain/wager"
	"github.com/diogo/golangbet/internal/domain/wallet"
)

type Tx interface {
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

type UnitOfWork interface { Begin(ctx context.Context) (Tx, error) }

type WalletRepository interface {
	Insert(ctx context.Context, tx Tx, w *wallet.Wallet) error
	GetForUpdate(ctx context.Context, tx Tx, id wallet.ID) (*wallet.Wallet, error)
	Get(ctx context.Context, id wallet.ID) (*wallet.Wallet, error)
	Update(ctx context.Context, tx Tx, w *wallet.Wallet, expectedVersion int64) error
}

type WagerRepository interface {
	Insert(ctx context.Context, tx Tx, t *wager.Transaction) error
	Update(ctx context.Context, tx Tx, t *wager.Transaction) error
	GetByID(ctx context.Context, id wager.ID) (*wager.Transaction, error)
	GetByIDTx(ctx context.Context, tx Tx, id wager.ID) (*wager.Transaction, error)
	GetByIdempotencyKey(ctx context.Context, tx Tx, key string) (*wager.Transaction, error)
	GetByProviderExternal(ctx context.Context, tx Tx, providerID, externalID string) (*wager.Transaction, error)
	ListPendingReferences(ctx context.Context, limit int, now time.Time) ([]*wager.Transaction, error)
	CountSuccessfulBetRestorations(ctx context.Context, tx Tx, betID wager.ID) (int, error)
	PendingAttempts(ctx context.Context, id wager.ID) (int, time.Time, error)
	UpdatePendingMeta(ctx context.Context, t Tx, id wager.ID, attempts int, next *time.Time, updatedAt time.Time) error
}

type LedgerRepository interface {
	Insert(ctx context.Context, tx Tx, e *ledger.Entry) error
	ListByWallet(ctx context.Context, walletID wallet.ID, cursorCreatedAt *time.Time, cursorID *string, limit int) ([]*ledger.Entry, error)
	SumSigned(ctx context.Context, walletID wallet.ID) (int64, int, error)
}

type InboxRepository interface {
	TryInsert(ctx context.Context, tx Tx, consumerName, messageID, payloadHash string, now time.Time) (inserted bool, err error)
	Complete(ctx context.Context, tx Tx, consumerName, messageID string, now time.Time) error
	Get(ctx context.Context, tx Tx, consumerName, messageID string) (hash string, completed bool, err error)
}

type OutboxRepository interface {
	Insert(ctx context.Context, tx Tx, env events.Envelope) error
	ClaimBatch(ctx context.Context, tx Tx, owner string, limit int, lease time.Duration, now time.Time) ([]events.Envelope, error)
	MarkPublished(ctx context.Context, tx Tx, eventID string, now time.Time) error
	MarkRetry(ctx context.Context, tx Tx, eventID string, attempts int, next time.Time) error
}

type Clock interface { Now() time.Time }
